package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"google.golang.org/protobuf/types/known/structpb"
)

func liveTVContractServer(mode model.SourceMode, channels []model.Channel, settings config.Settings) *HTTPRoutesServer {
	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{Source: model.LiveTVSource(mode), Channels: channels},
	})
	return NewHTTPRoutesServerWithSettings(store, func() config.Settings { return settings })
}

func liveTVContractGet(t *testing.T, server *HTTPRoutesServer, path string, query map[string]any) *pluginv1.HandleHTTPResponse {
	t.Helper()
	request := &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: path}
	if query != nil {
		parsed, err := structpb.NewStruct(query)
		if err != nil {
			t.Fatalf("new query struct: %v", err)
		}
		request.Query = parsed
	}
	response, err := server.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return response
}

func TestLiveTVContractInfo(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeXtream, []model.Channel{
		{ID: "xtream:1", Name: "News HD", StreamURL: "https://CDN.example/live/1.m3u8"},
		{ID: "xtream:2", Name: "Sports", StreamURL: "https://cdn.example/live/2.m3u8"},
		{ID: "xtream:3", Name: "No Stream"},
	}, config.Settings{
		SourceMode:     config.SourceModeXtream,
		DispatcharrURL: "http://dispatcharr.example:9191",
		M3UURL:         "not a url",
		XtreamSources: []config.XtreamSource{
			{ID: "primary", BaseURL: "http://Frost.example:8080", Username: "u", Password: "p", Enabled: true},
			{ID: "spare", BaseURL: "http://spare.example", Username: "u", Password: "p", Enabled: true},
			{ID: "off", BaseURL: "http://disabled.example", Username: "u", Password: "p", Enabled: false},
		},
	})

	response := liveTVContractGet(t, server, "/silo-compat/livetv/v1/info", nil)
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.GetStatusCode(), response.GetBody())
	}
	var info struct {
		ContractVersion int      `json:"contractVersion"`
		Name            string   `json:"name"`
		StreamHosts     []string `json:"streamHosts"`
	}
	if err := json.Unmarshal(response.GetBody(), &info); err != nil {
		t.Fatalf("decode info: %v", err)
	}
	if info.ContractVersion != 1 || info.Name != "XC for Silo" {
		t.Fatalf("unexpected info: %+v", info)
	}
	want := []string{"cdn.example", "dispatcharr.example:9191", "frost.example:8080", "spare.example"}
	if !reflect.DeepEqual(info.StreamHosts, want) {
		t.Fatalf("streamHosts = %v, want %v", info.StreamHosts, want)
	}
}

func TestLiveTVContractInfoIncludesLegacyXtreamHost(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeXtream, nil, config.Settings{
		SourceMode:     config.SourceModeXtream,
		XtreamBaseURL:  "https://legacy.example",
		XtreamUsername: "u",
		XtreamPassword: "p",
	})
	response := liveTVContractGet(t, server, "/silo-compat/livetv/v1/info", nil)
	var info livetvInfo
	if err := json.Unmarshal(response.GetBody(), &info); err != nil {
		t.Fatalf("decode info: %v", err)
	}
	if want := []string{"legacy.example"}; !reflect.DeepEqual(info.StreamHosts, want) {
		t.Fatalf("streamHosts = %v, want %v", info.StreamHosts, want)
	}
}

func TestLiveTVContractInfoEmptyHostsIsArray(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeXtream, nil, config.Settings{})
	response := liveTVContractGet(t, server, "/silo-compat/livetv/v1/info", nil)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(response.GetBody(), &raw); err != nil {
		t.Fatalf("decode info: %v", err)
	}
	if string(raw["streamHosts"]) != "[]" {
		t.Fatalf("streamHosts must be [] when nothing is configured, got %s", raw["streamHosts"])
	}
}

