package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

const (
	defaultSportsImageCacheDir  = "/var/lib/continuum/plugins/silo.ramindex.xtream/sports-images"
	sportsImagePositiveTTL      = 14 * 24 * time.Hour
	sportsImageNegativeTTL      = 24 * time.Hour
	sportsImageMaxBytes         = 8 << 20
	sportsImageMaxFiles         = 1500
	sportsImageURLRegistryLimit = 3000
)

type sportsImageSource struct {
	URL        string
	AccessedAt time.Time
}

type sportsImageCache struct {
	dir    string
	client *http.Client
	mu     sync.RWMutex
	urls   map[string]sportsImageSource
	locks  map[string]*sportsImageLock
}

type sportsImageLock struct {
	mu   sync.Mutex
	refs int
}

func newSportsImageCache(dir string, client *http.Client) *sportsImageCache {
	return &sportsImageCache{dir: dir, client: client, urls: map[string]sportsImageSource{}, locks: map[string]*sportsImageLock{}}
}

func secureSportsImageHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	// This client deliberately ignores environment proxies. The public-IP dial
	// guard must validate the artwork destination itself, not an intermediary.
	transport := &http.Transport{Proxy: nil, ForceAttemptHTTP2: true}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, candidate := range addresses {
			if !sportsImagePublicIP(candidate.IP) {
				continue
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
		}
		return nil, errors.New("sports image host resolved only to private addresses")
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many sports image redirects")
			}
			return nil
		},
	}
}

func sportsImagePublicIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range sportsImageBlockedPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

var sportsImageBlockedPrefixes = func() []netip.Prefix {
	values := []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
		"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"::/128", "::1/128", "64:ff9b:1::/48", "100::/64", "2001:db8::/32", "fc00::/7", "fe80::/10",
	}
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefixes = append(prefixes, netip.MustParsePrefix(value))
	}
	return prefixes
}()

var sportsSVGLocalURLPattern = regexp.MustCompile(`(?i)^url\(\s*['"]?#[a-z0-9_.:-]+['"]?\s*\)$`)

func (cache *sportsImageCache) register(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return rawURL
	}
	sum := sha256.Sum256([]byte(rawURL))
	key := hex.EncodeToString(sum[:16])
	cache.mu.Lock()
	if _, exists := cache.urls[key]; !exists && len(cache.urls) >= sportsImageURLRegistryLimit {
		cache.evictOldestSourceLocked()
	}
	cache.urls[key] = sportsImageSource{URL: rawURL, AccessedAt: time.Now()}
	cache.mu.Unlock()
	return "/xtream/api/sports/image?key=" + key
}

func (cache *sportsImageCache) source(key string) string {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	source := cache.urls[key]
	if source.URL != "" {
		source.AccessedAt = time.Now()
		cache.urls[key] = source
	}
	return source.URL
}

func (cache *sportsImageCache) evictOldestSourceLocked() {
	oldestKey := ""
	var oldestTime time.Time
	for key, source := range cache.urls {
		if oldestKey == "" || source.AccessedAt.Before(oldestTime) {
			oldestKey, oldestTime = key, source.AccessedAt
		}
	}
	delete(cache.urls, oldestKey)
}

