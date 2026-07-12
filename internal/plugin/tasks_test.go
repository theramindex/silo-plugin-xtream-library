package plugin

import (
	"context"
	"errors"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/app"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/cache"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/dispatcharr"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xtream"
)

func TestScheduledTaskServerRunsSyncTask(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	client := &scheduledStubClient{
		streams: []xtream.LiveStream{{Num: 1, Name: "News HD", StreamID: 1001, EPGChannelID: "news.hd"}},
		epg:     xtream.ShortEPGResponse{EPGListings: []xtream.EPGListing{{ID: "epg-1", Title: "Morning News", StartTimestamp: "1700000000", StopTimestamp: "1700003600"}}},
	}
	service := app.NewService(app.Dependencies{
		Store: store,
		XtreamFactory: func(string, string, string) app.XtreamClient {
			return client
		},
	})
	server := NewScheduledTaskServer(service, config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "https://dispatcharr.example.com", XtreamUsername: "demo", XtreamPassword: "secret", ChannelRefreshH: 24, EPGRefreshH: 6})

	response, err := server.Run(context.Background(), &pluginv1.RunScheduledTaskRequest{TaskKey: SyncTaskKey})
	if err != nil {
		t.Fatalf("run task: %v", err)
	}
	if response.GetOutput().AsMap()["status"] != "queued" {
		t.Fatalf("unexpected task output: %+v", response.GetOutput().AsMap())
	}
	if response.GetOutput().AsMap()["task"] != "catalog" {
		t.Fatalf("expected catalog task output, got %+v", response.GetOutput().AsMap())
	}
	waitForScheduledTask(t, server)

	snapshot := store.Current()
	if len(snapshot.Catalog.Channels) != 1 || len(snapshot.Catalog.Programs) != 1 {
		t.Fatalf("expected sync to populate channels and programs, got %+v", snapshot.Catalog)
	}
}

func TestScheduledTaskServerRunsSiloNamespacedSyncTask(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	client := &scheduledStubClient{
		streams: []xtream.LiveStream{{Num: 1, Name: "News HD", StreamID: 1001, EPGChannelID: "news.hd"}},
		epg:     xtream.ShortEPGResponse{EPGListings: []xtream.EPGListing{{ID: "epg-1", Title: "Morning News", StartTimestamp: "1700000000", StopTimestamp: "1700003600"}}},
	}
	service := app.NewService(app.Dependencies{
		Store: store,
		XtreamFactory: func(string, string, string) app.XtreamClient {
			return client
		},
	})
	server := NewScheduledTaskServer(service, config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "https://dispatcharr.example.com", XtreamUsername: "demo", XtreamPassword: "secret", ChannelRefreshH: 24, EPGRefreshH: 6})

	if _, err := server.Run(context.Background(), &pluginv1.RunScheduledTaskRequest{TaskKey: "plugin:14:dispatcharr-sync"}); err != nil {
		t.Fatalf("run namespaced task: %v", err)
	}
	waitForScheduledTask(t, server)

	snapshot := store.Current()
	if len(snapshot.Catalog.Channels) != 1 || len(snapshot.Catalog.Programs) != 1 {
		t.Fatalf("expected namespaced task to populate channels and programs, got %+v", snapshot.Catalog)
	}
}

func TestScheduledTaskServerRunsChannelRefreshTask(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	settings := config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "https://dispatcharr.example.com", XtreamUsername: "demo", XtreamPassword: "secret", ChannelRefreshH: 24, EPGRefreshH: 6}
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source:   model.LiveTVSource(model.SourceModeXtream),
			Channels: []model.Channel{{ID: "xtream:1001", Name: "Old News"}},
			Programs: []model.Program{{ID: "program:old", ChannelID: "xtream:1001", Title: "Old Morning"}},
			Health:   model.SyncHealth{LastSuccessUnix: 100, EPGStatus: "ok", EPGProgramCount: 1, EPGLastSuccessUnix: 90},
		},
		Health:    model.SyncHealth{LastSuccessUnix: 100, EPGStatus: "ok", EPGProgramCount: 1, EPGLastSuccessUnix: 90},
		ConfigKey: config.CatalogCacheKey(settings),
	})
	client := &scheduledStubClient{
		streams: []xtream.LiveStream{{Num: 1, Name: "News HD", StreamID: 1001, EPGChannelID: "news.hd"}},
		epg:     xtream.ShortEPGResponse{EPGListings: []xtream.EPGListing{{ID: "epg-1", Title: "Morning News", StartTimestamp: "1700000000", StopTimestamp: "1700003600"}}},
	}
	service := app.NewService(app.Dependencies{
		Store: store,
		XtreamFactory: func(string, string, string) app.XtreamClient {
			return client
		},
	})
	server := NewScheduledTaskServer(service, settings)

	response, err := server.Run(context.Background(), &pluginv1.RunScheduledTaskRequest{TaskKey: ChannelRefreshTaskKey})
	if err != nil {
		t.Fatalf("run channel refresh task: %v", err)
	}
	if response.GetOutput().AsMap()["task"] != "channels" {
		t.Fatalf("expected channels task output, got %+v", response.GetOutput().AsMap())
	}
	waitForScheduledTask(t, server)

	snapshot := store.Current()
	if len(snapshot.Catalog.Channels) != 1 {
		t.Fatalf("expected channel refresh to populate channels, got %+v", snapshot.Catalog)
	}
	if client.epgCalls != 0 {
		t.Fatalf("channel refresh fetched guide data %d times", client.epgCalls)
	}
	if snapshot.Health.EPGLastSuccessUnix != 90 || snapshot.Health.EPGProgramCount != 1 {
		t.Fatalf("channel refresh changed guide freshness: %+v", snapshot.Health)
	}
}

