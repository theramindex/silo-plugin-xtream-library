package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/cache"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
)

type staticSportsProvider struct {
	events []SportsEvent
	err    error
}

func (p staticSportsProvider) Events(context.Context, time.Time) ([]SportsEvent, error) {
	return cloneSportsEvents(p.events), p.err
}

func (p staticSportsProvider) Source() string {
	return "test"
}

func TestHTTPRoutesServerSportsMatchesChannelsAndFavoriteTeams(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source: model.LiveTVSource(model.SourceModeXtream),
			Channels: []model.Channel{
				{ID: "ch:fs1", Name: "FOX Sports 1", CategoryID: "world", CategoryName: "World Cup"},
				{ID: "ch:fs1", Name: "FOX Sports 1", CategoryID: "world", CategoryName: "World Cup"},
				{ID: "ch:arg", Name: "Argentina Deportes", CategoryID: "arg", CategoryName: "Sports | Argentina"},
				{ID: "ch:news", Name: "News Now", CategoryID: "news", CategoryName: "News"},
			},
			Programs: []model.Program{
				{ID: "p:1", ChannelID: "ch:fs1", Title: "Argentina vs Brazil", StartUnix: 1700000000, EndUnix: 1700007200},
				{ID: "p:2", ChannelID: "ch:news", Title: "Morning News", StartUnix: 1700000000, EndUnix: 1700007200},
			},
			Content: model.ContentState{
				LiveCategories: []model.Category{
					{ID: "world", Name: "World Cup", Kind: "live"},
					{ID: "arg", Name: "Sports | Argentina", Kind: "live"},
					{ID: "news", Name: "News", Kind: "live"},
				},
			},
		},
	})
	server := NewHTTPRoutesServer(store)
	server.sportsProvider = staticSportsProvider{events: []SportsEvent{{
		ID:         "event:1",
		LeagueID:   "world-cup",
		LeagueName: "World Cup",
		Name:       "Argentina vs Brazil",
		ShortName:  "ARG vs BRA",
		Status:     "pre",
		StatusText: "Tonight",
		StartUnix:  1700000000,
		Home:       SportsTeam{ID: "team:arg", Name: "Argentina", Abbreviation: "ARG"},
		Away:       SportsTeam{ID: "team:bra", Name: "Brazil", Abbreviation: "BRA"},
	}}}

	payload := fetchSportsPayload(t, server)
	if payload.Source != "test" || len(payload.Events) != 1 {
		t.Fatalf("unexpected sports payload: %+v", payload)
	}
	event := payload.Events[0]
	if event.Home.Favorite || event.Away.Favorite {
		t.Fatalf("teams should not start as favorites: %+v", event)
	}
	assertSportsMatch(t, event.Channels, "ch:fs1")
	assertSportsMatch(t, event.Channels, "ch:arg")
	assertNoSportsMatch(t, event.Channels, "ch:news")
	matchCount := 0
	for _, match := range event.Channels {
		if match.ID == "ch:fs1" {
			matchCount++
		}
	}
	if matchCount != 1 {
		t.Fatalf("expected duplicate channel IDs to collapse, got %+v", event.Channels)
	}

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: http.MethodPost,
		Path:   "/dispatcharr/api/sports/favorites",
		Body:   []byte(`{"teamId":"team:arg","enabled":true}`),
	})
	if err != nil {
		t.Fatalf("favorite route: %v", err)
	}
	if response.GetStatusCode() != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", response.GetStatusCode(), string(response.GetBody()))
	}

	payload = fetchSportsPayload(t, server)
	if payload.Events[0].Home.Favorite || payload.Events[0].Away.Favorite {
		t.Fatalf("sports payload must remain user-neutral: %+v", payload.Events[0])
	}
}

