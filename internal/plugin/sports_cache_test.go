package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
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
			value, err := group.do("key", func() (any, error) {
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
	if _, err := group.do("key", func() (any, error) { calls.Add(1); return nil, nil }); err != nil || len(group.calls) != 0 {
		t.Fatal("finished flights must be forgotten")
	}
}
