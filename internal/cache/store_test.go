package cache

import (
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
)

func TestStorePrunesExpiredWatchSessions(t *testing.T) {
	t.Parallel()

	store := NewStore()
	now := time.Now().Unix()
	store.sessions["expired"] = WatchSession{ID: "expired", LastHeartbeatUnix: now - watchSessionTTLSeconds - 1}
	store.sessions["recent"] = WatchSession{ID: "recent", LastHeartbeatUnix: now}

	started := store.StartWatch("channel", "channel:1", "News")
	if started.ID == "" {
		t.Fatal("expected a watch session ID")
	}
	if _, exists := store.sessions["expired"]; exists {
		t.Fatal("expected expired watch session to be pruned")
	}
	if _, exists := store.sessions["recent"]; !exists {
		t.Fatal("expected recent watch session to remain")
	}
}

func TestStorePreservesLastSuccessfulSnapshotOnFailure(t *testing.T) {
	t.Parallel()

	store := NewStore()
	snapshot := Snapshot{Catalog: model.CatalogState{Source: model.LiveTVSource(model.SourceModeXtream), Channels: []model.Channel{{ID: "xtream:1", Name: "News"}}}}
	store.Replace(snapshot)
	store.RecordFailure(100, "upstream unavailable")

	current := store.Current()
	if len(current.Catalog.Channels) != 1 {
		t.Fatalf("expected stale channels to remain available, got %d", len(current.Catalog.Channels))
	}
	if current.Health.LastError != "upstream unavailable" {
		t.Fatalf("expected failure to be tracked, got %q", current.Health.LastError)
	}
}

func TestStoreReplaceClearsPreviousFailureOnSuccess(t *testing.T) {
	t.Parallel()

	store := NewStore()
	store.RecordFailure(100, "timeout")
	store.Replace(Snapshot{Catalog: model.CatalogState{Source: model.LiveTVSource(model.SourceModeXtream)}, Health: model.SyncHealth{LastSuccessUnix: 200}})

	current := store.Current()
	if current.Health.LastFailureUnix != 0 {
		t.Fatalf("expected failure timestamp to clear, got %d", current.Health.LastFailureUnix)
	}
	if current.Health.LastError != "" {
		t.Fatalf("expected failure message to clear, got %q", current.Health.LastError)
	}
	if current.Health.LastSuccessUnix != 200 {
		t.Fatalf("expected success timestamp to persist, got %d", current.Health.LastSuccessUnix)
	}
}

func TestStorePreservesFullGuideWhenRefreshReturnsPartialPrograms(t *testing.T) {
	t.Parallel()

	store := NewStore()
	store.Replace(Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeDirectLogin),
		Channels: []model.Channel{{ID: "channel:1", Name: "News"}, {ID: "channel:2", Name: "Sports"}},
		Health:   model.SyncHealth{LastSuccessUnix: 100},
	}})
	store.ReplacePrograms([]model.Program{
		{ID: "program:1", ChannelID: "channel:1", Title: "Morning News"},
		{ID: "program:2", ChannelID: "channel:2", Title: "Highlights"},
	}, 200)

	store.Replace(Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeDirectLogin),
		Channels: []model.Channel{{ID: "channel:1", Name: "News"}, {ID: "channel:2", Name: "Sports"}},
		Programs: []model.Program{{ID: "program:partial", ChannelID: "channel:1", Title: "Grid Preview"}},
		Health:   model.SyncHealth{LastSuccessUnix: 300},
	}})

	current := store.Current()
	if len(current.Catalog.Programs) != 2 {
		t.Fatalf("expected full guide to be preserved, got %+v", current.Catalog.Programs)
	}
	if current.Health.EPGProgramCount != 2 || current.Health.EPGLastSuccessUnix != 200 {
		t.Fatalf("expected preserved epg health, got %+v", current.Health)
	}
}

func TestStoreReplaceAllowsLargerGuideToReplacePreservedGuide(t *testing.T) {
	t.Parallel()

	store := NewStore()
	store.Replace(Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeDirectLogin),
		Channels: []model.Channel{{ID: "channel:1", Name: "News"}},
		Health:   model.SyncHealth{LastSuccessUnix: 100},
	}})
	store.ReplacePrograms([]model.Program{{ID: "program:1", ChannelID: "channel:1", Title: "Morning News"}}, 200)

	store.Replace(Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeDirectLogin),
		Channels: []model.Channel{{ID: "channel:1", Name: "News"}},
		Programs: []model.Program{
			{ID: "program:2", ChannelID: "channel:1", Title: "Noon News"},
			{ID: "program:3", ChannelID: "channel:1", Title: "Evening News"},
		},
		Health: model.SyncHealth{LastSuccessUnix: 300},
	}})

	current := store.Current()
	if len(current.Catalog.Programs) != 2 || current.Catalog.Programs[0].ID != "program:2" {
		t.Fatalf("expected larger guide to replace preserved guide, got %+v", current.Catalog.Programs)
	}
}

func TestStoreDoesNotPersistPlaybackURLStateSeparately(t *testing.T) {
	t.Parallel()

	store := NewStore()
	store.Replace(Snapshot{Catalog: model.CatalogState{Channels: []model.Channel{{ID: "xtream:1", StreamURL: "https://example.com/live.m3u8"}}}})

	current := store.Current()
	if current.Catalog.Channels[0].StreamURL != "https://example.com/live.m3u8" {
		t.Fatalf("expected catalog snapshot to preserve stream url field, got %q", current.Catalog.Channels[0].StreamURL)
	}
	if current.PlaybackResolvedAtUnix != 0 {
		t.Fatalf("expected no cached playback resolution timestamp, got %d", current.PlaybackResolvedAtUnix)
	}
}

func TestStoreDoesNotPreserveGuideAcrossCatalogConfigChanges(t *testing.T) {
	t.Parallel()

	store := NewStore()
	store.Replace(Snapshot{
		Catalog: model.CatalogState{
			Source:   model.LiveTVSource(model.SourceModeXtream),
			Channels: []model.Channel{{ID: "xtream:1"}},
			Programs: []model.Program{{ID: "old", ChannelID: "xtream:1"}},
			Health:   model.SyncHealth{EPGStatus: "ok", EPGProgramCount: 1, EPGLastSuccessUnix: 10},
		},
		Health:    model.SyncHealth{EPGStatus: "ok", EPGProgramCount: 1, EPGLastSuccessUnix: 10},
		ConfigKey: "old-config",
	})
	store.Replace(Snapshot{
		Catalog:   model.CatalogState{Source: model.LiveTVSource(model.SourceModeXtream), Channels: []model.Channel{{ID: "xtream:1"}}},
		ConfigKey: "new-config",
	})

	if programs := store.Current().Catalog.Programs; len(programs) != 0 {
		t.Fatalf("expected old guide to be discarded across config change, got %+v", programs)
	}
}
