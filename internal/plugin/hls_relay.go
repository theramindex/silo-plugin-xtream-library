package plugin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	upstreamhttp "github.com/theramindex/silo-plugin-xtream-library/internal/upstream/httpclient"
)

const (
	relayTokenTTL        = 15 * time.Minute
	maxRelayResponseSize = int64(32 << 20)
)

var hlsURIAttribute = regexp.MustCompile(`URI="([^"]+)"`)

type relayTarget struct {
	url       string
	expiresAt time.Time
}

type hlsRelay struct {
	mu      sync.Mutex
	targets map[string]relayTarget
	client  *http.Client
}

func newHLSRelay() *hlsRelay {
	client := upstreamhttp.New()
	client.Timeout = 30 * time.Second
	return &hlsRelay{targets: map[string]relayTarget{}, client: client}
}

func (r *hlsRelay) register(rawURL string) (string, error) {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return "", fmt.Errorf("invalid relay target")
	}
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("create relay token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	now := time.Now()
	r.mu.Lock()
	for existing, target := range r.targets {
		if now.After(target.expiresAt) {
			delete(r.targets, existing)
		}
	}
	r.targets[token] = relayTarget{url: rawURL, expiresAt: now.Add(relayTokenTTL)}
	r.mu.Unlock()
	return token, nil
}

func (r *hlsRelay) target(token string) (string, bool) {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	target, ok := r.targets[token]
	if !ok || now.After(target.expiresAt) {
		delete(r.targets, token)
		return "", false
	}
	target.expiresAt = now.Add(relayTokenTTL)
	r.targets[token] = target
	return target.url, true
}

func (r *hlsRelay) start(ctx context.Context, rawURL string, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	return r.fetch(ctx, rawURL, request)
}

func (r *hlsRelay) resume(ctx context.Context, token string, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	rawURL, ok := r.target(token)
	if !ok {
		return textResponse(http.StatusGone, "relay token expired"), nil
	}
	return r.fetch(ctx, rawURL, request)
}

func (r *hlsRelay) fetch(ctx context.Context, rawURL string, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return textResponse(http.StatusBadGateway, "invalid upstream stream url"), nil
	}
	if rangeHeader := request.GetHeaders()["range"]; rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	response, err := r.client.Do(req)
	if err != nil {
		return textResponse(http.StatusBadGateway, upstreamhttp.RedactErrorURL(err).Error()), nil
	}
	defer response.Body.Close()
	body, err := upstreamhttp.ReadAllLimit(response.Body, maxRelayResponseSize)
	if err != nil {
		return textResponse(http.StatusBadGateway, "upstream stream response is too large or unreadable"), nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return textResponse(http.StatusBadGateway, fmt.Sprintf("upstream stream returned status %d", response.StatusCode)), nil
	}
	contentType := response.Header.Get("Content-Type")
	if isHLSManifest(response.Request.URL, contentType, body) {
		body, err = r.rewriteManifest(response.Request.URL, body)
		if err != nil {
			return textResponse(http.StatusBadGateway, "unable to rewrite upstream HLS manifest"), nil
		}
		contentType = "application/vnd.apple.mpegurl"
	}
	headers := map[string]string{"cache-control": "no-store", "content-type": contentType}
	if headers["content-type"] == "" {
		headers["content-type"] = "application/octet-stream"
	}
	for _, name := range []string{"Accept-Ranges", "Content-Range"} {
		if value := response.Header.Get(name); value != "" {
			headers[strings.ToLower(name)] = value
		}
	}
	return &pluginv1.HandleHTTPResponse{StatusCode: int32(response.StatusCode), Headers: headers, Body: body}, nil
}

func isHLSManifest(requestURL *url.URL, contentType string, body []byte) bool {
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "mpegurl") || strings.HasSuffix(strings.ToLower(requestURL.Path), ".m3u8") || strings.HasPrefix(strings.TrimSpace(string(body)), "#EXTM3U")
}

func (r *hlsRelay) rewriteManifest(baseURL *url.URL, body []byte) ([]byte, error) {
	lines := strings.Split(string(body), "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			rewritten, err := r.relayReference(baseURL, trimmed, "stream")
			if err != nil {
				return nil, err
			}
			lines[index] = rewritten
			continue
		}
		var rewriteErr error
		lines[index] = hlsURIAttribute.ReplaceAllStringFunc(line, func(match string) string {
			parts := hlsURIAttribute.FindStringSubmatch(match)
			if len(parts) != 2 {
				return match
			}
			rewritten, err := r.relayReference(baseURL, parts[1], "stream")
			if err != nil {
				rewriteErr = err
				return match
			}
			return `URI="` + rewritten + `"`
		})
		if rewriteErr != nil {
			return nil, rewriteErr
		}
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func (r *hlsRelay) relayReference(baseURL *url.URL, reference string, route string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil {
		return "", err
	}
	token, err := r.register(baseURL.ResolveReference(parsed).String())
	if err != nil {
		return "", err
	}
	return route + "?relay_token=" + url.QueryEscape(token), nil
}

func (r *hlsRelay) imageURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "http" {
		return rawURL
	}
	token, err := r.register(parsed.String())
	if err != nil {
		return rawURL
	}
	return "xtream/image?relay_token=" + url.QueryEscape(token)
}
