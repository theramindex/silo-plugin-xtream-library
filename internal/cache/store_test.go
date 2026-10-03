package cache

import (
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
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

func regressionTestPrograms(channels, perChannel int, endUnix int64) []model.Program {
	programs := make([]model.Program, 0, channels*perChannel)
	for c := 0; c < channels; c++ {
		for p := 0; p < perChannel; p++ {
			programs = append(programs, model.Program{
				ID:        "program:" + string(rune('a'+c)) + string(rune('a'+p)),
				ChannelID: "channel:" + string(rune('a'+c)),
				Title:     "Show",
				StartUnix: endUnix - 3600,
				EndUnix:   endUnix,
			})
		}
	}
	return programs
}

func regressionTestChannels(count int) []model.Channel {
	channels := make([]model.Channel, 0, count)
	for c := 0; c < count; c++ {
		channels = append(channels, model.Channel{ID: "channel:" + string(rune('a'+c))})
	}
	return channels
}

func TestStoreReplaceProgramsKeepsGuideOnSharpDropEvenWhileLoading(t *testing.T) {
	t.Parallel()

	store := NewStore()
	store.Replace(Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeDirectLogin),
		Channels: regressionTestChannels(4),
		Health:   model.SyncHealth{LastSuccessUnix: 100},
	}})
	store.ReplacePrograms(regressionTestPrograms(4, 5, 10_000), 200)
	store.MarkEPGLoading()

	if applied := store.ReplacePrograms(regressionTestPrograms(1, 3, 10_000), 300); applied {
		t.Fatal("expected sharp drop to be rejected")
	}
	current := store.Current()
	if len(current.Catalog.Programs) != 20 {
		t.Fatalf("expected last known good guide, got %d programs", len(current.Catalog.Programs))
	}
	if current.Health.EPGStatus != "ok" || current.Health.EPGWarning == "" || current.Health.EPGLastSuccessUnix != 200 {
		t.Fatalf("expected ok status with warning and original success time, got %+v", current.Health)
	}

	// A comparable refresh is applied and clears the warning.
	if applied := store.ReplacePrograms(regressionTestPrograms(4, 3, 10_000), 400); !applied {
		t.Fatal("expected comparable guide to apply")
	}
	current = store.Current()
	if len(current.Catalog.Programs) != 12 || current.Health.EPGWarning != "" || current.Health.EPGLastSuccessUnix != 400 {
		t.Fatalf("expected refreshed guide without warning, got %d programs %+v", len(current.Catalog.Programs), current.Health)
	}
}

func TestStoreReplaceProgramsRejectsChannelCoverageCollapse(t *testing.T) {
	t.Parallel()

	store := NewStore()
	store.Replace(Snapshot{Catalog: model.CatalogState{Channels: regressionTestChannels(6), Health: model.SyncHealth{LastSuccessUnix: 100}}})
	store.ReplacePrograms(regressionTestPrograms(6, 2, 10_000), 200)

	// Same program count, but only 2 of 6 channels covered.
	if store.ReplacePrograms(regressionTestPrograms(2, 6, 10_000), 300) {
		t.Fatal("expected coverage collapse to be rejected")
	}
}

func TestStoreReplaceProgramsIgnoresExpiredPrograms(t *testing.T) {
	t.Parallel()

	store := NewStore()
	store.Replace(Snapshot{Catalog: model.CatalogState{Channels: regressionTestChannels(4), Health: model.SyncHealth{LastSuccessUnix: 100}}})
	store.ReplacePrograms(regressionTestPrograms(4, 5, 1_000), 200)

	// The old guide has fully ended by t=5000, so a small fresh guide applies.
	if !store.ReplacePrograms(regressionTestPrograms(1, 1, 9_000), 5_000) {
		t.Fatal("expected fresh guide to replace an expired one")
	}
}

func TestStoreReplaceExactKeepsGuideOnSharpDropUnlessSourceModeChanges(t *testing.T) {
	t.Parallel()

	base := func(mode model.SourceMode, programs []model.Program, at int64) Snapshot {
		return Snapshot{
			ConfigKey: "key",
			Catalog: model.CatalogState{
				Source:   model.LiveTVSource(mode),
				Channels: regressionTestChannels(4),
				Programs: programs,
			},
			Health: model.SyncHealth{LastSuccessUnix: at, EPGStatus: "ok", EPGProgramCount: len(programs), EPGLastSuccessUnix: at},
		}
	}

	store := NewStore()
	store.ReplaceExact(base(model.SourceModeDirectLogin, regressionTestPrograms(4, 5, 10_000), 100))
	store.MarkEPGLoading()
	store.ReplaceExact(base(model.SourceModeDirectLogin, regressionTestPrograms(1, 2, 10_000), 200))

	current := store.Current()
	if len(current.Catalog.Programs) != 20 || current.Health.EPGWarning == "" || current.Health.EPGStatus != "ok" {
		t.Fatalf("expected preserved guide with warning, got %d programs %+v", len(current.Catalog.Programs), current.Health)
	}
	if current.Health.LastSuccessUnix != 200 {
		t.Fatalf("channel sync success should still advance, got %+v", current.Health)
	}

	switched := base(model.SourceModeXtream, regressionTestPrograms(1, 2, 10_000), 300)
	store.ReplaceExact(switched)
	current = store.Current()
	if len(current.Catalog.Programs) != 2 || current.Health.EPGWarning != "" {
		t.Fatalf("source mode change must accept the new guide, got %d programs %+v", len(current.Catalog.Programs), current.Health)
	}
}

func TestStoreReplacePreservesGuideWhileStatusIsLoading(t *testing.T) {
	t.Parallel()

	store := NewStore()
	store.Replace(Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeDirectLogin),
		Channels: regressionTestChannels(2),
		Health:   model.SyncHealth{LastSuccessUnix: 100},
	}})
	store.ReplacePrograms(regressionTestPrograms(2, 2, 10_000), 200)
	store.MarkEPGLoading()

	store.Replace(Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeDirectLogin),
		Channels: regressionTestChannels(2),
		Health:   model.SyncHealth{LastSuccessUnix: 300},
	}})
	if got := len(store.Current().Catalog.Programs); got != 4 {
		t.Fatalf("expected guide preserved despite loading status, got %d", got)
	}
}

func TestStoreVersionAdvancesOnEveryMutation(t *testing.T) {
	t.Parallel()

	store := NewStore()
	last := store.Current().Version
	steps := []func(){
		func() { store.Replace(Snapshot{Catalog: model.CatalogState{Channels: regressionTestChannels(1)}}) },
		func() { store.ReplacePrograms(regressionTestPrograms(1, 1, 10_000), 1) },
		func() { store.MarkEPGLoading() },
		func() { store.RecordEPGFailure(2, "boom") },
		func() { store.RecordFailure(3, "boom") },
		func() { store.ReplaceExact(Snapshot{Catalog: model.CatalogState{Channels: regressionTestChannels(1)}}) },
		func() { store.ClearGuidePrograms(4) },
	}
	for index, step := range steps {
		step()
		current := store.Current().Version
		if current <= last {
			t.Fatalf("step %d: expected version to advance past %d, got %d", index, last, current)
		}
		last = current
	}
}