func (s *HTTPRoutesServer) proxySportsEventImages(events []SportsEvent) []SportsEvent {
	if s.sportsImages == nil {
		return events
	}
	proxied := cloneSportsEvents(events)
	for index := range proxied {
		event := &proxied[index]
		event.ImageURL = s.sportsImages.register(event.ImageURL)
		for _, logo := range []*string{&event.LeagueLogoURL, &event.Home.LogoURL, &event.Away.LogoURL, &event.LeagueLogoFallbackURL, &event.Home.LogoFallbackURL, &event.Away.LogoFallbackURL} {
			if *logo == ceblLeagueLogoURL || *logo == fibaWomensLeagueLogoURL || *logo == iccLeagueLogoURL || isCEBLTeamLogo(*logo) || strings.HasPrefix(*logo, lplLogoBaseURL) {
				continue
			}
			if strings.HasPrefix(*logo, "https://sportarr.net/static/images/team/") || strings.HasPrefix(*logo, "https://sportarr.net/static/images/league/") || strings.HasPrefix(*logo, "https://r2.thesportsdb.com/images/media/team/badge/") {
				continue
			}
			// These public identity assets already load directly in guide results.
			// Keep the same URLs when provider scores arrive.
			if strings.HasPrefix(*logo, ncaaTeamLogoBase) || strings.HasPrefix(*logo, "https://a.espncdn.com/i/teamlogos/mlb/") || strings.HasPrefix(*logo, "https://flagcdn.com/") || strings.HasPrefix(*logo, "https://irp.cdn-website.com/ffc1e51d/dms3rep/multi/opt/BramptonHoneyBadgers_") || strings.HasPrefix(*logo, gameThumbsPublicBaseURL+"/") || *logo == ncaaLeagueLogoURL || *logo == formulaOneLeagueLogoURL || *logo == formulaELeagueLogoURL {
				continue
			}
			*logo = s.sportsImages.register(*logo)
		}
		if event.Artwork != nil {
			artwork := *event.Artwork
			for _, image := range []*SportsImage{artwork.Poster, artwork.Backdrop, artwork.Logo, artwork.Banner, artwork.Thumbnail} {
				if image != nil {
					image.URL = s.sportsImages.register(image.URL)
				}
			}
			event.Artwork = &artwork
		}
	}
	return proxied
}

func (s *HTTPRoutesServer) handleSportsImage(ctx context.Context, request *pluginv1.HandleHTTPRequest) *pluginv1.HandleHTTPResponse {
	if request.GetMethod() != "" && request.GetMethod() != http.MethodGet {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed")
	}
	if s.sportsImages == nil {
		return textResponse(http.StatusNotFound, "sports image cache unavailable")
	}
	key := sportsImageKeyFromRequest(request)
	if len(key) != 32 || strings.Trim(key, "0123456789abcdef") != "" {
		return textResponse(http.StatusNotFound, "sports image not found")
	}
	payload, contentType, status := s.sportsImages.load(ctx, key)
	if status != http.StatusOK {
		return textResponse(status, "sports image unavailable")
	}
	return &pluginv1.HandleHTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"cache-control":           "public, max-age=86400, stale-if-error=604800",
			"content-security-policy": "sandbox; default-src 'none'; style-src 'unsafe-inline'",
			"content-type":            contentType,
			"x-content-type-options":  "nosniff",
		},
		Body: payload,
	}
}

func sportsImageKeyFromRequest(request *pluginv1.HandleHTTPRequest) string {
	if request == nil {
		return ""
	}
	if key := strings.TrimSpace(queryValue(request, "key")); key != "" {
		return key
	}
	path := request.GetPath()
	if rawPath, rawQuery, ok := strings.Cut(path, "?"); ok {
		values, err := url.ParseQuery(rawQuery)
		if err == nil {
			if key := strings.TrimSpace(values.Get("key")); key != "" {
				return key
			}
		}
		path = rawPath
	}
	return strings.TrimPrefix(path, "/xtream/api/sports/image/")
}

