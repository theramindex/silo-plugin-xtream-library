package plugin

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"google.golang.org/protobuf/types/known/structpb"
)

func relayLines(body []byte) []string {
	var tokens []string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "stream?relay_token=") {
			tokens = append(tokens, line)
		}
	}
	return tokens
}

func relayFollow(t *testing.T, server *HTTPRoutesServer, line string) *pluginv1.HandleHTTPResponse {
	t.Helper()
	parsed, err := url.Parse(line)
	if err != nil {
		t.Fatalf("parse relay line %q: %v", line, err)
	}
	query, _ := structpb.NewStruct(map[string]any{"relay_token": parsed.Query().Get("relay_token")})
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/xtream/stream", Query: query})
	if err != nil {
		t.Fatalf("relay follow: %v", err)
	}
	return response
}

func TestHLSRelayRefusesManifestReferencesToUnconfiguredInternalHosts(t *testing.T) {
	t.Parallel()

	var internalHits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHits.Add(1)
		_, _ = w.Write([]byte("internal-admin-data"))
	}))
	defer internal.Close()
	// "localhost" resolves to loopback but is not a configured provider host.
	internalViaName := strings.Replace(internal.URL, "127.0.0.1", "localhost", 1)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/live/demo/secret/1001.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"%s/key\"\n#EXTINF:2,\nsegment.ts\n#EXTINF:2,\n%s/steal.ts\n#EXTINF:2,\nfile:///etc/passwd\n", internalViaName, internalViaName)
		case "/live/demo/secret/segment.ts":
			_, _ = w.Write([]byte("transport-stream"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{Source: model.LiveTVSource(model.SourceModeXtream), Channels: []model.Channel{{ID: "xtream:1001", Name: "News HD"}}}})
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings {
		return config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: upstream.URL, XtreamUsername: "demo", XtreamPassword: "secret", XtreamLiveFormat: "m3u8", ChannelRefreshH: 24, EPGRefreshH: 24}
	})
	server.relay.keyPath = filepath.Join(t.TempDir(), "relay.key")
	query, _ := structpb.NewStruct(map[string]any{"channel_id": "xtream:1001"})
	manifest, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/xtream/stream", Query: query})
	if err != nil || manifest.GetStatusCode() != http.StatusOK {
		t.Fatalf("relay manifest: status=%d err=%v", manifest.GetStatusCode(), err)
	}
	body := string(manifest.GetBody())
	if strings.Contains(body, "file://") {
		t.Fatalf("non-http media URI must be dropped, got %q", body)
	}
	lines := relayLines(manifest.GetBody())
	if len(lines) != 2 {
		t.Fatalf("expected two relayed media lines, got %d in %q", len(lines), body)
	}
	if segment := relayFollow(t, server, lines[0]); segment.GetStatusCode() != http.StatusOK || string(segment.GetBody()) != "transport-stream" {
		t.Fatalf("configured-host segment should relay, got %d %q", segment.GetStatusCode(), segment.GetBody())
	}
	if stolen := relayFollow(t, server, lines[1]); stolen.GetStatusCode() != http.StatusBadGateway {
		t.Fatalf("internal reference must be refused, got %d %q", stolen.GetStatusCode(), stolen.GetBody())
	}
	keyStart := strings.Index(body, `URI="`)
	keyLine := body[keyStart+len(`URI="`):]
	keyLine = keyLine[:strings.Index(keyLine, `"`)]
	if key := relayFollow(t, server, keyLine); key.GetStatusCode() != http.StatusBadGateway {
		t.Fatalf("internal key URI must be refused, got %d", key.GetStatusCode())
	}
	if internalHits.Load() != 0 {
		t.Fatalf("internal server was reached %d times", internalHits.Load())
	}
}