func TestLiveTVContractChannelsMatchesFixtureShape(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeXtream, []model.Channel{
		{ID: "xtream:42", Number: "101", Name: "News HD", LogoURL: "http://logos.example/news.png", CategoryID: "7", CategoryName: "News"},
		{ID: "xtream:frost:43", Number: "201", Name: "Sports One", LogoURL: "data:image/png;base64,AAAA", CategoryID: "frost:8", CategoryName: "Sports"},
	}, config.Settings{})

	response := liveTVContractGet(t, server, "/silo-compat/livetv/v1/channels", nil)
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.GetStatusCode(), response.GetBody())
	}
	var got []map[string]any
	if err := json.Unmarshal(response.GetBody(), &got); err != nil {
		t.Fatalf("decode channels: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "livetv-v1", "fixtures", "channels.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture []map[string]any
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected channels in response")
	}
	for i, item := range got {
		for key := range fixture[0] {
			if _, ok := item[key]; !ok {
				t.Fatalf("channel %d is missing fixture key %q: %v", i, key, item)
			}
		}
	}
	if got[0]["id"] != "xtream:42" || got[0]["number"] != "101" || got[0]["groupId"] != "7" || got[0]["group"] != "News" {
		t.Fatalf("unexpected first channel: %v", got[0])
	}
	// The raw provider logo, not the relayed "xtream/image?relay_token=..." URL
	// the web app receives for plain-http logos.
	if got[0]["logoUrl"] != "http://logos.example/news.png" {
		t.Fatalf("expected raw provider logo to pass through, got %v", got[0]["logoUrl"])
	}
	if got[1]["logoUrl"] != "" {
		t.Fatalf("expected non-http logo to be dropped, got %v", got[1]["logoUrl"])
	}
	if got[1]["groupId"] != "frost:8" {
		t.Fatalf("expected namespaced category id to pass through unchanged, got %v", got[1]["groupId"])
	}
}

func TestLiveTVContractChannelsEmptyLineupIsArray(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeXtream, nil, config.Settings{})
	response := liveTVContractGet(t, server, "/silo-compat/livetv/v1/channels", nil)
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	if string(response.GetBody()) != "[]" {
		t.Fatalf("empty lineup must be [], got %s", response.GetBody())
	}
}

func decodeLiveTVStream(t *testing.T, response *pluginv1.HandleHTTPResponse) livetvStream {
	t.Helper()
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.GetStatusCode(), response.GetBody())
	}
	var stream livetvStream
	if err := json.Unmarshal(response.GetBody(), &stream); err != nil {
		t.Fatalf("decode stream: %v", err)
	}
	return stream
}

func decodeLiveTVCode(t *testing.T, response *pluginv1.HandleHTTPResponse) string {
	t.Helper()
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("decode error body %q: %v", response.GetBody(), err)
	}
	return payload.Code
}

func TestLiveTVContractStreamRedirect(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeM3UXMLTV, []model.Channel{
		{ID: "m3u:news.hd", Name: "News HD", StreamURL: "https://cdn.example.com/live/news.m3u8"},
	}, config.Settings{})

	stream := decodeLiveTVStream(t, liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", map[string]any{"channel_id": "m3u:news.hd"}))
	want := livetvStream{URL: "https://cdn.example.com/live/news.m3u8", Container: "hls", Mode: "redirect"}
	if stream != want {
		t.Fatalf("stream = %+v, want %+v", stream, want)
	}
}

func TestLiveTVContractStreamServerMode(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeXtream, []model.Channel{
		{ID: "xtream:42", Name: "News Tonight"},
	}, config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "http://xtream.example", XtreamUsername: "u", XtreamPassword: "p"})

	stream := decodeLiveTVStream(t, liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", map[string]any{"channel_id": "xtream:42", "format": "ts"}))
	want := livetvStream{URL: "http://xtream.example/live/u/p/42.ts", Container: "ts", Mode: "server"}
	if stream != want {
		t.Fatalf("stream = %+v, want %+v", stream, want)
	}
}