func (cache *sportsImageCache) load(ctx context.Context, key string) ([]byte, string, int) {
	if payload, contentType, fresh := cache.readFresh(key); fresh {
		return payload, contentType, http.StatusOK
	}
	if cache.negativeFresh(key) {
		return nil, "", http.StatusNotFound
	}
	if cache.retryCooling(key) {
		if stale, staleType, ok := cache.readAny(key); ok {
			return stale, staleType, http.StatusOK
		}
		return nil, "", http.StatusTooManyRequests
	}
	release := cache.acquireKeyLock(key)
	defer release()
	if payload, contentType, fresh := cache.readFresh(key); fresh {
		return payload, contentType, http.StatusOK
	}
	if cache.retryCooling(key) {
		if stale, staleType, ok := cache.readAny(key); ok {
			return stale, staleType, http.StatusOK
		}
		return nil, "", http.StatusTooManyRequests
	}
	source := cache.source(key)
	if source == "" {
		return nil, "", http.StatusNotFound
	}
	payload, contentType, status, retryAfter := cache.fetch(ctx, source)
	if status == http.StatusOK {
		if err := cache.writeAtomic(key, payload); err == nil {
			_ = os.Remove(cache.missPath(key))
			_ = os.Remove(cache.retryPath(key))
			cache.cleanup()
		}
		return payload, contentType, http.StatusOK
	}
	if status == http.StatusNotFound || status == http.StatusGone {
		_ = cache.writeAtomic(key+".miss", []byte("missing"))
		cache.cleanup()
	}
	if status == http.StatusTooManyRequests {
		if retryAfter <= 0 {
			retryAfter = 30 * time.Second
		}
		expires := strconv.FormatInt(time.Now().Add(retryAfter).Unix(), 10)
		_ = cache.writeAtomic(key+".retry", []byte(expires))
		cache.cleanup()
	}
	if stale, staleType, ok := cache.readAny(key); ok {
		return stale, staleType, http.StatusOK
	}
	return nil, "", status
}

func (cache *sportsImageCache) acquireKeyLock(key string) func() {
	cache.mu.Lock()
	lock := cache.locks[key]
	if lock == nil {
		lock = &sportsImageLock{}
		cache.locks[key] = lock
	}
	lock.refs++
	cache.mu.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		cache.mu.Lock()
		lock.refs--
		if lock.refs == 0 && cache.locks[key] == lock {
			delete(cache.locks, key)
		}
		cache.mu.Unlock()
	}
}