func TestHLSRelayTrustsSealedOriginHostAcrossProcesses(t *testing.T) {
	t.Parallel()

	// An M3U channel on a LAN host that is not a configured source host: the
	// catalog URL is trusted, so its host is sealed into every token.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/news.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:2,\nseg-1.ts\n"))
		case "/seg-1.ts":
			_, _ = w.Write([]byte("lan-segment"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	streamURL := upstream.URL + "/news.m3u8?token=lan-secret-token"
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{Source: model.LiveTVSource(model.SourceModeM3UXMLTV), Channels: []model.Channel{{ID: "m3u:news", Name: "News", StreamURL: streamURL}}}})
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings {
		return config.Settings{SourceMode: config.SourceModeM3UXMLTV, M3UURL: "https://playlists.example.com/list.m3u", EPGXMLURL: "https://playlists.example.com/guide.xml", ChannelRefreshH: 24, EPGRefreshH: 24}
	})
	keyPath := filepath.Join(t.TempDir(), "relay.key")
	server.relay.keyPath = keyPath
	query, _ := structpb.NewStruct(map[string]any{"channel_id": "m3u:news"})
	manifest, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/xtream/stream", Query: query})
	if err != nil || manifest.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected credentialed M3U HLS to be relayed, got status=%d headers=%v err=%v", manifest.GetStatusCode(), manifest.GetHeaders(), err)
	}
	if strings.Contains(string(manifest.GetBody()), "lan-secret-token") || manifest.GetHeaders()["location"] != "" {
		t.Fatalf("relay exposed the credentialed URL: %v %q", manifest.GetHeaders(), manifest.GetBody())
	}
	lines := relayLines(manifest.GetBody())
	if len(lines) != 1 {
		t.Fatalf("expected one relayed segment, got %q", manifest.GetBody())
	}
	server.relay = newHLSRelay()
	server.relay.keyPath = keyPath
	if segment := relayFollow(t, server, lines[0]); segment.GetStatusCode() != http.StatusOK || string(segment.GetBody()) != "lan-segment" {
		t.Fatalf("sealed origin should allow same-host segment, got %d %q", segment.GetStatusCode(), segment.GetBody())
	}
}

func TestHLSRelayLegacyTokensDecodeWithoutOrigin(t *testing.T) {
	t.Parallel()

	relay := newHLSRelay()
	relay.keyPath = filepath.Join(t.TempDir(), "relay.key")
	key, err := relay.signingKey()
	if err != nil {
		t.Fatalf("signing key: %v", err)
	}
	block, _ := aes.NewCipher(key[:32])
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	_, _ = rand.Read(nonce)
	payload := fmt.Sprintf("%d\n%s", time.Now().Add(time.Minute).Unix(), "https://cdn.example.com/a.ts")
	legacy := base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(payload), nil))

	target, origin, ok := relay.target(legacy)
	if !ok || target != "https://cdn.example.com/a.ts" || origin != "" {
		t.Fatalf("legacy token: target=%q origin=%q ok=%v", target, origin, ok)
	}
	token, err := relay.register("https://cdn.example.com/b.ts", "lan.example")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if target, origin, ok := relay.target(token); !ok || target != "https://cdn.example.com/b.ts" || origin != "lan.example" {
		t.Fatalf("new token: target=%q origin=%q ok=%v", target, origin, ok)
	}
}

func TestStreamURLExposesSecrets(t *testing.T) {
	t.Parallel()

	settings := config.Settings{
		XtreamSources: []config.XtreamSource{{
			ID: "primary", BaseURL: "https://provider.example.com", Username: "demo", Password: "catalogpass", Enabled: true,
			Accounts: []config.XtreamAccount{{ID: "sub", Username: "viewer", Password: "subaccount-pass", Enabled: true, Compatible: true}},
		}},
	}
	cases := map[string]bool{
		"https://cdn.example.com/live/news.m3u8":                         false,
		"https://cdn.example.com/live/demo/catalogpass/1001.ts":          true,
		"https://cdn.example.com/timeshift/u/p/60/2026-01-01:00-00/1.ts": true,
		"https://cdn.example.com/viewer/x/1001.ts":                       true,
		"https://user:pw@cdn.example.com/a.m3u8":                         true,
		"https://cdn.example.com/a.m3u8?Auth-Token=abc":                  true,
		"https://cdn.example.com/a.m3u8?token=":                          false,
		"https://cdn.example.com/a.m3u8?username=u&password=p":           true,
		"https://cdn.example.com/hls/subaccount-pass/index.m3u8":         true,
		"https://cdn.example.com/index.m3u8?sid=xsubaccount-passx":       true,
		"https://cdn.example.com/index.m3u8?sid=abc":                     false,
		"://bad": true,
	}
	for raw, want := range cases {
		if got := streamURLExposesSecrets(raw, settings); got != want {
			t.Errorf("streamURLExposesSecrets(%q) = %v, want %v", raw, got, want)
		}
	}
}

func directURLTestServer(t *testing.T, allowDirect *bool, baseURL string) *HTTPRoutesServer {
	t.Helper()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeXtream),
		Channels: []model.Channel{{ID: "xtream:1001", Name: "News HD", Catchup: true, CatchupMins: 120}},
		Content:  model.ContentState{VODItems: []model.VODItem{{ID: "vod:2001", Name: "Movie", Container: "mp4"}}},
	}})
	if allowDirect != nil {
		store.SetAdminSettings([]byte(fmt.Sprintf(`{"allowDirectProviderURLs":%t}`, *allowDirect)))
	}
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings {
		return config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: baseURL, XtreamUsername: "demo", XtreamPassword: "secret", XtreamLiveFormat: "ts", ChannelRefreshH: 24, EPGRefreshH: 24}
	})
	server.relay.keyPath = filepath.Join(t.TempDir(), "relay.key")
	return server
}