func TestMatchSportsChannelsDoesNotUseLeagueOnlyGroups(t *testing.T) {
	t.Parallel()

	snapshot := cache.Snapshot{
		Catalog: model.CatalogState{
			Channels: []model.Channel{
				{ID: "ch:ari", Name: "Arizona Team Feed", CategoryID: "ari", CategoryName: "US Sports | NFL Teams | Arizona Cardinals"},
				{ID: "ch:lac", Name: "Los Angeles Team Feed", CategoryID: "lac", CategoryName: "US Sports | NFL Teams | Los Angeles Chargers"},
				{ID: "ch:atl", Name: "Atlanta Falcons", CategoryID: "atl", CategoryName: "US Sports | NFL Teams | Atlanta Falcons"},
				{ID: "ch:nfl", Name: "NFL Network", CategoryID: "nfl", CategoryName: "US Sports | NFL Teams"},
			},
			Content: model.ContentState{
				LiveCategories: []model.Category{
					{ID: "ari", Name: "US Sports | NFL Teams | Arizona Cardinals", Kind: "live"},
					{ID: "lac", Name: "US Sports | NFL Teams | Los Angeles Chargers", Kind: "live"},
					{ID: "atl", Name: "US Sports | NFL Teams | Atlanta Falcons", Kind: "live"},
					{ID: "nfl", Name: "US Sports | NFL Teams", Kind: "live"},
				},
			},
		},
	}
	event := SportsEvent{
		ID:         "event:nfl",
		LeagueID:   "nfl",
		LeagueName: "NFL",
		Name:       "Arizona Cardinals at Los Angeles Chargers",
		ShortName:  "ARI @ LAC",
		StartUnix:  1700000000,
		Home:       SportsTeam{ID: "team:lac", Name: "Los Angeles Chargers", Abbreviation: "LAC"},
		Away:       SportsTeam{ID: "team:ari", Name: "Arizona Cardinals", Abbreviation: "ARI"},
	}

	matches := matchSportsChannels(event, snapshot)
	assertSportsMatch(t, matches, "ch:ari")
	assertSportsMatch(t, matches, "ch:lac")
	assertNoSportsMatch(t, matches, "ch:atl")
	assertNoSportsMatch(t, matches, "ch:nfl")
}

func TestMatchSportsChannelsRejectsWeakGuideOnlyMatches(t *testing.T) {
	t.Parallel()

	snapshot := cache.Snapshot{
		Catalog: model.CatalogState{
			Channels: []model.Channel{
				{ID: "ch:sport", Name: "Sport1", CategoryID: "sports", CategoryName: "International Sports | Germany"},
				{ID: "ch:fox", Name: "FOX 7", CategoryID: "fox", CategoryName: "US | Locals | FOX"},
				{ID: "ch:starz", Name: "Starz Encore Westerns", CategoryID: "movies", CategoryName: "US | Movies"},
			},
			Programs: []model.Program{
				{ID: "p:sport", ChannelID: "ch:sport", Title: "Ecuador vs Mexico", StartUnix: 1700000000, EndUnix: 1700007200},
				{ID: "p:fox", ChannelID: "ch:fox", Title: "FIFA World Cup 2026: Ecuador vs. Mexico", StartUnix: 1700000000, EndUnix: 1700007200},
				{ID: "p:starz", ChannelID: "ch:starz", Title: "Western Movie", Summary: "A classic adventure near Mexico.", StartUnix: 1700000000, EndUnix: 1700007200},
			},
			Content: model.ContentState{
				LiveCategories: []model.Category{
					{ID: "sports", Name: "International Sports | Germany", Kind: "live"},
					{ID: "fox", Name: "US | Locals | FOX", Kind: "live"},
					{ID: "movies", Name: "US | Movies", Kind: "live"},
				},
			},
		},
	}
	event := SportsEvent{
		ID:         "event:world-cup",
		LeagueID:   "world-cup",
		LeagueName: "World Cup",
		Name:       "Ecuador vs Mexico",
		ShortName:  "ECU @ MEX",
		StartUnix:  1700000000,
		Home:       SportsTeam{ID: "team:mex", Name: "Mexico", Abbreviation: "MEX"},
		Away:       SportsTeam{ID: "team:ecu", Name: "Ecuador", Abbreviation: "ECU"},
	}

	matches := matchSportsChannels(event, snapshot)
	assertSportsMatch(t, matches, "ch:sport")
	assertSportsMatch(t, matches, "ch:fox")
	assertNoSportsMatch(t, matches, "ch:starz")
}

