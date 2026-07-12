package app

import (
	"context"
	"testing"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
)

func TestResolvePlaybackUsesFreshXtreamResolution(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{XtreamFactory: func(string, string, string) XtreamClient {
		return &stubXtreamClient{resolved: "https://dispatcharr.example.com/live/demo/secret/1001.m3u8"}
	}})

	resolved, err := service.ResolvePlayback(context.Background(), config.Settings{
		SourceMode:      config.SourceModeXtream,
		XtreamBaseURL:   "https://dispatcharr.example.com",
		XtreamUsername:  "demo",
		XtreamPassword:  "secret",
		ChannelRefreshH: 24,
		EPGRefreshH:     6,
	}, 1001)
	if err != nil {
		t.Fatalf("expected playback resolution success, got %v", err)
	}
	if resolved != "https://dispatcharr.example.com/live/demo/secret/1001.m3u8" {
		t.Fatalf("unexpected playback resolution %q", resolved)
	}
}

func TestResolvePlaybackUsesCurrentM3UEntry(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{FetchURL: func(_ context.Context, rawURL string) ([]byte, error) {
		if rawURL == "https://dispatcharr.example.com/playlist.m3u" {
			return []byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"news.hd\",News HD\nhttps://dispatcharr.example.com/live/news-hd.m3u8\n"), nil
		}
		if rawURL == "https://dispatcharr.example.com/guide.xml" {
			return []byte("<?xml version=\"1.0\"?><tv></tv>"), nil
		}
		return nil, nil
	}})

	resolved, err := service.ResolvePlayback(context.Background(), config.Settings{SourceMode: config.SourceModeM3UXMLTV, M3UURL: "https://dispatcharr.example.com/playlist.m3u", EPGXMLURL: "https://dispatcharr.example.com/guide.xml", ChannelRefreshH: 24, EPGRefreshH: 6}, 0)
	if err != nil {
		t.Fatalf("expected m3u playback resolution success, got %v", err)
	}
	if resolved != "https://dispatcharr.example.com/live/news-hd.m3u8" {
		t.Fatalf("unexpected fallback playback resolution %q", resolved)
	}
}