func TestLiveTVContractXtreamFormatSelection(t *testing.T) {
	t.Parallel()

	settings := config.Settings{
		SourceMode: config.SourceModeXtream,
		XtreamSources: []config.XtreamSource{
			{ID: "primary", BaseURL: "http://xtream.example", Username: "u", Password: "pass1234", LiveFormat: "ts", Enabled: true},
			{ID: "frost", BaseURL: "http://frost.example", Username: "fu", Password: "frostpass", Enabled: true},
		},
	}
	channels := []model.Channel{
		{ID: "xtream:42", Name: "News Tonight"},
		{ID: "xtream:frost:7", Name: "Frost News"},
	}
	server := liveTVContractServer(model.SourceModeXtream, channels, settings)

	cases := []struct {
		name, channelID, format string
		want                    livetvStream
	}{
		{"hls", "xtream:42", "hls", livetvStream{URL: "http://xtream.example/live/u/pass1234/42.m3u8", Container: "hls", Mode: "server"}},
		{"ts", "xtream:42", "ts", livetvStream{URL: "http://xtream.example/live/u/pass1234/42.ts", Container: "ts", Mode: "server"}},
		{"any follows a ts source", "xtream:42", "any", livetvStream{URL: "http://xtream.example/live/u/pass1234/42.ts", Container: "ts", Mode: "server"}},
		{"default format follows a ts source", "xtream:42", "", livetvStream{URL: "http://xtream.example/live/u/pass1234/42.ts", Container: "ts", Mode: "server"}},
		{"any follows an hls source", "xtream:frost:7", "any", livetvStream{URL: "http://frost.example/live/fu/frostpass/7.m3u8", Container: "hls", Mode: "server"}},
		{"ts on an hls source", "xtream:frost:7", "ts", livetvStream{URL: "http://frost.example/live/fu/frostpass/7.ts", Container: "ts", Mode: "server"}},
	}
	for _, tc := range cases {
		query := map[string]any{"channel_id": tc.channelID}
		if tc.format != "" {
			query["format"] = tc.format
		}
		stream := decodeLiveTVStream(t, liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", query))
		if stream != tc.want {
			t.Fatalf("%s: stream = %+v, want %+v", tc.name, stream, tc.want)
		}
	}
}

func TestLiveTVContractXtreamStreamUsesCatalogAccount(t *testing.T) {
	t.Parallel()

	settings := config.Settings{SourceMode: config.SourceModeXtream, XtreamSources: []config.XtreamSource{{
		ID: "frost", BaseURL: "https://frost.example", LiveFormat: "ts", Enabled: true, CatalogAccountID: "a",
		Accounts: []config.XtreamAccount{
			{ID: "a", Username: "first-user", Password: "first-secret", Enabled: true, Catalog: true, Compatible: true, ConnectionLimit: 1},
			{ID: "b", Username: "second-user", Password: "second-secret", Enabled: true, Compatible: true, ConnectionLimit: 1},
		},
	}}}
	server := liveTVContractServer(model.SourceModeXtream, []model.Channel{{ID: "xtream:frost:101", Name: "News"}}, settings)

	// M1 takes no pool lease, so every request resolves with the catalog
	// account instead of rotating through the playback accounts.
	for i := 0; i < 3; i++ {
		stream := decodeLiveTVStream(t, liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", map[string]any{"channel_id": "xtream:frost:101"}))
		if stream.URL != "https://frost.example/live/first-user/first-secret/101.ts" || stream.Mode != "server" {
			t.Fatalf("request %d: unexpected stream %+v", i, stream)
		}
	}
}

func TestLiveTVContractXtreamStreamUnknownSource(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeXtream, []model.Channel{
		{ID: "xtream:ghost:5", Name: "Ghost"},
	}, config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "http://xtream.example", XtreamUsername: "u", XtreamPassword: "p"})
	response := liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", map[string]any{"channel_id": "xtream:ghost:5"})
	if response.GetStatusCode() != http.StatusNotFound || decodeLiveTVCode(t, response) != "channel_not_found" {
		t.Fatalf("expected 404 channel_not_found, got %d: %s", response.GetStatusCode(), response.GetBody())
	}
}

func TestLiveTVContractStreamFormatUnavailable(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeM3UXMLTV, []model.Channel{
		{ID: "m3u:news.hd", Name: "News HD", StreamURL: "https://cdn.example.com/live/news.ts"},
	}, config.Settings{})

	response := liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", map[string]any{"channel_id": "m3u:news.hd", "format": "hls"})
	if response.GetStatusCode() != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", response.GetStatusCode(), response.GetBody())
	}
	if code := decodeLiveTVCode(t, response); code != "format_unavailable" {
		t.Fatalf("code = %q, want format_unavailable", code)
	}
}