func directURLRouteRequests() map[string]*pluginv1.HandleHTTPRequest {
	live, _ := structpb.NewStruct(map[string]any{"channel_id": "xtream:1001"})
	vod, _ := structpb.NewStruct(map[string]any{"item_id": "vod:2001"})
	catchup, _ := structpb.NewStruct(map[string]any{"channel_id": "xtream:1001", "start_unix": "1767225600", "end_unix": "1767229200"})
	return map[string]*pluginv1.HandleHTTPRequest{
		"ts live": {Method: http.MethodGet, Path: "/xtream/stream", Query: live},
		"vod":     {Method: http.MethodGet, Path: "/dispatcharr/vod/stream", Query: vod},
		"catchup": {Method: http.MethodGet, Path: "/dispatcharr/catchup/stream", Query: catchup},
	}
}

func TestDirectProviderURLsSettingGatesCredentialedNonHLSStreams(t *testing.T) {
	t.Parallel()

	on, off := true, false
	for name, allow := range map[string]*bool{"default": nil, "on": &on, "off": &off} {
		server := directURLTestServer(t, allow, "https://provider.example.com")
		for kind, request := range directURLRouteRequests() {
			response, err := server.Handle(context.Background(), request)
			if err != nil {
				t.Fatalf("%s/%s: %v", name, kind, err)
			}
			if allow == nil || *allow {
				if response.GetStatusCode() != http.StatusFound || !strings.Contains(response.GetHeaders()["location"], "/demo/secret/") {
					t.Fatalf("%s/%s: expected credentialed redirect, got %d %v", name, kind, response.GetStatusCode(), response.GetHeaders())
				}
				continue
			}
			if response.GetStatusCode() != http.StatusUnprocessableEntity || response.GetHeaders()["location"] != "" {
				t.Fatalf("%s/%s: expected 422, got %d %v", name, kind, response.GetStatusCode(), response.GetHeaders())
			}
			body := string(response.GetBody())
			if strings.Contains(body, "secret") || !strings.Contains(body, `"code":"stream_requires_hls"`) || !strings.Contains(body, `"ok":false`) {
				t.Fatalf("%s/%s: unexpected 422 body %s", name, kind, body)
			}
		}
		// Episodes resolve through get_series_info; the final step is shared.
		episode, err := server.serveProviderStream(context.Background(), "https://provider.example.com/series/demo/secret/3001.mkv", &pluginv1.HandleHTTPRequest{Method: http.MethodGet})
		if err != nil {
			t.Fatalf("%s/episode: %v", name, err)
		}
		want := http.StatusFound
		if allow != nil && !*allow {
			want = http.StatusUnprocessableEntity
		}
		if episode.GetStatusCode() != int32(want) {
			t.Fatalf("%s/episode: expected %d, got %d", name, want, episode.GetStatusCode())
		}
	}
}

func TestDirectProviderURLsSettingNeverBlocksHLSOrCredentialFreeURLs(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:2,\nseg.ts\n"))
	}))
	defer upstream.Close()

	off := false
	server := directURLTestServer(t, &off, upstream.URL)
	request := &pluginv1.HandleHTTPRequest{Method: http.MethodGet}
	hls, err := server.serveProviderStream(context.Background(), upstream.URL+"/movie/demo/secret/2001.m3u8", request)
	if err != nil || hls.GetStatusCode() != http.StatusOK || len(relayLines(hls.GetBody())) != 1 {
		t.Fatalf("credentialed HLS must be relayed with the setting off, got %d %q %v", hls.GetStatusCode(), hls.GetBody(), err)
	}
	open, err := server.serveProviderStream(context.Background(), "https://cdn.example.com/public/movie.mp4", request)
	if err != nil || open.GetStatusCode() != http.StatusFound {
		t.Fatalf("credential-free URL should redirect, got %d %v", open.GetStatusCode(), err)
	}
}

func TestDirectProviderURLsSettingDefaultsOn(t *testing.T) {
	t.Parallel()

	if got := normalizeAdminSettingsPayload(map[string]any{})[allowDirectProviderURLsKey]; got != true {
		t.Fatalf("expected default true, got %#v", got)
	}
	if got := normalizeAdminSettingsPayload(map[string]any{allowDirectProviderURLsKey: false})[allowDirectProviderURLsKey]; got != false {
		t.Fatalf("expected explicit false to persist, got %#v", got)
	}
	if !NewHTTPRoutesServer(cache.NewStore()).directProviderURLsAllowed() {
		t.Fatal("expected direct provider URLs to be allowed without saved settings")
	}
}
