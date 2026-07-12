package xtream

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLiveStreamAcceptsStringArchiveDuration(t *testing.T) {
	t.Parallel()

	var stream LiveStream
	if err := json.Unmarshal([]byte(`{"stream_id":1001,"tv_archive":1,"tv_archive_duration":"60"}`), &stream); err != nil {
		t.Fatalf("decode provider stream: %v", err)
	}
	if !stream.CatchupAvailable() || stream.TVArchiveDurationMinutes != 60 {
		t.Fatalf("unexpected catchup metadata: %+v", stream)
	}
}

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
	if !channels[0].CatchupAvailable() || channels[0].TVArchiveDurationMinutes != 60 {
		t.Fatalf("expected provider catchup metadata, got %+v", channels[0])
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
	if resolved := client.ResolveLiveStreamURLWithExtension(1001, "m3u8"); resolved != server.URL+"/live/demo/secret/1001.m3u8" {
		t.Fatalf("unexpected HLS resolved url %q", resolved)
	}
	if resolved := client.ResolveLiveStreamURLWithExtension(1001, "unsupported"); resolved != server.URL+"/live/demo/secret/1001.ts" {
		t.Fatalf("expected unsupported format to fall back to MPEG-TS, got %q", resolved)
	}
}

func TestClientSeriesEpisodesAndCatchupResolution(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t)
	client := NewClient(server.URL, "demo", "secret")

	series, err := client.SeriesInfo(t.Context(), 501)
	if err != nil {
		t.Fatalf("load series info: %v", err)
	}
	if len(series.Episodes) != 2 || series.Episodes[0].ID != 9001 || series.Episodes[0].SeasonNumber != 1 {
		t.Fatalf("unexpected series episodes: %+v", series.Episodes)
	}

	if resolved := client.ResolveEpisodeStreamURL(series.Episodes[0]); resolved != server.URL+"/series/demo/secret/9001.mkv" {
		t.Fatalf("unexpected episode url %q", resolved)
	}
	if resolved := client.ResolveCatchupStreamURL(1001, 60, "2026-07-11:20-30"); resolved != server.URL+"/timeshift/demo/secret/60/2026-07-11:20-30/1001.ts" {
		t.Fatalf("unexpected catchup url %q", resolved)
	}
}

func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()

	liveCategories := readFixture(t, "live_categories.json")
	liveStreams := readFixture(t, "live_streams.json")
	shortEPG := readFixture(t, "short_epg.json")
	seriesInfo := readFixture(t, "series_info.json")

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
			case "get_series_info":
				_, _ = w.Write(seriesInfo)
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