func TestLiveTVContractStreamUnknownChannel(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeXtream, nil, config.Settings{})
	response := liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", map[string]any{"channel_id": "xtream:999"})
	if response.GetStatusCode() != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", response.GetStatusCode(), response.GetBody())
	}
	if code := decodeLiveTVCode(t, response); code != "channel_not_found" {
		t.Fatalf("code = %q, want channel_not_found", code)
	}
}

func TestLiveTVContractStreamRejectsBadRequests(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeM3UXMLTV, []model.Channel{
		{ID: "m3u:news.hd", Name: "News HD", StreamURL: "https://cdn.example.com/live/news.m3u8"},
	}, config.Settings{})
	for name, query := range map[string]map[string]any{
		"missing channel_id": nil,
		"blank channel_id":   {"channel_id": "  "},
		"unknown format":     {"channel_id": "m3u:news.hd", "format": "dash"},
	} {
		response := liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", query)
		if response.GetStatusCode() != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", name, response.GetStatusCode(), response.GetBody())
		}
	}
}

func TestLiveTVContractRoutesWorkWithoutSettings(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{Channels: []model.Channel{
		{ID: "m3u:news.hd", Name: "News HD", StreamURL: "https://cdn.example.com/live/news.m3u8"},
	}}})
	server := NewHTTPRoutesServer(store)
	for _, path := range []string{"/silo-compat/livetv/v1/info", "/silo-compat/livetv/v1/channels"} {
		if response := liveTVContractGet(t, server, path, nil); response.GetStatusCode() != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", path, response.GetStatusCode(), response.GetBody())
		}
	}
	stream := decodeLiveTVStream(t, liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", map[string]any{"channel_id": "m3u:news.hd"}))
	if stream.Mode != "redirect" || stream.Container != "hls" {
		t.Fatalf("unexpected stream: %+v", stream)
	}
}

func TestLiveTVContractChannelsOmitStreamCredentials(t *testing.T) {
	t.Parallel()

	server := liveTVContractServer(model.SourceModeXtream, []model.Channel{{ID: "xtream:42", Name: "News", StreamURL: "http://xtream.example/live/u/p/42.ts"}},
		config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "http://xtream.example", XtreamUsername: "u", XtreamPassword: "p"})
	response := liveTVContractGet(t, server, "/silo-compat/livetv/v1/channels", nil)
	if strings.Contains(string(response.GetBody()), "/live/u/p/") {
		t.Fatalf("channels listing must not carry stream credentials: %s", response.GetBody())
	}
}

func TestManifestDeclaresLiveTVContractRoutes(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("..", "..", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest struct {
		HTTPRoutes []struct {
			ID        string `json:"id"`
			Method    string `json:"method"`
			Path      string `json:"path"`
			Access    string `json:"access"`
			Navigable bool   `json:"navigable"`
		} `json:"http_routes"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	// info and stream are admin-only so Silo's plugin proxy never hands
	// credentialed stream URLs or provider hosts to ordinary users; Silo calls
	// them in-process. channels is how Silo discovers the provider.
	want := map[string]struct{ path, access string }{
		"livetv-v1-info":     {"/silo-compat/livetv/v1/info", "admin"},
		"livetv-v1-channels": {"/silo-compat/livetv/v1/channels", "authenticated"},
		"livetv-v1-programs": {"/silo-compat/livetv/v1/programs", "authenticated"},
		"livetv-v1-stream":   {"/silo-compat/livetv/v1/stream", "admin"},
	}
	found := map[string]bool{}
	for _, route := range manifest.HTTPRoutes {
		expected, ok := want[route.ID]
		if !ok {
			continue
		}
		found[route.ID] = true
		if route.Method != http.MethodGet || route.Path != expected.path || route.Access != expected.access || route.Navigable {
			t.Fatalf("route %s declared incorrectly: %+v", route.ID, route)
		}
	}
	for id := range want {
		if !found[id] {
			t.Fatalf("manifest does not declare %s", id)
		}
	}
}