func TestHTTPRoutesServerSportsUsesStaleCacheOnProviderError(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	server := NewHTTPRoutesServer(store)
	server.sportsProvider = staticSportsProvider{events: []SportsEvent{{ID: "event:cached", LeagueID: "nfl", LeagueName: "NFL", Name: "Jets at Giants", StartUnix: 1700000000}}}
	first := fetchSportsPayload(t, server)
	if len(first.Events) != 1 {
		t.Fatalf("expected cached event seed, got %+v", first)
	}
	server.sportsProvider = staticSportsProvider{err: errors.New("provider down")}
	server.sportsCache.ExpiresAfter = time.Now().Add(-time.Second)
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/dispatcharr/api/sports"})
	if err != nil {
		t.Fatalf("sports route: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	var payload SportsPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Error == "" || len(payload.Events) != 1 || payload.Events[0].ID != "event:cached" {
		t.Fatalf("expected stale cached event with error, got %+v", payload)
	}
}

func TestESPNSportsEventParsesStartWithoutSeconds(t *testing.T) {
	t.Parallel()

	event := espnEvent{
		ID:        "401",
		Name:      "Panama vs Croatia",
		ShortName: "PAN vs CRO",
		Date:      "2026-06-26T22:35Z",
		Status:    espnStatus{Type: espnStatusType{State: "pre", Detail: "6:35 PM"}},
	}
	converted := event.sportsEvent(espnLeagueConfig{ID: "world-cup", Name: "World Cup"})
	expected := time.Date(2026, 6, 26, 22, 35, 0, 0, time.UTC).Unix()
	if converted.StartUnix != expected {
		t.Fatalf("expected parsed start %d, got %d", expected, converted.StartUnix)
	}
}

func TestESPNSportsProviderDoesNotFallbackToFetchTimeForUnknownStart(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"events":[{"id":"402","name":"Time Unknown FC at TBD United","shortName":"TBD @ TBD","date":"not-a-date","status":{"type":{"state":"pre","detail":"TBD"}}}]}`))
	})
	testServer := httptest.NewServer(handler)
	defer testServer.Close()

	provider := espnSportsProvider{
		client: testServer.Client(),
		endpointBuilder: func(league espnLeagueConfig) string {
			return testServer.URL
		},
	}
	events, err := provider.leagueEvents(context.Background(), espnLeagueConfig{ID: "soccer", Sport: "soccer", League: "test", Name: "Test League"}, time.Date(2026, 6, 26, 20, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("league events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %+v", events)
	}
	if events[0].StartUnix != 0 {
		t.Fatalf("unknown ESPN start should stay 0, got %d", events[0].StartUnix)
	}
}

func fetchSportsPayload(t *testing.T, server *HTTPRoutesServer) SportsPayload {
	t.Helper()
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/dispatcharr/api/sports"})
	if err != nil {
		t.Fatalf("sports route: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.GetStatusCode(), string(response.GetBody()))
	}
	var payload SportsPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

func assertSportsMatch(t *testing.T, matches []SportsChannelMatch, channelID string) {
	t.Helper()
	for _, match := range matches {
		if match.ID == channelID {
			return
		}
	}
	t.Fatalf("expected %s in sports matches: %+v", channelID, matches)
}

func assertNoSportsMatch(t *testing.T, matches []SportsChannelMatch, channelID string) {
	t.Helper()
	for _, match := range matches {
		if match.ID == channelID {
			t.Fatalf("did not expect %s in sports matches: %+v", channelID, matches)
		}
	}
}
