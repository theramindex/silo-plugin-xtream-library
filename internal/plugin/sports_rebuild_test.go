package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

type slowSportsProvider struct {
	staticSportsProvider
	delay time.Duration
}

func (p slowSportsProvider) Events(ctx context.Context, now time.Time) ([]SportsEvent, error) {
	select {
	case <-time.After(p.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return p.staticSportsProvider.Events(ctx, now)
}

// rebuildTestServer has one provider game that only channel matching can place
// on a channel (the guide title does not parse as a matchup).
func rebuildTestServer(t *testing.T, provider sportsProvider) *HTTPRoutesServer {
	t.Helper()
	now := time.Now()
	start := now.Add(2 * time.Hour)
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "channel:habs", Name: "NHL 05: Montreal Canadiens vs Toronto Maple Leafs"}},
		Programs: []model.Program{{ID: "program:habs", ChannelID: "channel:habs", Title: "Hockey Night", StartUnix: start.Unix(), EndUnix: start.Add(3 * time.Hour).Unix()}},
	}})
	server := NewHTTPRoutesServer(store)
	server.sportsProvider = provider
	server.sportsImages = newSportsImageCache(t.TempDir(), nil)
	return server
}

func rebuildTestEvent() SportsEvent {
	start := time.Now().Add(2 * time.Hour)
	return SportsEvent{
		ID: "provider:habs", LeagueID: "nhl", LeagueName: "NHL", Name: "Toronto Maple Leafs at Montreal Canadiens", StartUnix: start.Unix(),
		Home: SportsTeam{Name: "Montreal Canadiens"}, Away: SportsTeam{Name: "Toronto Maple Leafs"},
	}
}

func TestSportsMatchingDeadlineIsIndependentOfProviderFetch(t *testing.T) {
	t.Parallel()
	server := rebuildTestServer(t, slowSportsProvider{staticSportsProvider: staticSportsProvider{events: []SportsEvent{rebuildTestEvent()}}, delay: 300 * time.Millisecond})
	// The provider takes longer than the whole match budget; matching must
	// still get its full window afterwards.
	payload := server.sportsPayloadWithBudget(context.Background(), true, sportsBuildBudget{Provider: 2 * time.Second, Match: 250 * time.Millisecond})
	if payload.Incomplete {
		t.Fatal("matching must not inherit the time the provider fetch used")
	}
	if len(payload.Events) != 1 || len(payload.Events[0].Channels) != 1 || payload.Events[0].Channels[0].ID != "channel:habs" {
		t.Fatalf("expected the provider game on its channel, got %+v", payload.Events)
	}
}

func TestSportsRebuildCutShortIsMarkedIncompleteAndRetriedSoon(t *testing.T) {
	t.Parallel()
	server := rebuildTestServer(t, staticSportsProvider{events: []SportsEvent{rebuildTestEvent()}})
	server.sportsPrepared.budget = sportsBuildBudget{Match: time.Nanosecond}
	server.sportsPrepared.Refreshing = true
	before := time.Now()
	server.rebuildSportsPayload(true)

	server.sportsPreparedMu.Lock()
	prepared := server.sportsPrepared
	server.sportsPreparedMu.Unlock()
	if !prepared.Ready || !prepared.Incomplete || !prepared.Payload.Incomplete || prepared.Refreshing {
		t.Fatalf("a cut-short first build should be served but marked incomplete: %+v", prepared)
	}
	if ttl := prepared.ExpiresAfter.Sub(before); ttl > sportsIncompleteRetry+time.Second {
		t.Fatalf("incomplete payload cached for %s, want a retry within %s", ttl, sportsIncompleteRetry)
	}
}

func TestSportsRebuildKeepsPreviousCompletePayloadWhenCutShort(t *testing.T) {
	t.Parallel()
	server := rebuildTestServer(t, staticSportsProvider{events: []SportsEvent{rebuildTestEvent()}})
	server.rebuildSportsPayload(true)
	server.sportsPreparedMu.Lock()
	complete := cloneSportsPayload(server.sportsPrepared.Payload)
	if server.sportsPrepared.Incomplete || len(complete.Events) != 1 || len(complete.Events[0].Channels) != 1 {
		server.sportsPreparedMu.Unlock()
		t.Fatalf("baseline rebuild must be complete with a matched channel: %+v", complete.Events)
	}
	server.sportsPrepared.budget = sportsBuildBudget{Match: time.Nanosecond}
	server.sportsPrepared.Refreshing = true
	server.sportsPreparedMu.Unlock()

	before := time.Now()
	server.rebuildSportsPayload(true)
	server.sportsPreparedMu.Lock()
	prepared := server.sportsPrepared
	server.sportsPreparedMu.Unlock()
	if prepared.Incomplete || prepared.Payload.Incomplete || prepared.Refreshing {
		t.Fatalf("the previous complete payload must stay in place: %+v", prepared)
	}
	if len(prepared.Payload.Events) != 1 || len(prepared.Payload.Events[0].Channels) != 1 {
		t.Fatalf("partial matching replaced the complete payload: %+v", prepared.Payload.Events)
	}
	if ttl := prepared.ExpiresAfter.Sub(before); ttl > sportsIncompleteRetry+time.Second {
		t.Fatalf("an incomplete rebuild must retry within %s, got %s", sportsIncompleteRetry, ttl)
	}
}
