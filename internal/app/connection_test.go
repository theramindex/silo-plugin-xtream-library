package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/dispatcharr"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xtream"
)

func TestServiceTestConnectionXtreamSuccess(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{XtreamFactory: func(string, string, string) XtreamClient {
		return &stubXtreamClient{streams: []xtream.LiveStream{{StreamID: 1001}}, epg: xtream.ShortEPGResponse{EPGListings: []xtream.EPGListing{{ID: "epg-1"}}}}
	}})

	err := service.TestConnection(context.Background(), config.Settings{
		SourceMode:      config.SourceModeXtream,
		XtreamBaseURL:   "https://dispatcharr.example.com",
		XtreamUsername:  "demo",
		XtreamPassword:  "secret",
		ChannelRefreshH: 24,
		EPGRefreshH:     6,
	})
	if err != nil {
		t.Fatalf("expected connection success, got %v", err)
	}
}

func TestServiceTestConnectionReturnsValidationError(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{})
	err := service.TestConnection(context.Background(), config.Settings{SourceMode: config.SourceModeXtream})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestServiceTestConnectionRejectsOldDispatcharrDirectVersion(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{
		DispatcharrFactory: func(config.Settings) DispatcharrClient {
			return &stubDispatcharrClient{version: dispatcharr.VersionInfo{Version: "0.26.9"}}
		},
	})

	err := service.TestConnection(context.Background(), config.Settings{
		SourceMode:      config.SourceModeDirectLogin,
		DispatcharrURL:  "https://dispatcharr.example.com",
		DispatcharrUser: "demo",
		DispatcharrPass: "secret",
		ChannelRefreshH: 24,
		EPGRefreshH:     6,
	})
	if err == nil || !strings.Contains(err.Error(), config.MinimumDispatcharrVersion) {
		t.Fatalf("expected minimum version error, got %v", err)
	}
}

func TestServiceTestConnectionRejectsMissingEPG(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{XtreamFactory: func(string, string, string) XtreamClient {
		return &stubXtreamClient{streams: []xtream.LiveStream{{StreamID: 1001}}, epg: xtream.ShortEPGResponse{}}
	}})

	err := service.TestConnection(context.Background(), config.Settings{
		SourceMode:      config.SourceModeXtream,
		XtreamBaseURL:   "https://dispatcharr.example.com",
		XtreamUsername:  "demo",
		XtreamPassword:  "secret",
		ChannelRefreshH: 24,
		EPGRefreshH:     6,
	})
	if err == nil {
		t.Fatal("expected epg validation error")
	}
}

func TestServiceTestConnectionM3UXMLTVSuccess(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{FetchURL: func(_ context.Context, rawURL string) ([]byte, error) {
		switch rawURL {
		case "https://dispatcharr.example.com/playlist.m3u":
			return []byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"news.hd\",News HD\nhttps://dispatcharr.example.com/live/news-hd.m3u8\n"), nil
		case "https://dispatcharr.example.com/guide.xml":
			return []byte("<?xml version=\"1.0\"?><tv><channel id=\"news.hd\"><display-name>News HD</display-name></channel><programme start=\"20231114221320 +0000\" stop=\"20231114231320 +0000\" channel=\"news.hd\"><title>Morning News</title></programme></tv>"), nil
		default:
			return nil, errors.New("unexpected url")
		}
	}})

	err := service.TestConnection(context.Background(), config.Settings{
		SourceMode:      config.SourceModeM3UXMLTV,
		M3UURL:          "https://dispatcharr.example.com/playlist.m3u",
		EPGXMLURL:       "https://dispatcharr.example.com/guide.xml",
		ChannelRefreshH: 24,
		EPGRefreshH:     6,
	})
	if err != nil {
		t.Fatalf("expected fallback connection success, got %v", err)
	}
}

type stubXtreamClient struct {
	connectionErr error
	streams       []xtream.LiveStream
	streamsErr    error
	epg           xtream.ShortEPGResponse
	epgErr        error
	resolved      string
}

func (s *stubXtreamClient) TestConnection(context.Context) error { return s.connectionErr }
func (s *stubXtreamClient) LiveStreams(context.Context) ([]xtream.LiveStream, error) {
	return s.streams, s.streamsErr
}
func (s *stubXtreamClient) ShortEPG(context.Context, int64) (xtream.ShortEPGResponse, error) {
	if s.epgErr != nil {
		return xtream.ShortEPGResponse{}, s.epgErr
	}
	return s.epg, nil
}
func (s *stubXtreamClient) ResolveLiveStreamURL(int64) string { return s.resolved }

func TestStubXtreamClientSanity(t *testing.T) {
	t.Parallel()

	stub := &stubXtreamClient{connectionErr: errors.New("boom")}
	if !errors.Is(stub.TestConnection(context.Background()), stub.connectionErr) {
		t.Fatal("expected stub to return configured error")
	}
}