func TestScheduledTaskServerRunsEPGRefreshTask(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeM3UXMLTV),
		Channels: []model.Channel{{ID: "m3u:news-hd", Name: "News HD", GuideID: "news.hd"}},
		Health:   model.SyncHealth{LastSuccessUnix: 100},
	}})
	service := app.NewService(app.Dependencies{
		Store: store,
		FetchURL: func(_ context.Context, rawURL string) ([]byte, error) {
			if rawURL != "https://dispatcharr.example.com/guide.xml" {
				t.Fatalf("unexpected epg url %q", rawURL)
			}
			return []byte("<?xml version=\"1.0\"?><tv><programme start=\"20260619070000 +0000\" stop=\"20260619080000 +0000\" channel=\"news.hd\"><title>Morning News</title></programme></tv>"), nil
		},
	})
	server := NewScheduledTaskServer(service, config.Settings{SourceMode: config.SourceModeM3UXMLTV, M3UURL: "https://dispatcharr.example.com/playlist.m3u", EPGXMLURL: "https://dispatcharr.example.com/guide.xml", ChannelRefreshH: 24, EPGRefreshH: 6})

	response, err := server.Run(context.Background(), &pluginv1.RunScheduledTaskRequest{TaskKey: "plugin:14:" + EPGRefreshTaskKey})
	if err != nil {
		t.Fatalf("run epg refresh task: %v", err)
	}
	if response.GetOutput().AsMap()["task"] != "epg" {
		t.Fatalf("expected epg task output, got %+v", response.GetOutput().AsMap())
	}
	waitForScheduledTask(t, server)

	snapshot := store.Current()
	if len(snapshot.Catalog.Programs) != 1 || snapshot.Catalog.Programs[0].Title != "Morning News" {
		t.Fatalf("expected epg refresh to populate guide programs, got %+v", snapshot.Catalog.Programs)
	}
	if snapshot.Health.EPGStatus != "ok" {
		t.Fatalf("expected epg health to be ok, got %+v", snapshot.Health)
	}
}

func TestScheduledTaskServerEPGRefreshKeepsGuideOnDirectFailure(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeDirectLogin),
		Channels: []model.Channel{{ID: "dispatcharr:old", Name: "Old News"}},
		Health:   model.SyncHealth{LastSuccessUnix: 100},
	}})
	store.ReplacePrograms([]model.Program{{ID: "program:old-1", ChannelID: "dispatcharr:old", Title: "Old Morning"}}, 200)
	service := app.NewService(app.Dependencies{
		Store: store,
		DispatcharrFactory: func(config.Settings) app.DispatcharrClient {
			return &scheduledDispatcharrFailureClient{}
		},
	})
	server := NewScheduledTaskServer(service, config.Settings{
		SourceMode:      config.SourceModeDirectLogin,
		DispatcharrURL:  "https://dispatcharr.example.com",
		DispatcharrUser: "demo",
		DispatcharrPass: "secret",
		ChannelRefreshH: 24,
		EPGRefreshH:     6,
	})

	if _, err := server.Run(context.Background(), &pluginv1.RunScheduledTaskRequest{TaskKey: EPGRefreshTaskKey}); err != nil {
		t.Fatalf("queue direct EPG refresh: %v", err)
	}
	if err := waitForScheduledTaskResult(t, server); err == nil {
		t.Fatal("expected background direct EPG refresh failure")
	}

	snapshot := store.Current()
	if len(snapshot.Catalog.Programs) != 1 || snapshot.Catalog.Programs[0].Title != "Old Morning" {
		t.Fatalf("expected scheduled EPG failure to keep existing guide, got %+v", snapshot.Catalog.Programs)
	}
	if snapshot.Health.EPGStatus != "failed" || snapshot.Health.EPGLastError == "" {
		t.Fatalf("expected failed epg health, got %+v", snapshot.Health)
	}
}

