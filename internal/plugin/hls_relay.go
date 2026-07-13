package plugin

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	upstreamhttp "github.com/theramindex/silo-plugin-xtream-library/internal/upstream/httpclient"
)

const (
	relayTokenTTL        = 15 * time.Minute
	maxRelayResponseSize = int64(32 << 20)
	defaultRelayKeyFile  = "/var/lib/continuum/plugins/silo.ramindex.xtream/relay.key"
)

var hlsURIAttribute = regexp.MustCompile(`URI="([^"]+)"`)

type hlsRelay struct {
	keyPath string
	client  *http.Client
}

func newHLSRelay() *hlsRelay {
	client := upstreamhttp.New()
	client.Timeout = 30 * time.Second
	return &hlsRelay{keyPath: defaultRelayKeyFile, client: client}
}

func (r *hlsRelay) register(rawURL string) (string, error) {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return "", fmt.Errorf("invalid relay target")
	}
	key, err := r.signingKey()
	if err != nil {
		return "", err
	}
	payload := fmt.Sprintf("%d\n%s", time.Now().Add(relayTokenTTL).Unix(), rawURL)
	block, err := aes.NewCipher(key[:32])
	if err != nil {
		return "", fmt.Errorf("create relay cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create relay token cipher: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("create relay nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(payload), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (r *hlsRelay) target(token string) (string, bool) {
	key, err := r.signingKey()
	if err != nil {
		return "", false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", false
	}
	block, err := aes.NewCipher(key[:32])
	if err != nil {
		return "", false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(sealed) < gcm.NonceSize() {
		return "", false
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	payload, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", false
	}
	values := strings.SplitN(string(payload), "\n", 2)
	if len(values) != 2 {
		return "", false
	}
	expiresAt, err := time.Parse(time.RFC3339, values[0])
	if err != nil {
		var expiresUnix int64
		if _, scanErr := fmt.Sscan(values[0], &expiresUnix); scanErr != nil {
			return "", false
		}
		expiresAt = time.Unix(expiresUnix, 0)
	}
	return values[1], time.Now().Before(expiresAt)
}

func (r *hlsRelay) signingKey() ([]byte, error) {
	if data, err := os.ReadFile(r.keyPath); err == nil && len(data) >= 32 {
		return data, nil
	}
	if err := os.MkdirAll(filepath.Dir(r.keyPath), 0o700); err != nil {
		return nil, fmt.Errorf("prepare relay key: %w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("create relay key: %w", err)
	}
	file, err := os.OpenFile(r.keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if _, writeErr := file.Write(key); writeErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("write relay key: %w", writeErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return nil, fmt.Errorf("close relay key: %w", closeErr)
		}
		return key, nil
	}
	data, readErr := os.ReadFile(r.keyPath)
	if readErr != nil || len(data) < 32 {
		return nil, fmt.Errorf("load relay key: %w", err)
	}
	return data, nil
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
