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
	"sort"
	"strings"
	"sync"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	upstreamhttp "github.com/theramindex/silo-plugin-xtream-library/internal/upstream/httpclient"
)

const (
	relayTokenTTL        = 15 * time.Minute
	relayFetchTimeout    = 30 * time.Second
	maxRelayResponseSize = int64(32 << 20)
	defaultRelayKeyFile  = "/var/lib/continuum/plugins/silo.ramindex.xtream/relay.key"
	// relayClientCacheLimit bounds distinct allowlists kept with warm
	// connection pools; it only grows when origin hosts differ (M3U lineups).
	relayClientCacheLimit = 32
)

var (
	hlsURIAttribute = regexp.MustCompile(`URI="([^"]+)"`)
	// Xtream-style credential paths: /live|movie|series|timeshift/USER/PASS/...
	xtreamCredentialPath = regexp.MustCompile(`(?i)^/(live|movie|series|timeshift)/[^/]+/[^/]+/`)
	// Bare /USER/PASS/12345[.ts] paths, served by many M3U providers.
	xtreamStylePath = regexp.MustCompile(`^/[^/]+/[^/]+/[0-9]+(\.[A-Za-z0-9]+)?$`)
)

// sensitiveQueryKeys are compared case-insensitively with "-" folded to "_".
var sensitiveQueryKeys = map[string]bool{
	"password": true, "pass": true, "passwd": true, "pwd": true, "secret": true,
	"token": true, "access_token": true, "auth": true, "auth_token": true, "authorization": true,
	"api_key": true, "apikey": true, "key": true, "signature": true, "sig": true,
}

// hlsRelay fetches provider HLS manifests and segments on behalf of the
// browser so credentialed provider URLs never leave the plugin. Manifest
// references come from untrusted upstream content, so every fetch uses a
// restricted client: loopback/private/link-local/multicast destinations are
// refused at dial time unless the host is an admin-configured provider host or
// the origin host of the stream the relay chain started from. That origin is
// sealed into each relay token, so short-lived plugin processes can enforce
// the same policy without shared state.
type hlsRelay struct {
	keyPath string

	clientsMu sync.Mutex
	clients   map[string]*http.Client
}

func newHLSRelay() *hlsRelay {
	return &hlsRelay{keyPath: defaultRelayKeyFile}
}

// register seals rawURL and its trusted origin host into a relay token.
func (r *hlsRelay) register(rawURL, originHost string) (string, error) {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return "", fmt.Errorf("invalid relay target")
	}
	key, err := r.signingKey()
	if err != nil {
		return "", err
	}
	// Newlines cannot appear in a parsed URL or host, so they delimit fields.
	payload := fmt.Sprintf("%d\n%s\n%s", time.Now().Add(relayTokenTTL).Unix(), rawURL, strings.TrimSpace(originHost))
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

