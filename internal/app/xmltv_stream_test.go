package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
)

type trackingBody struct {
	io.Reader
	closed bool
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

func TestM3UXMLTVGuideIsStreamedThroughOpenURL(t *testing.T) {
	t.Parallel()

	guide := `<tv><channel id="news.hd"><display-name>News HD</display-name></channel>` +
		`<programme channel="news.hd" start="20260101000000 +0000" stop="20260101010000 +0000"><title>Morning News</title></programme></tv>`
	body := &trackingBody{Reader: strings.NewReader(guide)}
	store := cache.NewStore()
	service := NewService(Dependencies{
		Store: store,
		FetchURL: func(_ context.Context, rawURL string) ([]byte, error) {
			if rawURL == "https://provider.example/playlist.m3u" {
				return []byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"news.hd\",News HD\nhttps://provider.example/live/news-hd.m3u8\n"), nil
			}
			return nil, errors.New("guide must not be buffered via FetchURL")
		},
		OpenURL: func(_ context.Context, rawURL string) (io.ReadCloser, error) {
			if rawURL != "https://provider.example/guide.xml" {
				return nil, errors.New("unexpected url " + rawURL)
			}
			return body, nil
		},
	})
	settings := config.Settings{
		SourceMode:      config.SourceModeM3UXMLTV,
		M3UURL:          "https://provider.example/playlist.m3u",
		EPGXMLURL:       "https://provider.example/guide.xml",
		ChannelRefreshH: 24,
		EPGRefreshH:     6,
	}
	if err := service.SyncNow(context.Background(), settings, 200); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !body.closed {
		t.Fatal("expected streamed body to be closed")
	}
	if got := len(store.Current().Catalog.Programs); got != 1 {
		t.Fatalf("expected 1 streamed program, got %d", got)
	}
}

func TestFetchXMLTVFallsBackToInjectedFetchURL(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{FetchURL: func(context.Context, string) ([]byte, error) {
		return []byte(`<tv><programme channel="a" start="1" stop="2"><title>x</title></programme></tv>`), nil
	}})
	doc, err := service.fetchXMLTV(context.Background(), "https://guide.example/epg.xml", "epg")
	if err != nil || len(doc.Programmes) != 1 {
		t.Fatalf("expected fallback parse, got %+v %v", doc, err)
	}
}

func TestFetchXMLTVPrefixesAndRedactsErrors(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{FetchURL: func(context.Context, string) ([]byte, error) {
		return []byte(`<html></html>`), nil
	}})
	_, err := service.fetchXMLTV(context.Background(), "https://guide.example/epg.xml", "epg")
	if err == nil || !strings.HasPrefix(err.Error(), "parse epg xmltv:") {
		t.Fatalf("expected parse error prefix, got %v", err)
	}

	// The default opener must not echo credentialed URLs in errors.
	service = NewService(Dependencies{})
	_, err = service.fetchXMLTV(context.Background(), "http://127.0.0.1:1/xmltv.php?username=demo&password=secret", "epg")
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.HasPrefix(err.Error(), "fetch epg xmltv:") {
		t.Fatalf("expected redacted fetch error, got %v", err)
	}
}
