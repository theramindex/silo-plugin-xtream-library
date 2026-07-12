package app

import (
	"context"
	"testing"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/cache"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xtream"
)

func TestSwitchSourceModeWarnsAndRebuildsCatalog(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{Source: model.LiveTVSource(model.SourceModeXtream), Channels: []model.Channel{{ID: "xtream:1001", Name: "News HD"}}}})
	service := NewService(Dependencies{
		Store: store,
		FetchURL: func(_ context.Context, rawURL string) ([]byte, error) {
			switch rawURL {
			case "https://dispatcharr.example.com/playlist.m3u":
				return []byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"news.hd\",News HD\nhttps://dispatcharr.example.com/live/news-hd.m3u8\n"), nil
			case "https://dispatcharr.example.com/guide.xml":
				return []byte("<?xml version=\"1.0\"?><tv><channel id=\"news.hd\"><display-name>News HD</display-name></channel><programme start=\"20231114221320 +0000\" stop=\"20231114231320 +0000\" channel=\"news.hd\"><title>Morning News</title></programme></tv>"), nil
			default:
				return nil, context.DeadlineExceeded
			}
		},
		XtreamFactory: func(string, string, string) XtreamClient {
			return &stubXtreamClient{streams: []xtream.LiveStream{{Num: 1, Name: "News HD", StreamID: 1001, EPGChannelID: "news.hd"}}, epg: xtream.ShortEPGResponse{EPGListings: []xtream.EPGListing{{ID: "epg-1", Title: "Morning News", StartTimestamp: "1700000000", StopTimestamp: "1700003600"}}}}
		},
	})

	previous := config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "https://dispatcharr.example.com", XtreamUsername: "demo", XtreamPassword: "secret", ChannelRefreshH: 24, EPGRefreshH: 6}
	next := config.Settings{SourceMode: config.SourceModeM3UXMLTV, M3UURL: "https://dispatcharr.example.com/playlist.m3u", EPGXMLURL: "https://dispatcharr.example.com/guide.xml", ChannelRefreshH: 24, EPGRefreshH: 6}

	warning, err := service.SwitchSourceMode(context.Background(), previous, next, 500)
	if err != nil {
		t.Fatalf("expected mode switch success, got %v", err)
	}
	if warning == "" {
		t.Fatal("expected reset warning")
	}

	snapshot := store.Current()
	if snapshot.Catalog.Source.ID != model.LiveTVSourceID {
		t.Fatalf("expected stable live tv source id, got %q", snapshot.Catalog.Source.ID)
	}
	if snapshot.Catalog.Source.Mode != model.SourceModeM3UXMLTV {
		t.Fatalf("expected rebuilt source mode, got %q", snapshot.Catalog.Source.Mode)
	}
	if len(snapshot.Catalog.Channels) != 1 || snapshot.Catalog.Channels[0].ID == "xtream:1001" {
		t.Fatalf("expected rebuilt fallback catalog, got %+v", snapshot.Catalog.Channels)
	}
}