func TestScheduledTaskServerReturnsBeforeSlowRefreshCompletes(t *testing.T) {
	t.Parallel()

	target := &controlledRefreshTarget{started: make(chan RefreshOperation, 1), release: make(chan struct{})}
	coordinator := NewRefreshCoordinator(target)
	settings := config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "https://dispatcharr.example.com", XtreamUsername: "demo", XtreamPassword: "secret", ChannelRefreshH: 24, EPGRefreshH: 6}
	server := NewScheduledTaskServerWithCoordinator(coordinator, func() config.Settings { return settings })

	startedAt := time.Now()
	response, err := server.Run(context.Background(), &pluginv1.RunScheduledTaskRequest{TaskKey: ChannelRefreshTaskKey})
	if err != nil {
		t.Fatalf("queue slow channel refresh: %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
		t.Fatalf("scheduled task waited for background refresh: %s", elapsed)
	}
	if response.GetOutput().AsMap()["status"] != "queued" || response.GetOutput().AsMap()["jobId"] == "" {
		t.Fatalf("unexpected queued task output: %+v", response.GetOutput().AsMap())
	}
	if operation := waitForRefreshOperation(t, target.started); operation != RefreshChannels {
		t.Fatalf("expected channel refresh, got %q", operation)
	}
	close(target.release)
	waitForScheduledTask(t, server)
}

func waitForScheduledTask(t *testing.T, server *ScheduledTaskServer) {
	t.Helper()
	if err := waitForScheduledTaskResult(t, server); err != nil {
		t.Fatalf("background scheduled task: %v", err)
	}
}

func waitForScheduledTaskResult(t *testing.T, server *ScheduledTaskServer) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return server.coordinator.Wait(ctx)
}

type scheduledStubClient struct {
	streams  []xtream.LiveStream
	epg      xtream.ShortEPGResponse
	epgCalls int
}

func (s *scheduledStubClient) TestConnection(context.Context) error { return nil }
func (s *scheduledStubClient) LiveStreams(context.Context) ([]xtream.LiveStream, error) {
	return s.streams, nil
}
func (s *scheduledStubClient) ShortEPG(context.Context, int64) (xtream.ShortEPGResponse, error) {
	s.epgCalls++
	return s.epg, nil
}
func (s *scheduledStubClient) ResolveLiveStreamURL(streamID int64) string {
	return "https://dispatcharr.example.com/live/demo/secret/" + "1001.m3u8"
}

type scheduledDispatcharrFailureClient struct{}

func (s *scheduledDispatcharrFailureClient) TestConnection(context.Context) error { return nil }
func (s *scheduledDispatcharrFailureClient) Version(context.Context) (dispatcharr.VersionInfo, error) {
	return dispatcharr.VersionInfo{Version: dispatcharr.String(config.MinimumDispatcharrVersion)}, nil
}
func (s *scheduledDispatcharrFailureClient) Channels(context.Context) ([]dispatcharr.Channel, error) {
	return nil, errors.New("dispatcharr unavailable")
}
func (s *scheduledDispatcharrFailureClient) ChannelGroups(context.Context) ([]dispatcharr.ChannelGroup, error) {
	return nil, nil
}
func (s *scheduledDispatcharrFailureClient) ChannelProfiles(context.Context) ([]dispatcharr.ChannelProfile, error) {
	return nil, nil
}
func (s *scheduledDispatcharrFailureClient) CurrentUser(context.Context) (dispatcharr.CurrentUser, error) {
	return dispatcharr.CurrentUser{ID: "1", Username: "admin", UserLevel: 10}, nil
}
func (s *scheduledDispatcharrFailureClient) Programs(context.Context) ([]dispatcharr.Program, error) {
	return nil, nil
}
func (s *scheduledDispatcharrFailureClient) SearchPrograms(context.Context, time.Time, time.Time) ([]dispatcharr.ProgramSearchResult, error) {
	return nil, nil
}
func (s *scheduledDispatcharrFailureClient) VODCategories(context.Context) ([]dispatcharr.VODCategory, error) {
	return nil, nil
}
func (s *scheduledDispatcharrFailureClient) Movies(context.Context) ([]dispatcharr.Movie, error) {
	return nil, nil
}
func (s *scheduledDispatcharrFailureClient) Series(context.Context) ([]dispatcharr.Series, error) {
	return nil, nil
}
func (s *scheduledDispatcharrFailureClient) LiveStreamURL(string) string { return "" }
func (s *scheduledDispatcharrFailureClient) LogoCacheURL(string) string  { return "" }
func (s *scheduledDispatcharrFailureClient) MovieStreamURL(string) string {
	return ""
}
func (s *scheduledDispatcharrFailureClient) SeriesStreamURL(string) string {
	return ""
}
func (s *scheduledDispatcharrFailureClient) AbsoluteURL(raw string) string { return raw }