func (cache *sportsImageCache) fetch(ctx context.Context, source string) ([]byte, string, int, time.Duration) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, "", http.StatusBadGateway, 0
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif;q=0.9,*/*;q=0.1")
	response, err := cache.client.Do(req)
	if err != nil {
		return nil, "", http.StatusBadGateway, 0
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", response.StatusCode, sportsRetryAfter(response.Header.Get("Retry-After"))
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, sportsImageMaxBytes+1))
	if err != nil || len(payload) == 0 || len(payload) > sportsImageMaxBytes {
		return nil, "", http.StatusBadGateway, 0
	}
	contentType := sportsImageContentType(payload)
	if contentType == "" {
		return nil, "", http.StatusUnsupportedMediaType, 0
	}
	return payload, contentType, http.StatusOK, 0
}

func sportsRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		if seconds > 3600 {
			seconds = 3600
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		duration := time.Until(when)
		if duration > time.Hour {
			return time.Hour
		}
		if duration > 0 {
			return duration
		}
	}
	return 0
}

func sportsImageContentType(payload []byte) string {
	switch {
	case len(payload) >= 8 && string(payload[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(payload) >= 3 && payload[0] == 0xff && payload[1] == 0xd8 && payload[2] == 0xff:
		return "image/jpeg"
	case len(payload) >= 6 && (string(payload[:6]) == "GIF87a" || string(payload[:6]) == "GIF89a"):
		return "image/gif"
	case len(payload) >= 12 && string(payload[:4]) == "RIFF" && string(payload[8:12]) == "WEBP":
		return "image/webp"
	case len(payload) >= 12 && string(payload[4:8]) == "ftyp" && (string(payload[8:12]) == "avif" || string(payload[8:12]) == "avis"):
		return "image/avif"
	case sportsSafeSVG(payload):
		return "image/svg+xml"
	default:
		return ""
	}
}

func sportsSafeSVG(payload []byte) bool {
	decoder := xml.NewDecoder(strings.NewReader(string(payload)))
	rootSeen := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return rootSeen
		}
		if err != nil {
			return false
		}
		switch value := token.(type) {
		case xml.StartElement:
			name := strings.ToLower(value.Name.Local)
			if !rootSeen {
				if name != "svg" {
					return false
				}
				rootSeen = true
			}
			switch name {
			case "script", "foreignobject", "iframe", "object", "embed", "audio", "video", "image", "style":
				return false
			}
			for _, attribute := range value.Attr {
				attributeName := strings.ToLower(attribute.Name.Local)
				attributeValue := strings.TrimSpace(strings.ToLower(attribute.Value))
				if attributeName == "base" && attribute.Name.Space != "" {
					return false
				}
				if strings.HasPrefix(attributeName, "on") || strings.Contains(attributeValue, "javascript:") || strings.Contains(attributeValue, "data:text/html") {
					return false
				}
				if (attributeName == "href" || attributeName == "src") && attributeValue != "" && !strings.HasPrefix(attributeValue, "#") {
					return false
				}
				if strings.Contains(attributeValue, "url(") && !sportsSVGLocalURLPattern.MatchString(attributeValue) {
					return false
				}
				if attributeName == "style" && strings.Contains(attributeValue, "expression(") {
					return false
				}
			}
		case xml.Directive, xml.ProcInst:
			return false
		}
	}
}

func (cache *sportsImageCache) readFresh(key string) ([]byte, string, bool) {
	info, err := os.Stat(cache.imagePath(key))
	if err != nil || time.Since(info.ModTime()) > sportsImagePositiveTTL {
		return nil, "", false
	}
	return cache.readAny(key)
}

func (cache *sportsImageCache) readAny(key string) ([]byte, string, bool) {
	payload, err := os.ReadFile(cache.imagePath(key))
	if err != nil {
		return nil, "", false
	}
	contentType := sportsImageContentType(payload)
	if contentType == "" {
		_ = os.Remove(cache.imagePath(key))
		return nil, "", false
	}
	return payload, contentType, true
}

func (cache *sportsImageCache) negativeFresh(key string) bool {
	info, err := os.Stat(cache.missPath(key))
	if err != nil {
		return false
	}
	if time.Since(info.ModTime()) <= sportsImageNegativeTTL {
		return true
	}
	_ = os.Remove(cache.missPath(key))
	return false
}

func (cache *sportsImageCache) retryCooling(key string) bool {
	payload, err := os.ReadFile(cache.retryPath(key))
	if err != nil {
		return false
	}
	expiresUnix, err := strconv.ParseInt(strings.TrimSpace(string(payload)), 10, 64)
	if err == nil && time.Now().Unix() < expiresUnix {
		return true
	}
	_ = os.Remove(cache.retryPath(key))
	return false
}

func (cache *sportsImageCache) writeAtomic(name string, payload []byte) error {
	if err := os.MkdirAll(cache.dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(cache.dir, ".sports-image-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err = temporary.Write(payload); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Chmod(temporaryName, 0o600); err != nil {
		return err
	}
	return os.Rename(temporaryName, filepath.Join(cache.dir, name))
}

func (cache *sportsImageCache) cleanup() {
	entries, err := os.ReadDir(cache.dir)
	if err != nil || len(entries) <= sportsImageMaxFiles {
		return
	}
	type cachedFile struct {
		path string
		mod  time.Time
	}
	files := make([]cachedFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".sports-image-") {
			continue
		}
		if info, infoErr := entry.Info(); infoErr == nil {
			files = append(files, cachedFile{path: filepath.Join(cache.dir, entry.Name()), mod: info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for len(files) > sportsImageMaxFiles {
		_ = os.Remove(files[0].path)
		files = files[1:]
	}
}

func (cache *sportsImageCache) imagePath(key string) string { return filepath.Join(cache.dir, key) }
func (cache *sportsImageCache) missPath(key string) string {
	return filepath.Join(cache.dir, key+".miss")
}
func (cache *sportsImageCache) retryPath(key string) string {
	return filepath.Join(cache.dir, key+".retry")
}
