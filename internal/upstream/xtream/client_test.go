package xtream

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestClientConnectionAndCatalogFetch(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t)
	client := NewClient(server.URL, "demo", "secret")

	if err := client.TestConnection(t.Context()); err != nil {
		t.Fatalf("expected connection success, got %v", err)
	}

	channels, err := client.LiveStreams(t.Context())
	if err != nil {
		t.Fatalf("expected live streams, got %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}
}

func TestClientShortEPGAndPlaybackResolution(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t)
	client := NewClient(server.URL, "demo", "secret")

	epg, err := client.ShortEPG(t.Context(), 1001)
	if err != nil {
		t.Fatalf("expected epg success, got %v", err)
	}
	if len(epg.EPGListings) != 1 {
		t.Fatalf("expected 1 epg listing, got %d", len(epg.EPGListings))
	}

	resolved := client.ResolveLiveStreamURL(1001)
	if resolved == "" {
		t.Fatal("expected resolved playback url")
	}
	if resolved != server.URL+"/live/demo/secret/1001.ts" {
		t.Fatalf("unexpected resolved url %q", resolved)
	}
}

func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()

	liveCategories := readFixture(t, "live_categories.json")
	liveStreams := readFixture(t, "live_streams.json")
	shortEPG := readFixture(t, "short_epg.json")

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/player_api.php" {
			switch r.URL.Query().Get("action") {
			case "get_live_categories":
				_, _ = w.Write(liveCategories)
				return
			case "get_live_streams":
				_, _ = w.Write(liveStreams)
				return
			case "get_short_epg":
				_, _ = w.Write(shortEPG)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "xtream", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