// target opens a relay token, returning the upstream URL and its origin host.
// Tokens minted before origins were sealed decode with an empty origin, which
// limits them to the configured provider hosts and public addresses.
func (r *hlsRelay) target(token string) (string, string, bool) {
	key, err := r.signingKey()
	if err != nil {
		return "", "", false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", "", false
	}
	block, err := aes.NewCipher(key[:32])
	if err != nil {
		return "", "", false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(sealed) < gcm.NonceSize() {
		return "", "", false
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	payload, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", "", false
	}
	values := strings.SplitN(string(payload), "\n", 3)
	if len(values) < 2 {
		return "", "", false
	}
	expiresAt, err := time.Parse(time.RFC3339, values[0])
	if err != nil {
		var expiresUnix int64
		if _, scanErr := fmt.Sscan(values[0], &expiresUnix); scanErr != nil {
			return "", "", false
		}
		expiresAt = time.Unix(expiresUnix, 0)
	}
	origin := ""
	if len(values) == 3 {
		origin = values[2]
	}
	return values[1], origin, time.Now().Before(expiresAt)
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

// start relays a catalog-resolved stream URL. That URL is trusted, so its host
// becomes the origin for every reference minted from the manifest.
func (r *hlsRelay) start(ctx context.Context, rawURL string, configuredHosts []string, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	return r.fetch(ctx, rawURL, urlHost(rawURL), configuredHosts, request)
}

func (r *hlsRelay) resume(ctx context.Context, token string, configuredHosts []string, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	rawURL, origin, ok := r.target(token)
	if !ok {
		return textResponse(http.StatusGone, "relay token expired"), nil
	}
	return r.fetch(ctx, rawURL, origin, configuredHosts, request)
}

func (r *hlsRelay) fetch(ctx context.Context, rawURL, origin string, configuredHosts []string, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || upstreamhttp.CheckScheme(parsed.Scheme) != nil {
		return textResponse(http.StatusBadGateway, "invalid upstream stream url"), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return textResponse(http.StatusBadGateway, "invalid upstream stream url"), nil
	}
	if rangeHeader := request.GetHeaders()["range"]; rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	response, err := r.clientFor(append(append([]string(nil), configuredHosts...), origin)).Do(req)
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
		body, err = r.rewriteManifest(response.Request.URL, origin, body)
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

// clientFor returns a restricted client for the allowlist, reusing clients so
// segment fetches keep warm connection pools.
func (r *hlsRelay) clientFor(allowedHosts []string) *http.Client {
	hosts := make([]string, 0, len(allowedHosts))
	seen := map[string]bool{}
	for _, host := range allowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" && !seen[host] {
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	sort.Strings(hosts)
	cacheKey := strings.Join(hosts, "\n")

	r.clientsMu.Lock()
	defer r.clientsMu.Unlock()
	if client, ok := r.clients[cacheKey]; ok {
		return client
	}
	if r.clients == nil || len(r.clients) >= relayClientCacheLimit {
		for _, stale := range r.clients {
			stale.CloseIdleConnections()
		}
		r.clients = map[string]*http.Client{}
	}
	client := upstreamhttp.NewRestricted(hosts...)
	client.Timeout = relayFetchTimeout
	r.clients[cacheKey] = client
	return client
}

func isHLSManifest(requestURL *url.URL, contentType string, body []byte) bool {
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "mpegurl") || strings.HasSuffix(strings.ToLower(requestURL.Path), ".m3u8") || strings.HasPrefix(strings.TrimSpace(string(body)), "#EXTM3U")
}

func (r *hlsRelay) rewriteManifest(baseURL *url.URL, origin string, body []byte) ([]byte, error) {
	lines := strings.Split(string(body), "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			rewritten, ok, err := r.relayReference(baseURL, origin, trimmed, "stream")
			if err != nil {
				return nil, err
			}
			if !ok {
				// Non-http(s) media URIs are dropped rather than handed to the
				// browser or fetched.
				lines[index] = ""
				continue
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
			rewritten, ok, err := r.relayReference(baseURL, origin, parts[1], "stream")
			if err != nil {
				rewriteErr = err
				return match
			}
			if !ok {
				// Key-system URIs such as skd:// stay as they are.
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

// relayReference resolves a manifest reference and mints a relay token for it.
// ok is false for references that are not http(s) and so are never relayed.
func (r *hlsRelay) relayReference(baseURL *url.URL, origin, reference, route string) (string, bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil {
		return "", false, err
	}
	target := baseURL.ResolveReference(parsed)
	if upstreamhttp.CheckScheme(target.Scheme) != nil {
		return "", false, nil
	}
	token, err := r.register(target.String(), origin)
	if err != nil {
		return "", false, err
	}
	return route + "?relay_token=" + url.QueryEscape(token), true, nil
}

// imageURL relays plain-http channel logos so HTTPS pages avoid mixed content.
// Logo URLs come from provider metadata, so no origin is trusted beyond the
// configured provider hosts.
func (r *hlsRelay) imageURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "http" {
		return rawURL
	}
	token, err := r.register(parsed.String(), "")
	if err != nil {
		return rawURL
	}
	return "xtream/image?relay_token=" + url.QueryEscape(token)
}

// relayConfiguredHosts lists the admin-configured provider hosts across every
// source and its playback account pool (sub-accounts share the source host).
// They may be LAN addresses, so the relay's SSRF guard lets them through.
func relayConfiguredHosts(settings config.Settings) []string {
	raw := []string{settings.XtreamBaseURL, settings.M3UURL, settings.DispatcharrURL}
	for _, source := range settings.XtreamSources {
		raw = append(raw, source.BaseURL)
	}
	for _, source := range settings.EffectiveXtreamSources() {
		raw = append(raw, source.BaseURL)
	}
	hosts := make([]string, 0, len(raw))
	for _, value := range raw {
		if host := urlHost(value); host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func (s *HTTPRoutesServer) relayConfiguredHosts() []string {
	if s.settingsProvider == nil {
		return nil
	}
	return relayConfiguredHosts(s.settingsProvider())
}

func urlHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// streamURLExposesSecrets errs on the side of caution: a false positive costs
// a relay, a false negative hands provider credentials to the browser.
func streamURLExposesSecrets(streamURL string, settings config.Settings) bool {
	parsed, err := url.Parse(streamURL)
	if err != nil {
		return true
	}
	if parsed.User != nil {
		return true
	}
	if xtreamCredentialPath.MatchString(parsed.Path) || xtreamStylePath.MatchString(parsed.Path) {
		return true
	}
	query := parsed.Query()
	for key, values := range query {
		normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(key)), "-", "_")
		if sensitiveQueryKeys[normalized] && strings.TrimSpace(strings.Join(values, "")) != "" {
			return true
		}
	}
	if strings.TrimSpace(query.Get("username")) != "" && strings.TrimSpace(query.Get("password")) != "" {
		return true
	}
	segments := strings.Split(parsed.Path, "/")
	for _, secret := range configuredStreamSecrets(settings) {
		for _, segment := range segments {
			if segment == secret {
				return true
			}
		}
		for _, values := range query {
			for _, value := range values {
				if value == secret {
					return true
				}
			}
		}
		// Substring matches on very short secrets would flag unrelated URLs.
		if len(secret) < 6 {
			continue
		}
		for _, form := range []string{secret, url.PathEscape(secret), url.QueryEscape(secret)} {
			if strings.Contains(streamURL, form) {
				return true
			}
		}
	}
	return false
}

// configuredStreamSecrets collects every provider password in the account
// pool, including playback sub-accounts.
func configuredStreamSecrets(settings config.Settings) []string {
	values := []string{settings.XtreamPassword, settings.DispatcharrPass, settings.DispatcharrAPIKey}
	for _, source := range settings.XtreamSources {
		values = append(values, source.Password)
		for _, account := range source.Accounts {
			values = append(values, account.Password)
		}
	}
	var secrets []string
	seen := map[string]bool{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !seen[value] {
			seen[value] = true
			secrets = append(secrets, value)
		}
	}
	return secrets
}

// streamRequiresHLSCode is the 422 error code for credentialed streams that
// cannot be relayed while direct provider URLs are disabled.
const (
	streamRequiresHLSCode    = "stream_requires_hls"
	streamRequiresHLSMessage = "This stream would send the provider account credentials to your browser, and Silo can only relay HLS streams. " +
		"Ask a Silo admin to switch the source to the m3u8 live format or turn on \"Allow direct provider URLs\" in XC Admin."
)

// allowDirectProviderURLsKey is the admin setting that lets non-HLS
// credentialed streams (TS live, VOD, series, catch-up) redirect the browser
// to the provider. It defaults to true so existing installs keep playing.
const allowDirectProviderURLsKey = "allowDirectProviderURLs"

func (s *HTTPRoutesServer) directProviderURLsAllowed() bool {
	return s.adminFlag(allowDirectProviderURLsKey, true)
}

// serveProviderStream answers a resolved playback URL. Credential-free URLs
// are redirected so the browser streams straight from the provider. URLs that
// embed provider credentials are relayed when they are HLS, so the browser
// only ever sees relay tokens. Continuous MPEG-TS and VOD files cannot pass
// through the buffered plugin route SDK: they redirect while the admin
// "Allow direct provider URLs" setting is on (the default) and are refused
// with 422 stream_requires_hls when it is off.
func (s *HTTPRoutesServer) serveProviderStream(ctx context.Context, streamURL string, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	settings := config.Settings{}
	if s.settingsProvider != nil {
		settings = s.settingsProvider()
	}
	if !streamURLExposesSecrets(streamURL, settings) {
		return redirectResponse(streamURL), nil
	}
	if publicStreamFormat(streamURL) == "hls" {
		return s.relay.start(ctx, streamURL, relayConfiguredHosts(settings), request)
	}
	if !s.directProviderURLsAllowed() {
		return s.respondJSON(http.StatusUnprocessableEntity, map[string]any{
			"ok":    false,
			"code":  streamRequiresHLSCode,
			"error": streamRequiresHLSMessage,
		})
	}
	return redirectResponse(streamURL), nil
}
