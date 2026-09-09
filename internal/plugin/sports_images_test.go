package plugin

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

type sportsImageRoundTripFunc func(*http.Request) (*http.Response, error)

func (function sportsImageRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func sportsImageTestClient(handler func() (int, []byte)) *http.Client {
	return &http.Client{Transport: sportsImageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		status, payload := handler()
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(payload)), Header: make(http.Header)}, nil
	})}
}

func TestSportsImageCacheValidatesAndCachesRasterArtwork(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	client := sportsImageTestClient(func() (int, []byte) {
		requests.Add(1)
		return http.StatusOK, append([]byte("\x89PNG\r\n\x1a\n"), []byte("fixture")...)
	})

	cache := newSportsImageCache(t.TempDir(), client)
	path := cache.register("https://images.example/team.png")
	key := strings.TrimPrefix(path, "/xtream/api/sports/image?key=")
	for attempt := 0; attempt < 2; attempt++ {
		payload, contentType, status := cache.load(context.Background(), key)
		if status != http.StatusOK || contentType != "image/png" || len(payload) == 0 {
			t.Fatalf("load attempt %d = status %d, type %q, bytes %d", attempt, status, contentType, len(payload))
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("upstream requests = %d, want 1", got)
	}
}

func TestSportsImageCacheNegativeCachesMissingAndCoolsDownRateLimits(t *testing.T) {
	t.Parallel()

	var missingRequests atomic.Int32
	missingClient := sportsImageTestClient(func() (int, []byte) {
		missingRequests.Add(1)
		return http.StatusNotFound, nil
	})
	cache := newSportsImageCache(t.TempDir(), missingClient)
	key := strings.TrimPrefix(cache.register("https://images.example/missing.png"), "/xtream/api/sports/image?key=")
	for attempt := 0; attempt < 2; attempt++ {
		if _, _, status := cache.load(context.Background(), key); status != http.StatusNotFound {
			t.Fatalf("missing status = %d, want 404", status)
		}
	}
	if got := missingRequests.Load(); got != 1 {
		t.Fatalf("missing upstream requests = %d, want 1", got)
	}

	var limitedRequests atomic.Int32
	limitedClient := sportsImageTestClient(func() (int, []byte) {
		limitedRequests.Add(1)
		return http.StatusTooManyRequests, nil
	})
	limitedCache := newSportsImageCache(t.TempDir(), limitedClient)
	limitedKey := strings.TrimPrefix(limitedCache.register("https://images.example/limited.png"), "/xtream/api/sports/image?key=")
	for attempt := 0; attempt < 2; attempt++ {
		if _, _, status := limitedCache.load(context.Background(), limitedKey); status != http.StatusTooManyRequests {
			t.Fatalf("rate-limit status = %d, want 429", status)
		}
	}
	if got := limitedRequests.Load(); got != 1 {
		t.Fatalf("rate-limit upstream requests = %d, want 1 during cooldown", got)
	}
	limitedCache.mu.RLock()
	activeLocks := len(limitedCache.locks)
	limitedCache.mu.RUnlock()
	if activeLocks != 0 {
		t.Fatalf("key locks retained after loads = %d, want 0", activeLocks)
	}
}

func TestSportsImageRouteRejectsNonImagesAndProxiesEventArtwork(t *testing.T) {
	t.Parallel()

	client := sportsImageTestClient(func() (int, []byte) { return http.StatusOK, []byte("<html>not an image</html>") })
	server := newEnabledSportsTestServer(nil)
	server.sportsImages = newSportsImageCache(t.TempDir(), client)
	events := server.proxySportsEventImages([]SportsEvent{{
		ImageURL: "https://images.example/event.png",
		Home:     SportsTeam{LogoURL: "https://images.example/home.png"},
		Away:     SportsTeam{LogoURL: "https://images.example/away.svg"},
	}})
	if !strings.HasPrefix(events[0].ImageURL, "/xtream/api/sports/image?key=") || !strings.HasPrefix(events[0].Home.LogoURL, "/xtream/api/sports/image?key=") {
		t.Fatalf("raster artwork was not proxied: %#v", events[0])
	}
	if got := events[0].Away.LogoURL; !strings.HasPrefix(got, "/xtream/api/sports/image?key=") {
		t.Fatalf("SVG URL = %q, want proxied URL", got)
	}
	response := server.handleSportsImage(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: events[0].ImageURL})
	if response.GetStatusCode() != http.StatusUnsupportedMediaType {
		t.Fatalf("non-image response status = %d, want 415", response.GetStatusCode())
	}
}

func TestSportsImagePublicIPRejectsSpecialUseNetworks(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"8.8.8.8": true, "2606:4700:4700::1111": true,
		"127.0.0.1": false, "10.0.0.1": false, "100.64.0.1": false,
		"169.254.1.1": false, "192.168.1.1": false, "198.18.0.1": false,
		"192.0.2.1": false, "224.0.0.1": false, "fc00::1": false, "fe80::1": false,
		"2001:db8::1": false,
	}
	for raw, want := range tests {
		if got := sportsImagePublicIP(net.ParseIP(raw)); got != want {
			t.Errorf("sportsImagePublicIP(%q) = %t, want %t", raw, got, want)
		}
	}
}

func TestSportsImageContentTypeAcceptsSafeSVGAndRejectsActiveContent(t *testing.T) {
	t.Parallel()

	safe := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path fill="#fff" d="M0 0h10v10z"/></svg>`)
	if got := sportsImageContentType(safe); got != "image/svg+xml" {
		t.Fatalf("safe SVG type = %q", got)
	}
	for _, unsafe := range [][]byte{
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><image href="https://tracker.example/x.png"/></svg>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path fill="url(https://tracker.example/a.svg#x)"/></svg>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path filter="url(https://tracker.example/f.svg#x)"/></svg>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" xml:base="https://tracker.example/"><use href="#x"/></svg>`),
	} {
		if got := sportsImageContentType(unsafe); got != "" {
			t.Fatalf("unsafe SVG type = %q, want empty", got)
		}
	}
	localReference := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="g"/></defs><path fill="url(#g)"/></svg>`)
	if got := sportsImageContentType(localReference); got != "image/svg+xml" {
		t.Fatalf("local SVG reference type = %q", got)
	}
}

func TestSportsImageSourceRegistryIsBounded(t *testing.T) {
	t.Parallel()

	cache := newSportsImageCache(t.TempDir(), sportsImageTestClient(func() (int, []byte) { return http.StatusNotFound, nil }))
	for index := 0; index < sportsImageURLRegistryLimit+25; index++ {
		cache.register("https://images.example/" + strconv.Itoa(index) + ".png")
	}
	cache.mu.RLock()
	count := len(cache.urls)
	cache.mu.RUnlock()
	if count != sportsImageURLRegistryLimit {
		t.Fatalf("source registry size = %d, want %d", count, sportsImageURLRegistryLimit)
	}
}
