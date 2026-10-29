package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

func TestFootballStatsLoadDedupesWithoutHoldingTheLock(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		entered <- struct{}{}
		<-release
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/scoreboard":
			_, _ = w.Write([]byte(`{"events":[{"id":"401860879","competitions":[` + statsFixtureCompetition + `]}]}`))
		case "/summary":
			_, _ = w.Write([]byte(`{"header":{"id":"401860879","competitions":[` + statsFixtureCompetition + `]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	stats := &footballStatsCache{baseURL: server.URL, client: server.Client()}

	var wg sync.WaitGroup
	results := make([]SportsGameStats, 6)
	for index := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[index] = stats.load(context.Background(), statsFixtureEvent())
		}()
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("stats fetch never started")
	}
	// The lock must stay free while ESPN is slow, so other games and the
	// remembered-fixture lookup are not blocked.
	if !stats.mu.TryLock() {
		t.Fatal("footballStatsCache.mu is held across the ESPN request")
	}
	stats.mu.Unlock()
	close(release)
	wg.Wait()
	// Callers that arrive after the shared load finished read its cached
	// entry, so the whole burst costs one scoreboard and one summary.
	if got := requests.Load(); got != 2 {
		t.Fatalf("concurrent pollers must share one load (scoreboard + summary), got %d requests", got)
	}
	for _, result := range results {
		if result.SourceURL == "" || result.HomeScore != "46" {
			t.Fatalf("every poller gets the shared result: %+v", result)
		}
	}
}

func TestPreparedSportsEventCopiesOnlyTheRequestedEvent(t *testing.T) {
	t.Parallel()
	server := NewHTTPRoutesServer(cache.NewStore())
	spread := 3.5
	server.sportsPrepared = sportsPreparedCache{Ready: true, ExpiresAfter: time.Now().Add(time.Minute), Payload: SportsPayload{Events: []SportsEvent{
		{ID: "other"},
		{ID: "game", StableID: "sports-event:stable", Spread: &spread, Channels: []SportsChannelMatch{{ID: "channel"}}},
	}}}
	event, ok := server.preparedSportsEvent("sports-event:stable")
	if !ok || event.ID != "game" {
		t.Fatalf("lookup by stable ID failed: %+v %v", event, ok)
	}
	event.Channels[0].ID = "mutated"
	*event.Spread = 0
	if server.sportsPrepared.Payload.Events[1].Channels[0].ID != "channel" || *server.sportsPrepared.Payload.Events[1].Spread != 3.5 {
		t.Fatal("the returned event must be a copy")
	}
	if _, ok := server.preparedSportsEvent("missing"); ok {
		t.Fatal("unknown IDs must not resolve")
	}
}

func TestSportsFlightGroupSharesOneCall(t *testing.T) {
	t.Parallel()
	var group sportsFlightGroup
	var calls atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			value, err := group.do(context.Background(), "key", func() (any, error) {
				calls.Add(1)
				time.Sleep(20 * time.Millisecond)
				return "value", nil
			})
			if err != nil || value != "value" {
				t.Errorf("shared value = %v, %v", value, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if calls.Load() > 2 {
		t.Fatalf("concurrent callers should share a call, got %d", calls.Load())
	}
	if _, err := group.do(context.Background(), "key", func() (any, error) { calls.Add(1); return nil, nil }); err != nil || len(group.calls) != 0 {
		t.Fatal("finished flights must be forgotten")
	}
}

func TestSportsGuideCacheExpiresAtBroadcastBoundaryAndSnapshotChange(t *testing.T) {
	t.Parallel()
	for _, sqlite := range []bool{false, true} {
		name := "memory"
		if sqlite {
			name = "sqlite"
		}
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, time.October, 26, 20, 0, 0, 0, time.UTC)
			store := cache.NewStore()
			store.Replace(cache.Snapshot{Catalog: model.CatalogState{
				Channels: []model.Channel{{ID: "mlb", Name: "MLB Extra Innings", CategoryName: "MLB"}},
				Programs: []model.Program{{ID: "game", ChannelID: "mlb", Title: "Live: Atlanta Braves vs New York Mets", StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Minute).Unix()}},
			}})
			var storage *cache.FileSnapshotStorage
			if sqlite {
				storage = cache.NewFileSnapshotStorage(filepath.Join(t.TempDir(), "catalog.json"))
				defer storage.Close()
				store.SetStorage(storage)
				if err := store.Persist(); err != nil {
					t.Fatal(err)
				}
			}
			var guide sportsGuideCache
			first := guide.get(store.Current(), now)
			if len(first.Events) != 1 || !first.Events[0].Live {
				t.Fatalf("expected the currently airing game: %+v", first.Events)
			}
			first.Events[0].Home.Name = "changed by consumer"
			fresh := guide.get(store.Current(), now.Add(30*time.Second))
			if fresh.Events[0].Home.Name == "changed by consumer" {
				t.Fatal("a consumer changed the cached guide")
			}
			ended := guide.get(store.Current(), now.Add(time.Minute+time.Second))
			if ended.Events[0].Live || !ended.Events[0].Completed {
				t.Fatalf("the broadcast boundary must invalidate live state: %+v", ended.Events[0])
			}
			snapshot := store.Current()
			snapshot.Catalog.Programs = []model.Program{{ID: "replacement", ChannelID: "mlb", Title: "New York Yankees vs Boston Red Sox", StartUnix: now.Unix(), EndUnix: now.Add(time.Hour).Unix()}}
			store.Replace(snapshot)
			replaced := guide.get(store.Current(), now.Add(2*time.Minute))
			if len(replaced.Events) != 1 || !strings.Contains(replaced.Events[0].Name, "Yankees") {
				t.Fatalf("snapshot change served the previous matchup: %+v", replaced.Events)
			}
			if sqlite {
				if err := storage.Close(); err != nil {
					t.Fatal(err)
				}
				if unavailable := guide.get(store.Current(), now.Add(10*time.Minute)); !unavailable.Unavailable {
					t.Fatal("a cache read failure was accepted as an empty sports guide")
				}
			}
		})
	}
}
