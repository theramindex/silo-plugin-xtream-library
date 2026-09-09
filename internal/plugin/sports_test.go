package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"google.golang.org/protobuf/types/known/structpb"
)

type staticSportsProvider struct {
	events []SportsEvent
	err    error
}

func TestSportsPayloadKeepsGuideMatchedScoresAfterDeadline(t *testing.T) {
	t.Parallel()
	now := time.Now()
	start := now.Add(-time.Hour).Unix()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "nbc", Name: "NBC"}},
		Programs: []model.Program{{ID: "game", ChannelID: "nbc", Title: "College Football: Western Michigan at Michigan", StartUnix: start, EndUnix: now.Add(time.Hour).Unix()}},
	}})
	server := newEnabledSportsTestServer(store)
	server.sportsProvider = staticSportsProvider{events: []SportsEvent{{
		ID: "provider-game", Name: "Michigan vs Western Michigan", LeagueName: "NCAA Division 1", StartUnix: start, Status: "scheduled",
		Home: SportsTeam{Name: "Michigan"}, Away: SportsTeam{Name: "Western Michigan"}, HomeScore: "7", AwayScore: "3",
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	payload := server.sportsPayload(ctx, true)
	if len(payload.Events) != 1 || payload.Events[0].HomeScore != "7" || payload.Events[0].AwayScore != "3" || !payload.Events[0].Live || len(payload.Events[0].Channels) != 1 {
		t.Fatalf("guide-matched scores must survive the matching deadline: %+v", payload.Events)
	}
	index := newSportsChannelIndex(store.Current())
	server.sportsImages = newSportsImageCache(t.TempDir(), nil)
	withImages := server.proxySportsEventImages(payload.Events)
	if withImages[0].Home.LogoURL != payload.Events[0].Home.LogoURL {
		t.Fatal("public college identity logos must stay usable when provider scores arrive")
	}
	if payload.Events[0].LeagueName != "College Football" || payload.Events[0].Home.ID != stableSportsTeamID(SportsTeam{Name: "Michigan", Abbreviation: "M"}) || payload.Events[0].Home.LogoURL == "" {
		t.Fatal("provider scores must preserve guide college identity and saved passes")
	}
	matches, _ := index.MatchDetailedContext(ctx, payload.Events[0])
	if len(matches) != 0 {
		t.Fatal("expired matching must stop scanning channels")
	}
}

type deadlineSportsProvider struct {
	maxRemaining time.Duration
}

type blockingSportsProvider struct {
	events  []SportsEvent
	started chan struct{}
	release chan struct{}
	once    sync.Once
	calls   atomic.Int32
}

type countingSportsProvider struct {
	events []SportsEvent
	calls  atomic.Int32
}

type rosterSportsProvider struct {
	staticSportsProvider
	teams []SportsTeam
}

func (p rosterSportsProvider) LeagueTeams(context.Context, string) ([]SportsTeam, error) {
	return append([]SportsTeam(nil), p.teams...), nil
}

func (p *countingSportsProvider) Events(context.Context, time.Time) ([]SportsEvent, error) {
	p.calls.Add(1)
	return cloneSportsEvents(p.events), nil
}

func (*countingSportsProvider) Source() string {
	return "counting-test"
}

func (p *blockingSportsProvider) Events(ctx context.Context, _ time.Time) ([]SportsEvent, error) {
	p.calls.Add(1)
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
		return cloneSportsEvents(p.events), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (*blockingSportsProvider) Source() string {
	return "blocking-test"
}

func (p *deadlineSportsProvider) Events(ctx context.Context, _ time.Time) ([]SportsEvent, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, errors.New("sports provider context has no deadline")
	}
	p.maxRemaining = time.Until(deadline)
	return nil, errors.New("provider unavailable")
}

func (*deadlineSportsProvider) Source() string {
	return "deadline-test"
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
	server := newEnabledSportsTestServer(store)
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

func TestSportsPayloadFallsBackToGuideMatchups(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Source: model.LiveTVSource(model.SourceModeXtream),
		Channels: []model.Channel{
			{ID: "ch:wnba-1", Name: "WNBA League Pass", CategoryID: "sports", CategoryName: "US TV | Sports"},
			{ID: "ch:wnba-2", Name: "ESPN", CategoryID: "sports", CategoryName: "US TV | Sports"},
			{ID: "ch:news", Name: "News", CategoryID: "news", CategoryName: "News"},
		},
		Programs: []model.Program{
			{ID: "p:wnba-1", ChannelID: "ch:wnba-1", Title: "WNBA Basketball: Indiana Fever vs Las Vegas Aces", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(90 * time.Minute).Unix()},
			{ID: "p:wnba-2", ChannelID: "ch:wnba-2", Title: "WNBA Basketball: Indiana Fever vs Las Vegas Aces", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(90 * time.Minute).Unix()},
			{ID: "p:news", ChannelID: "ch:news", Title: "Morning News", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(30 * time.Minute).Unix()},
		},
		Content: model.ContentState{LiveCategories: []model.Category{
			{ID: "sports", Name: "US TV | Sports", Kind: "live"},
			{ID: "news", Name: "News", Kind: "live"},
		}},
	}})
	server := newEnabledSportsTestServer(store)
	server.sportsProvider = staticSportsProvider{}

	payload := server.sportsPayload(context.Background(), false)
	if payload.Source != "EPG fallback" || len(payload.Events) != 1 {
		t.Fatalf("expected one EPG fallback event, got %+v", payload)
	}
	event := payload.Events[0]
	if event.LeagueID != "wnba" || event.Away.Name != "Indiana Fever" || event.Home.Name != "Las Vegas Aces" || !event.Live {
		t.Fatalf("unexpected fallback event: %+v", event)
	}
	assertSportsMatch(t, event.Channels, "ch:wnba-1")
	assertSportsMatch(t, event.Channels, "ch:wnba-2")
	assertNoSportsMatch(t, event.Channels, "ch:news")

	server = newEnabledSportsTestServer(store)
	server.sportsProvider = staticSportsProvider{events: []SportsEvent{{
		ID:         "sportarr:wnba-game",
		LeagueID:   "wnba",
		LeagueName: "WNBA",
		Name:       "Indiana Fever at Las Vegas Aces",
		ShortName:  "IND vs LV",
		StartUnix:  now.Add(-30 * time.Minute).Unix(),
		Live:       true,
		Status:     "live",
		Away:       SportsTeam{ID: "team:indiana", Name: "Indiana Fever", Abbreviation: "IND"},
		Home:       SportsTeam{ID: "team:las-vegas", Name: "Las Vegas Aces", Abbreviation: "LV"},
	}}}
	payload = server.sportsPayload(context.Background(), false)
	if payload.Source != "test + EPG" || len(payload.Events) != 1 || payload.Events[0].ID != "sportarr:wnba-game" {
		t.Fatalf("expected EPG to enrich the Sportarr event without duplicating it, got %+v", payload)
	}
	assertSportsMatch(t, payload.Events[0].Channels, "ch:wnba-1")
	assertSportsMatch(t, payload.Events[0].Channels, "ch:wnba-2")
}

func TestNormalizeSportsEventFreshnessExpiresStaleLiveGames(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		event     SportsEvent
		completed bool
	}{
		{
			name: "recent live event remains live",
			event: SportsEvent{
				Live:       true,
				Status:     "live",
				StatusText: "Top 4th",
				StartUnix:  now.Add(-2 * time.Hour).Unix(),
				EndUnix:    now.Add(time.Hour).Unix(),
			},
		},
		{
			name: "past end time expires",
			event: SportsEvent{
				Live:       true,
				Status:     "live",
				StatusText: "Top 3rd",
				StartUnix:  now.Add(-6 * time.Hour).Unix(),
				EndUnix:    now.Add(-3 * time.Hour).Unix(),
			},
			completed: true,
		},
		{
			name: "missing end time eventually expires",
			event: SportsEvent{
				Live:       true,
				Status:     "live",
				StatusText: "Live",
				StartUnix:  now.Add(-19 * time.Hour).Unix(),
			},
			completed: true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeSportsEventFreshness(test.event, now)
			if test.completed {
				if got.Live || !got.Completed || got.Status != "final" || got.StatusText != "Final" {
					t.Fatalf("expected stale event to become final, got %+v", got)
				}
				return
			}
			if !got.Live || got.Completed {
				t.Fatalf("expected current event to remain live, got %+v", got)
			}
		})
	}
}

func TestNormalizeSportsEventsAddsGameThumbsIdentityFallbacks(t *testing.T) {
	t.Parallel()

	events := normalizeSportsEvents([]SportsEvent{
		{
			LeagueID: "nba", LeagueName: "NBA", Name: "NBA Playback: Heat vs. Pistons",
			Away: SportsTeam{Name: "Heat"}, Home: SportsTeam{Name: "Pistons"},
		},
		{
			LeagueID: "sports", LeagueName: "Sports", Name: "NWSL Soccer: Teal Rising Cup: Kansas City Current vs. Palmeiras",
			Away: SportsTeam{Name: "Kansas City Current"}, Home: SportsTeam{Name: "Palmeiras"},
		},
		{
			LeagueID: "nfl", LeagueName: "NFL", Name: "Existing artwork",
			LeagueLogoURL: "https://images.example/nfl.png",
			Away:          SportsTeam{Name: "Cowboys", LogoURL: "https://images.example/cowboys.png"},
			Home:          SportsTeam{Name: "Cardinals"},
		},
		{
			LeagueID: "sports", LeagueName: "Sports", Name: "Unknown local competition",
			Away: SportsTeam{Name: "Alpha"}, Home: SportsTeam{Name: "Beta"},
		},
		{
			LeagueID: "bra.1", LeagueName: "Brazil Serie A", Name: "São Paulo vs. Grêmio",
			Away: SportsTeam{Name: "São Paulo"}, Home: SportsTeam{Name: "Grêmio"},
		},
		{
			LeagueID: "efl-trophy", LeagueName: "EFL Trophy", Name: "Bristol Rovers vs Chelsea U21",
			Away: SportsTeam{Name: "Chelsea U21"}, Home: SportsTeam{Name: "Bristol Rovers"},
		},
		{
			LeagueID: "sports", LeagueName: "Sports", Name: "Best of Islanders: New York Islanders at Tampa Bay Lightning",
			Away: SportsTeam{Name: "New York Islanders"}, Home: SportsTeam{Name: "Tampa Bay Lightning"},
		},
		{
			LeagueID: "sports", LeagueName: "Sports", Name: "Best of the Knicks: Orlando Magic at New York Knicks",
			Away: SportsTeam{Name: "Orlando Magic"}, Home: SportsTeam{Name: "New York Knicks"},
		},
		{
			LeagueID: "college-football", LeagueName: "College Football", Name: "CFP National Championship: Miami vs. Indiana",
			Away: SportsTeam{Name: "Miami"}, Home: SportsTeam{Name: "Indiana"},
		},
		{
			LeagueID: "nascar-cup-series", LeagueName: "NASCAR Cup Series", SportName: "Motorsport", Name: "NCS Race at New Hampshire",
			Away: SportsTeam{Name: "NCS Race"}, Home: SportsTeam{Name: "New Hampshire"},
		},
		{
			LeagueID: "cricket", LeagueName: "Cricket", SportName: "Cricket", Name: "New Zealand vs Victoria",
			Away: SportsTeam{Name: "New Zealand"}, Home: SportsTeam{Name: "Victoria"},
		},
		{
			LeagueID: "afl", LeagueName: "AFL", SportName: "Australian Football", Name: "Essendon vs Port Adelaide",
			Away: SportsTeam{Name: "Essendon"}, Home: SportsTeam{Name: "Port Adelaide"},
		},
		{
			LeagueID: "english-league-1", LeagueName: "English League 1", SportName: "Soccer", Name: "Cambridge United vs Huddersfield Town",
			Away: SportsTeam{Name: "Huddersfield Town"}, Home: SportsTeam{Name: "Cambridge United"},
		},
		{
			LeagueID: "boxing", LeagueName: "Boxing", SportName: "Combat Sports", Name: "Canelo Alvarez vs. Cotto & Cesar Chavez Jr",
			Away: SportsTeam{Name: "Canelo Alvarez"}, Home: SportsTeam{Name: "Cotto & Cesar Chavez Jr"},
		},
		{
			LeagueID: "college-football", LeagueName: "College Football", SportName: "Football", Name: "Oregon vs Indiana",
			Away: SportsTeam{Name: "Oregon"}, Home: SportsTeam{Name: "Indiana"},
		},
		{
			LeagueID: "sports", LeagueName: "Sports", SportName: "Volleyball", Name: "Canada vs Dominican Republic",
			Away: SportsTeam{Name: "Canada"}, Home: SportsTeam{Name: "Dominican Republic"},
		},
		{
			LeagueID: "sports", LeagueName: "Sports", SportName: "Soccer", Name: "Women's College Soccer: Florida State vs Florida",
			Away: SportsTeam{Name: "Florida State"}, Home: SportsTeam{Name: "Florida"},
		},
		{
			LeagueID: "sports", LeagueName: "Sports", SportName: "Sports", Name: "Florida vs Oregon at College Park",
			Away: SportsTeam{Name: "Florida"}, Home: SportsTeam{Name: "Oregon"},
		},
	})

	if got := events[0].LeagueLogoURL; got != "https://game-thumbs.swvn.io/nba/logo.png" {
		t.Fatalf("expected NBA league fallback, got %q", got)
	}
	if got := events[0].Away.LogoURL; got != "https://game-thumbs.swvn.io/nba/heat/logo.png?variant=dark" {
		t.Fatalf("expected Heat logo fallback, got %q", got)
	}
	if got := events[0].Home.LogoURL; got != "https://game-thumbs.swvn.io/nba/pistons/logo.png?variant=dark" {
		t.Fatalf("expected Pistons logo fallback, got %q", got)
	}
	if got := events[1].LeagueLogoURL; got != "https://game-thumbs.swvn.io/usa.nwsl/logo.png" {
		t.Fatalf("expected NWSL league fallback, got %q", got)
	}
	if got := events[1].Away.LogoURL; got != "https://game-thumbs.swvn.io/usa.nwsl/kansas-city-current/logo.png?variant=dark" {
		t.Fatalf("expected Kansas City Current logo fallback, got %q", got)
	}
	if got := events[1].Home.LogoURL; got != "https://game-thumbs.swvn.io/bra.1/palmeiras/logo.png?variant=dark" {
		t.Fatalf("expected Palmeiras cross-league fallback, got %q", got)
	}
	if events[2].LeagueLogoFallbackURL != "https://images.example/nfl.png" || events[2].Away.LogoFallbackURL != "https://images.example/cowboys.png" {
		t.Fatalf("expected upstream artwork to remain available as fallback, got %+v", events[2])
	}
	if events[3].LeagueLogoURL != "" || events[3].Away.LogoURL != "" || events[3].Home.LogoURL != "" {
		t.Fatalf("expected unknown competition to retain honest empty artwork, got %+v", events[3])
	}
	if got := events[4].Away.LogoURL; got != "https://game-thumbs.swvn.io/bra.1/sao-paulo/logo.png?variant=dark" {
		t.Fatalf("expected diacritics removed from São Paulo fallback, got %q", got)
	}
	if got := events[4].Home.LogoURL; got != "https://game-thumbs.swvn.io/bra.1/gremio/logo.png?variant=dark" {
		t.Fatalf("expected diacritics removed from Grêmio fallback, got %q", got)
	}
	if got := events[5].Away.LogoURL; got != "https://game-thumbs.swvn.io/epl/chelsea/logo.png?variant=dark" {
		t.Fatalf("expected Chelsea U21 to use the senior club crest, got %q", got)
	}
	if got := events[5].Home.LogoURL; got != "https://game-thumbs.swvn.io/epl/bristol-rovers/logo.png?variant=dark" {
		t.Fatalf("expected Bristol Rovers to use the English pyramid crest, got %q", got)
	}
	if got := events[6].Away.LogoURL; got != "https://game-thumbs.swvn.io/nhl/new-york-islanders/logo.png?variant=dark" {
		t.Fatalf("expected generic sports Islanders event to recover the NHL namespace, got %q", got)
	}
	if got := events[6].Home.LogoURL; got != "https://game-thumbs.swvn.io/nhl/tampa-bay-lightning/logo.png?variant=dark" {
		t.Fatalf("expected generic sports Lightning event to recover the NHL namespace, got %q", got)
	}
	if got := events[7].Away.LogoURL; got != "https://game-thumbs.swvn.io/nba/orlando-magic/logo.png?variant=dark" {
		t.Fatalf("expected generic sports Magic event to recover the NBA namespace, got %q", got)
	}
	if got := events[7].Home.LogoURL; got != "https://game-thumbs.swvn.io/nba/new-york-knicks/logo.png?variant=dark" {
		t.Fatalf("expected generic sports Knicks event to recover the NBA namespace, got %q", got)
	}
	if got := events[8].LeagueLogoURL; got != "https://game-thumbs.swvn.io/ncaaf/logo.png" {
		t.Fatalf("expected College Football to use the canonical NCAAF league mark, got %q", got)
	}
	if got := events[8].Away.LogoURL; got != "https://game-thumbs.swvn.io/ncaaf/miami/logo.png?variant=dark" {
		t.Fatalf("expected College Football teams to use the NCAAF namespace, got %q", got)
	}
	if got := events[9].LeagueLogoURL; got != "https://game-thumbs.swvn.io/NASCAR/logo.png" {
		t.Fatalf("expected NASCAR Cup Series to use the NASCAR league mark, got %q", got)
	}
	if got := events[10].Away.LogoURL; got != "https://game-thumbs.swvn.io/country/new-zealand/logo.png?variant=dark" {
		t.Fatalf("expected an international cricket side to use its country flag, got %q", got)
	}
	if got := events[10].Home.LogoURL; got != "" {
		t.Fatalf("expected domestic Victoria to avoid a false country flag, got %q", got)
	}
	if got := events[11].LeagueLogoURL; got != "https://r2.thesportsdb.com/images/media/league/badge/wvx4721525519372.png" {
		t.Fatalf("expected AFL league identity, got %q", got)
	}
	if got := events[11].Away.LogoURL; got != "https://squiggle.com.au/wp-content/themes/squiggle/assets/images/Essendon.png" {
		t.Fatalf("expected Essendon crest from Squiggle, got %q", got)
	}
	if got := events[11].Home.LogoURL; got != "https://squiggle.com.au/wp-content/themes/squiggle/assets/images/PortAdelaide.png" {
		t.Fatalf("expected Port Adelaide crest from Squiggle, got %q", got)
	}
	if got := events[12].LeagueLogoURL; got != "https://game-thumbs.swvn.io/league-one/logo.png" {
		t.Fatalf("expected English League One to use the GameThumbs league namespace, got %q", got)
	}
	if got := events[12].Away.LogoURL; got != "https://game-thumbs.swvn.io/league-one/huddersfield-town/logo.png?variant=dark" {
		t.Fatalf("expected Huddersfield Town crest fallback, got %q", got)
	}
	if got := events[12].Home.LogoURL; got != "https://game-thumbs.swvn.io/league-one/cambridge-united/logo.png?variant=dark" {
		t.Fatalf("expected Cambridge United crest fallback, got %q", got)
	}
	if got := events[13].Home.LogoFallbackURL; got != "https://game-thumbs.swvn.io/boxing/logo.png" {
		t.Fatalf("expected a compound boxing opponent to use an honest boxing mark, got %q", got)
	}
	if got := events[14].Away.LogoFallbackURL; got != "https://a.espncdn.com/i/teamlogos/ncaa/500/2483.png" {
		t.Fatalf("expected Oregon to use its NCAA mark, got %q", got)
	}
	if got := events[14].Home.LogoFallbackURL; got != "https://a.espncdn.com/i/teamlogos/ncaa/500/84.png" {
		t.Fatalf("expected Indiana to use its NCAA mark, got %q", got)
	}
	if got := events[15].Away.LogoURL; got != "https://game-thumbs.swvn.io/country/canada/logo.png?variant=dark" {
		t.Fatalf("expected Canada to use its country flag, got %q", got)
	}
	if got := events[15].Home.LogoURL; got != "https://game-thumbs.swvn.io/country/dominican-republic/logo.png?variant=dark" {
		t.Fatalf("expected Dominican Republic to use its country flag, got %q", got)
	}
	if got := events[16].Away.LogoFallbackURL; got != "https://a.espncdn.com/i/teamlogos/ncaa/500/52.png" {
		t.Fatalf("expected Florida State to use its NCAA mark, got %q", got)
	}
	if got := events[16].Home.LogoFallbackURL; got != "https://a.espncdn.com/i/teamlogos/ncaa/500/57.png" {
		t.Fatalf("expected Florida to use its NCAA mark, got %q", got)
	}
	if events[17].Away.LogoURL != "" || events[17].Home.LogoURL != "" {
		t.Fatalf("expected a generic College Park event to avoid NCAA identity, got %+v", events[17])
	}
}

func TestCollegeGamePassesSeparateSports(t *testing.T) {
	t.Parallel()
	cases := []struct{ title, league string }{
		{"College Football: Michigan at Notre Dame", "College Football"},
		{"Men's College Basketball: Michigan at Notre Dame", "Men's College Basketball"},
		{"Women's College Basketball: Michigan at Notre Dame", "Women's College Basketball"},
		{"Men's College Soccer: Michigan at Notre Dame", "Men's College Soccer"},
		{"Women's College Volleyball: Michigan at Notre Dame", "Women's College Volleyball"},
		{"NCAA Women's Field Hockey: Michigan at Notre Dame", "Women's College Field Hockey"},
		{"NCAA Softball: Michigan at Notre Dame", "College Softball"},
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		event := normalizeSportsEvents([]SportsEvent{{Name: tc.title, Home: SportsTeam{Name: "Michigan", Abbreviation: "M"}}})[0]
		if event.LeagueName != tc.league || seen[event.Home.ID] {
			t.Fatalf("college passes must distinguish competition: %+v", event)
		}
		seen[event.Home.ID] = true
		if tc.league != "College Football" && event.LeagueLogoFallbackURL != ncaaLeagueLogoURL {
			t.Fatalf("college competitions need an NCAA league mark: %s", event.LeagueName)
		}
		if tc.league == "College Football" && event.Home.ID != stableSportsTeamID(SportsTeam{Name: "Michigan", Abbreviation: "M"}) {
			t.Fatal("existing football passes must retain their identity")
		}
		roster := mergeSportsLeagueRosterTeams(event.LeagueID, event.LeagueName, event.SportName, []SportsTeam{event.Home})
		if len(roster) != 1 || roster[0].ID != event.Home.ID || normalizeSportsEvents([]SportsEvent{event})[0].Home.ID != event.Home.ID {
			t.Fatal("roster and repeated normalization must retain the competition identity")
		}
	}
}

func TestWorldCupCompetitionsRemainSeparate(t *testing.T) {
	t.Parallel()
	cases := []struct{ title, id, league, sport string }{
		{"Copa Mundial Femenina Sub-20 de la FIFA 2026 : Estados Unidos Sub-20 vs. Italia Sub-20", "fifa-womens-u20-world-cup", "FIFA Women's U-20 World Cup", "Soccer"},
		{"2026 FIFA U-20 Women's World Cup : USA U20 vs. Italy U20", "fifa-womens-u20-world-cup", "FIFA Women's U-20 World Cup", "Soccer"},
		{"2026 FIBA Women's Basketball World Cup : Puerto Rico vs. Belgium", "fiba-womens-world-cup", "FIBA Women's Basketball World Cup", "Basketball"},
		{"FIFA U20 World Cup: USA vs. Italy", "fifa-u20-world-cup", "FIFA U-20 World Cup", "Soccer"},
		{"FIFA Women's World Cup: USA vs. Italy", "fifa-womens-world-cup", "FIFA Women's World Cup", "Soccer"},
		{"FIBA Basketball World Cup: USA vs. Italy", "fiba-world-cup", "FIBA Basketball World Cup", "Basketball"},
		{"FIFA World Cup: USA vs. Italy", "world-cup", "World Cup", "Soccer"},
	}
	for _, tc := range cases {
		id, league, sport, matched := guideSportsLeague(tc.title)
		if !matched || id != tc.id || league != tc.league || sport != tc.sport {
			t.Fatalf("wrong competition for %q: %q %q %q", tc.title, id, league, sport)
		}
		// Previously cached/provider classifications must also be corrected.
		event := SportsEvent{Name: tc.title, LeagueID: "world-cup", LeagueName: "World Cup", SportName: "Soccer"}
		for range 2 {
			event = canonicalizeKnownSportsLeague(event)
			if event.LeagueID != tc.id || event.LeagueName != tc.league || event.SportName != tc.sport {
				t.Fatalf("incorrect or unstable canonical competition: %+v", event)
			}
		}
	}
}

func TestCollegeFieldHockeyClassification(t *testing.T) {
	t.Parallel()
	for _, title := range []string{
		"NCAA Women's Field Hockey: Michigan at Notre Dame",
		"Women's College Field Hockey: Michigan at Notre Dame",
		"NCAA Women’s Field Hockey: Michigan at Notre Dame",
	} {
		id, name, sport, ok := guideSportsLeague(title)
		if !ok || id != "college-womens-field-hockey" || name != "Women's College Field Hockey" || sport != "Field Hockey" {
			t.Fatalf("incorrect field hockey classification for %q: %q %q %q %v", title, id, name, sport, ok)
		}
	}
	id, _, sport, ok := guideSportsLeague("Women's College Hockey: Michigan at Notre Dame")
	if !ok || id != "college-womens-hockey" || sport != "Hockey" {
		t.Fatal("field hockey must remain separate from college hockey")
	}
}

func TestMichiganSchoolsHaveDistinctRosterLogos(t *testing.T) {
	t.Parallel()
	input := []SportsTeam{
		{ID: "michigan", Name: "Michigan", LogoURL: gameThumbsTeamLogoURL("ncaaf", "Michigan")},
		{ID: "western", Name: "Western Michigan", LogoURL: gameThumbsTeamLogoURL("ncaaf", "Western Michigan")},
		{ID: "central", Name: "Central Michigan", LogoURL: gameThumbsTeamLogoURL("ncaaf", "Central Michigan")},
	}
	teams := mergeSportsLeagueRosterTeams("ncaaf", "College Football", "Football", input)
	want := map[string]string{"Michigan": "130", "Western Michigan": "2711", "Central Michigan": "2117"}
	if len(teams) != 3 {
		t.Fatalf("schools must remain separate: %+v", teams)
	}
	for _, team := range teams {
		if team.LogoFallbackURL != ncaaTeamLogoBase+want[team.Name]+".png" {
			t.Errorf("wrong logo for %s: %s", team.Name, team.LogoURL)
		}
	}
}

func TestFormulaLeagueLogosSurvivePreparedPayload(t *testing.T) {
	t.Parallel()
	server := newEnabledSportsTestServer(nil)
	server.sportsImages = newSportsImageCache(t.TempDir(), nil)
	events := server.proxySportsEventImages(normalizeSportsEvents([]SportsEvent{
		{Name: "Formula 1: Italian Grand Prix", LeagueName: "Formula 1"},
		{Name: "Formula E: London E-Prix", LeagueName: "Formula E"},
	}))
	if events[0].LeagueLogoURL != gameThumbsLeagueLogoURL("F1") || events[0].LeagueLogoFallbackURL != formulaOneLeagueLogoURL || events[1].LeagueLogoURL != formulaELeagueLogoURL {
		t.Fatalf("public racing logos must remain loadable after preparation: %+v", events)
	}
	leagues := sportsLeagues([]SportsEvent{{LeagueID: "sports", LeagueName: "Sports"}})
	if len(leagues) != 1 || leagues[0].ID != "sports" || leagues[0].Name != "Other sports" || leagues[0].Description == "" {
		t.Fatal("unidentified broadcasts must remain available with an honest category label")
	}
}

func TestSportsIdentityFallbacksAddFormulaELeagueLogo(t *testing.T) {
	t.Parallel()

	event := applySportsIdentityFallbacks(SportsEvent{
		LeagueID:   "formula-e",
		LeagueName: "Formula E",
		SportName:  "Motorsport",
		Name:       "Formule : Formule E v Šanghaji",
	})

	const want = "https://upload.wikimedia.org/wikipedia/commons/8/8c/Formula-e-logo-championship_2023.svg"
	if event.LeagueLogoURL != want {
		t.Fatalf("expected Formula E series logo %q, got %q", want, event.LeagueLogoURL)
	}
}

func TestNormalizeSportsEventsCanonicalizesKnownLeagueIDs(t *testing.T) {
	t.Parallel()

	events := normalizeSportsEvents([]SportsEvent{
		{LeagueID: "provider:baseball", LeagueName: "MLB", SportName: "Baseball", Name: "Yankees at Red Sox", Live: true},
		{LeagueID: "mlb", LeagueName: "MLB", SportName: "Baseball", Name: "Cubs at Cardinals"},
		{LeagueID: "provider:wnba", LeagueName: "WNBA", SportName: "Basketball", Name: "Fever at Sky"},
		{LeagueID: "wnba", LeagueName: "WNBA", SportName: "Basketball", Name: "Liberty at Storm"},
	})

	leagues := sportsLeagues(events)
	if len(leagues) != 2 {
		t.Fatalf("expected provider and EPG league aliases to collapse into two canonical leagues, got %+v", leagues)
	}
	if leagues[0].ID != "mlb" || leagues[0].Name != "MLB" || leagues[0].LiveCount != 1 || leagues[0].UpcomingCount != 1 {
		t.Fatalf("expected one canonical MLB league with combined counts, got %+v", leagues[0])
	}
	if leagues[1].ID != "wnba" || leagues[1].Name != "WNBA" || leagues[1].UpcomingCount != 2 {
		t.Fatalf("expected one canonical WNBA league with combined counts, got %+v", leagues[1])
	}
}

func TestGuideSportsMatchupParsesQualifiedBroadcastTitles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		title     string
		wantAway  string
		wantHome  string
		wantMatch bool
	}{
		{
			name:      "college bowl prefix containing at",
			title:     "CFP Quarterfinal at the Capital One Orange Bowl : Oregon vs. Texas Tech",
			wantAway:  "Oregon",
			wantHome:  "Texas Tech",
			wantMatch: true,
		},
		{
			name:      "cricket suffix after matchup",
			title:     "Cricket Highlights : Bangladesh vs Australia: 2nd ODI",
			wantAway:  "Bangladesh",
			wantHome:  "Australia",
			wantMatch: true,
		},
		{
			name:      "cricket match number is not part of team name",
			title:     "Cricket Review : Top End T20 Series: New Zealand vs Victoria, Match 16",
			wantAway:  "New Zealand",
			wantHome:  "Victoria",
			wantMatch: true,
		},
		{
			name:      "cricket ordinal match suffix is event metadata",
			title:     "Cricket Highlights : The Hundred 2026: Leeds vs Super Giants - 10th Match",
			wantAway:  "Leeds",
			wantHome:  "Super Giants",
			wantMatch: true,
		},
		{
			name:      "cricket qualifier is event metadata",
			title:     "Cricket Highlights : MLC 2026: Unicorns vs Knight Riders - Qualifier",
			wantAway:  "Unicorns",
			wantHome:  "Knight Riders",
			wantMatch: true,
		},
		{
			name:      "editorial verdict is not a matchup",
			title:     "Brady vs. Belichick: The Verdict: The Case for Bill Belichick",
			wantMatch: false,
		},
		{
			name:      "competition and round suffix is not part of the home team",
			title:     "Bodo/Glimt (NOR) vs N.E.C. (NED) - UEFA Champions League 2026-2027 - Play-Off - 2nd leg",
			wantAway:  "Bodo/Glimt (NOR)",
			wantHome:  "N.E.C. (NED)",
			wantMatch: true,
		},
		{
			name:      "venue suffix is not part of the home country",
			title:     "(CA) (CBC 01) | 2026 Women's Volleyball Nations League: Canada vs Dominican Republic _ Hong Kong (2026-07-12 04:15:00)",
			wantAway:  "Canada",
			wantHome:  "Dominican Republic",
			wantMatch: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			away, home, matched := guideSportsMatchup(test.title)
			if matched != test.wantMatch || away != test.wantAway || home != test.wantHome {
				t.Fatalf("guideSportsMatchup(%q) = %q, %q, %v; want %q, %q, %v", test.title, away, home, matched, test.wantAway, test.wantHome, test.wantMatch)
			}
		})
	}
	if got := guideSportsVenue("(CA) (CBC 01) | 2026 Women's Volleyball Nations League: Canada vs Dominican Republic _ Hong Kong (2026-07-12 04:15:00)"); got != "Hong Kong" {
		t.Fatalf("expected volleyball location to be preserved as venue metadata, got %q", got)
	}
}

func TestSportsIdentityFallbacksUseReferencedSoccerMarks(t *testing.T) {
	t.Parallel()

	event := applySportsIdentityFallbacks(SportsEvent{
		LeagueID: "leagues-cup", LeagueName: "Leagues Cup", SportName: "Soccer", Name: "Toluca vs Austin FC",
		Away: SportsTeam{Name: "Austin FC"}, Home: SportsTeam{Name: "Toluca"},
	})

	if got := event.Away.LogoURL; got != sportsLogosRawBaseURL+"/MLS/ATX.png" {
		t.Fatalf("expected Austin FC to use the referenced crest, got %q", got)
	}
	if got := event.Home.LogoURL; got != sportsLogosRawBaseURL+"/MLS/TOL.png" {
		t.Fatalf("expected Toluca to use the referenced crest, got %q", got)
	}
}

func TestSportsIdentityFallbacksUseHonestBoxingMarkForUnknownFighters(t *testing.T) {
	t.Parallel()

	event := applySportsIdentityFallbacks(SportsEvent{
		LeagueID: "boxing", LeagueName: "Boxing", SportName: "Combat Sports", Name: "Diego Pacheco vs Steve Nelson",
		Away: SportsTeam{Name: "Diego Pacheco"}, Home: SportsTeam{Name: "Steve Nelson"},
	})
	want := "https://game-thumbs.swvn.io/boxing/logo.png"
	if event.Away.LogoFallbackURL != want || event.Home.LogoFallbackURL != want {
		t.Fatalf("expected unavailable fighter portraits to use the honest boxing mark, got away=%q home=%q", event.Away.LogoURL, event.Home.LogoURL)
	}
}

func TestGuideSportsLeagueRecognizesUEFAChampionsLeague(t *testing.T) {
	t.Parallel()

	id, name, sport, matched := guideSportsLeague("Bodo/Glimt vs N.E.C. - UEFA Champions League 2026-2027 - Play-Off - 2nd leg")
	if !matched || id != "uefa-champions-league" || name != "UEFA Champions League" || sport != "Soccer" {
		t.Fatalf("unexpected Champions League identity: %q, %q, %q, %v", id, name, sport, matched)
	}
}

func TestSportsEventsFromGuideCleansPromoMetadataAndRejectsNonSportsShows(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 25, 23, 0, 0, 0, time.UTC)
	programs := []model.Program{
		{ID: "program:hunt", ChannelID: "channel:sports", Title: "Bob Redfern's Outdoor Magazine : Pheasant Hunt at Heartland Lodge", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(30 * time.Minute).Unix()},
		{ID: "program:soccer", ChannelID: "channel:sports", Title: "Fútbol UEFA Champions League : Sabah FK vs. Hapoel Beer Sheva ᴸᶦᵛᵉ", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(30 * time.Minute).Unix()},
		{ID: "program:morning-show", ChannelID: "channel:sports", Title: "Good Morning Arizona at 9am ᴺᵉʷ", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(30 * time.Minute).Unix()},
		{ID: "program:next-game", ChannelID: "channel:sports", Title: "Next Game: Boston Red Sox @ Miami Marlins on 2026-08-25 at 06:40PM EDT", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(30 * time.Minute).Unix()},
		{ID: "program:dated-team", ChannelID: "channel:sports", Title: "(CA) (CBC 07) | CEBL: Calgary at Winnipeg (2026-07-12 15:30:00)", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(30 * time.Minute).Unix()},
		{ID: "program:cfp", ChannelID: "channel:sports", Title: "CFP Quarterfinal at the Rose Bowl : Alabama vs. Indiana", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(30 * time.Minute).Unix()},
		{ID: "program:cricket-match-number", ChannelID: "channel:sports", Title: "Cricket Highlights : The Hundred 2026: Leeds vs Super Giants - 10th Match", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(30 * time.Minute).Unix()},
		{ID: "program:champions-league-round", ChannelID: "channel:sports", Title: "Bodo/Glimt (NOR) vs N.E.C. (NED) - UEFA Champions League 2026-2027 - Play-Off - 2nd leg", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(30 * time.Minute).Unix()},
	}
	events := sportsEventsFromGuide(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "channel:sports", Name: "Sports Network", CategoryID: "sports", CategoryName: "Sports"}},
		Programs: programs,
		Content:  model.ContentState{LiveCategories: []model.Category{{ID: "sports", Name: "Sports", Kind: "live"}}},
	}}, now)
	byName := map[string]SportsEvent{}
	for _, event := range events {
		byName[event.Name] = event
	}

	if _, found := byName[programs[0].Title]; found {
		t.Fatalf("expected a hunt at a location to be excluded from sports matchups: %+v", byName[programs[0].Title])
	}
	if _, found := byName[programs[2].Title]; found {
		t.Fatalf("expected a morning news show to be excluded from sports matchups: %+v", byName[programs[2].Title])
	}

	soccer, found := byName["Fútbol UEFA Champions League : Sabah FK vs. Hapoel Beer Sheva"]
	if !found || soccer.Away.Name != "Sabah FK" || soccer.Home.Name != "Hapoel Beer Sheva" {
		t.Fatalf("expected live annotation removed from the event and team names, got %+v", soccer)
	}

	nextGame := byName[programs[3].Title]
	if nextGame.Away.Name != "Boston Red Sox" || nextGame.Home.Name != "Miami Marlins" || nextGame.Live || nextGame.Status != "scheduled" || nextGame.StatusText != "Upcoming" {
		t.Fatalf("expected Next Game promo to parse as an upcoming matchup, got %+v", nextGame)
	}

	dated := byName[programs[4].Title]
	if dated.Away.Name != "Calgary" || dated.Home.Name != "Winnipeg" {
		t.Fatalf("expected trailing date metadata removed from team names, got %+v", dated)
	}

	cfp := byName[programs[5].Title]
	if cfp.LeagueID != "college-football" || cfp.LeagueName != "College Football" || cfp.SportName != "Football" {
		t.Fatalf("expected CFP event classified as college football, got %+v", cfp)
	}

	cricket := byName[programs[6].Title]
	if cricket.Away.Name != "Leeds" || cricket.Home.Name != "Super Giants" {
		t.Fatalf("expected ordinal match metadata removed from cricket team names, got %+v", cricket)
	}

	championsLeague := byName[programs[7].Title]
	if championsLeague.Away.Name != "Bodo/Glimt (NOR)" || championsLeague.Home.Name != "N.E.C. (NED)" || championsLeague.LeagueID != "uefa-champions-league" {
		t.Fatalf("expected competition metadata removed and Champions League identity retained, got %+v", championsLeague)
	}
}

func TestSportsEventsFromGuidePrioritizesProgramCategories(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 25, 20, 30, 0, 0, time.UTC)
	events := sportsEventsFromGuide(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{
			{ID: "channel:general", Name: "Channel 7", CategoryID: "general", CategoryName: "General TV"},
			{ID: "channel:sports", Name: "Sports Network", CategoryID: "sports", CategoryName: "Sports"},
		},
		Programs: []model.Program{
			{ID: "program:metadata-sports", ChannelID: "channel:general", Title: "Olympic Gymnastics Qualifying", Categories: []string{"Sports event"}, StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix()},
			{ID: "program:sporting-event", ChannelID: "channel:general", Title: "Local Derby: Alpha at Beta", Categories: []string{"Sporting event"}, StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix()},
			{ID: "program:inconclusive-metadata", ChannelID: "channel:sports", Title: "College Football: Alabama at Indiana", Categories: []string{"HD"}, StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix()},
			{ID: "program:sports-talk", ChannelID: "channel:sports", Title: "College Football: Georgia at Ole Miss", Categories: []string{"Sports talk", "Football"}, StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix()},
			{ID: "program:fallback", ChannelID: "channel:sports", Title: "NFL Football: Jets at Giants", StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix()},
		},
		Content: model.ContentState{LiveCategories: []model.Category{
			{ID: "general", Name: "General TV", Kind: "live"},
			{ID: "sports", Name: "Sports", Kind: "live"},
		}},
	}}, now)
	byName := map[string]SportsEvent{}
	for _, event := range events {
		byName[event.Name] = event
	}
	if event, ok := byName["Olympic Gymnastics Qualifying"]; !ok || event.EventType != "event" {
		t.Fatalf("expected Sports event metadata to include non-matchup coverage outside a sports channel, got %+v", events)
	}
	if _, ok := byName["Local Derby: Alpha at Beta"]; !ok {
		t.Fatalf("expected Sporting event metadata to be authoritative, got %+v", events)
	}
	if _, ok := byName["College Football: Alabama at Indiana"]; !ok {
		t.Fatalf("expected inconclusive metadata to continue through title fallback, got %+v", events)
	}
	if _, ok := byName["College Football: Georgia at Ole Miss"]; ok {
		t.Fatalf("expected Sports talk metadata to exclude the program even with a sport category, got %+v", events)
	}
	if _, ok := byName["NFL Football: Jets at Giants"]; !ok {
		t.Fatalf("expected missing metadata to preserve the conservative title/channel fallback, got %+v", events)
	}
}

func TestSportsPayloadRefreshesScoresForAnnotatedGuideEvents(t *testing.T) {
	for _, annotation := range []string{"ᴸᶦᵛᵉ", "ᴺᵉʷ"} {
		t.Run(annotation, func(t *testing.T) {
			now := time.Now()
			title := "Fútbol UEFA Champions League : Sabah FK vs. Hapoel Beer Sheva " + annotation
			store := cache.NewStore()
			store.Replace(cache.Snapshot{Catalog: model.CatalogState{
				Channels: []model.Channel{{ID: "channel:sports", Name: "Sports Network", CategoryID: "sports", CategoryName: "Sports"}},
				Programs: []model.Program{{ID: "program:annotated", ChannelID: "channel:sports", Title: title, StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(30 * time.Minute).Unix()}},
				Content:  model.ContentState{LiveCategories: []model.Category{{ID: "sports", Name: "Sports", Kind: "live"}}},
			}, Health: model.SyncHealth{EPGStatus: "ok", EPGLastSuccessUnix: now.Unix()}})
			fresh := SportsEvent{
				ID: "provider:event", LeagueID: "sports", LeagueName: "Sports", Name: "Sabah FK vs Hapoel Beer Sheva",
				StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(30 * time.Minute).Unix(),
				Away: SportsTeam{Name: "Sabah FK"}, Home: SportsTeam{Name: "Hapoel Beer Sheva"},
				AwayScore: "1", HomeScore: "2", Live: true, Status: "live", StatusText: "Live",
			}
			provider := &countingSportsProvider{events: []SportsEvent{fresh}}
			server := newEnabledSportsTestServer(store)
			server.sportsProvider = provider
			stale := fresh
			stale.AwayScore, stale.HomeScore = "0", "0"
			server.sportsCache = sportsEventCache{Events: []SportsEvent{stale}, UpdatedUnix: now.Add(-time.Minute).Unix(), Source: "counting-test", ExpiresAfter: now.Add(5 * time.Minute)}
			server.sportsPrepared = sportsPreparedCache{
				Payload:      SportsPayload{Events: []SportsEvent{stale}, Source: "counting-test", UpdatedAtUnix: now.Add(-time.Minute).Unix()},
				ExpiresAfter: now.Add(5 * time.Minute), GuideUpdatedUnix: now.Add(-time.Minute).Unix(), Ready: true,
			}

			_ = server.preparedSportsPayload(false)
			deadline := time.Now().Add(time.Second)
			for provider.calls.Load() == 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if provider.calls.Load() != 1 {
				t.Fatalf("expected %s annotation to force a score lookup, got %d provider calls", annotation, provider.calls.Load())
			}
			var payload SportsPayload
			for time.Now().Before(deadline) {
				payload = server.preparedSportsPayload(false)
				if !payload.Refreshing {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if len(payload.Events) != 1 || payload.Events[0].AwayScore != "1" || payload.Events[0].HomeScore != "2" {
				t.Fatalf("expected refreshed scores for %s annotation, got %+v", annotation, payload.Events)
			}
			if strings.Contains(payload.Events[0].Name, annotation) || strings.Contains(payload.Events[0].Home.Name, annotation) {
				t.Fatalf("expected %s annotation hidden after score lookup, got %+v", annotation, payload.Events[0])
			}
		})
	}
}

func TestSportsGuideScoreLookupHintsIgnoreUnqualifiedPrograms(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name       string
		title      string
		start, end time.Time
	}{
		{name: "non sports", title: "Good Morning Arizona at 9am ᴺᵉʷ", start: now.Add(-time.Hour), end: now.Add(time.Hour)},
		{name: "unannotated", title: "WNBA: Indiana Fever vs Chicago Sky", start: now.Add(-time.Hour), end: now.Add(time.Hour)},
		{name: "upcoming", title: "WNBA: Indiana Fever vs Chicago Sky ᴺᵉʷ", start: now.Add(time.Hour), end: now.Add(3 * time.Hour)},
		{name: "ended", title: "WNBA: Indiana Fever vs Chicago Sky ᴸᶦᵛᵉ", start: now.Add(-3 * time.Hour), end: now.Add(-time.Hour)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, refreshScores := sportsEventsFromGuideWithScoreHints(cache.Snapshot{Catalog: model.CatalogState{
				Channels: []model.Channel{{ID: "channel:sports", Name: "Sports Network", CategoryID: "sports", CategoryName: "Sports"}},
				Programs: []model.Program{{ID: "program:test", ChannelID: "channel:sports", Title: test.title, StartUnix: test.start.Unix(), EndUnix: test.end.Unix()}},
				Content:  model.ContentState{LiveCategories: []model.Category{{ID: "sports", Name: "Sports", Kind: "live"}}},
			}}, now)
			if refreshScores {
				t.Fatalf("expected %s program not to force a score lookup", test.name)
			}
		})
	}
}

func TestPreparedSportsPayloadRebuildsAgainWhenGuideChangesDuringBuild(t *testing.T) {
	now := time.Now()
	store := cache.NewStore()
	store.ReplaceExact(cache.Snapshot{Health: model.SyncHealth{EPGStatus: "ok", EPGLastSuccessUnix: now.Add(-time.Minute).Unix()}})
	provider := &blockingSportsProvider{
		events: []SportsEvent{{
			ID: "provider:event", LeagueID: "wnba", LeagueName: "WNBA", Name: "Indiana Fever vs Chicago Sky",
			Away: SportsTeam{Name: "Indiana Fever"}, Home: SportsTeam{Name: "Chicago Sky"}, Live: true,
		}},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	server := newEnabledSportsTestServer(store)
	server.sportsProvider = provider
	server.sportsPrepared = sportsPreparedCache{
		Payload: SportsPayload{Source: "blocking-test"}, ExpiresAfter: now.Add(5 * time.Minute),
		GuideUpdatedUnix: now.Add(-2 * time.Minute).Unix(), Ready: true,
	}

	_ = server.preparedSportsPayload(false)
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("initial sports rebuild did not start")
	}
	store.ReplaceExact(cache.Snapshot{
		Catalog: model.CatalogState{
			Channels: []model.Channel{{ID: "channel:wnba", Name: "WNBA League Pass", CategoryID: "wnba", CategoryName: "WNBA"}},
			Programs: []model.Program{{ID: "program:annotated", ChannelID: "channel:wnba", Title: "WNBA: Indiana Fever vs Chicago Sky ᴸᶦᵛᵉ", StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix()}},
			Content:  model.ContentState{LiveCategories: []model.Category{{ID: "wnba", Name: "WNBA", Kind: "live"}}},
		},
		Health: model.SyncHealth{EPGStatus: "ok", EPGLastSuccessUnix: now.Unix()},
	})
	close(provider.release)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		server.sportsPreparedMu.Lock()
		refreshing := server.sportsPrepared.Refreshing
		server.sportsPreparedMu.Unlock()
		if !refreshing {
			break
		}
		time.Sleep(time.Millisecond)
	}
	_ = server.preparedSportsPayload(false)
	for provider.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if provider.calls.Load() != 2 {
		t.Fatalf("expected changed guide revision to schedule a second score lookup, got %d provider calls", provider.calls.Load())
	}
}

func TestSportsEventsFromGuideClassifiesRacesAndBroadcastState(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	programs := []model.Program{
		{
			ID: "program:formula-e", ChannelID: "channel:sports",
			Title: "Formule : Formule E v Miami", StartUnix: now.Add(-30 * time.Minute).Unix(), EndUnix: now.Add(90 * time.Minute).Unix(),
		},
		{
			ID: "program:nfl-replay", ChannelID: "channel:sports",
			Title: "NFL Football : San Francisco 49ers at Los Angeles Chargers", Summary: "A rebroadcast of the complete game.",
			StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix(),
		},
		{
			ID: "program:cricket-highlights", ChannelID: "channel:sports",
			Title: "Cricket Highlights : Bangladesh vs Australia: 2nd ODI", StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix(),
		},
	}
	events := sportsEventsFromGuide(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "channel:sports", Name: "Sports Network", CategoryID: "sports", CategoryName: "Sports"}},
		Programs: programs,
		Content:  model.ContentState{LiveCategories: []model.Category{{ID: "sports", Name: "Sports", Kind: "live"}}},
	}}, now)
	byName := map[string]SportsEvent{}
	for _, event := range events {
		byName[event.Name] = event
	}

	race := byName[programs[0].Title]
	if race.EventType != "race" || race.LeagueID != "formula-e" || race.Away.Name != "Formula E" || race.Home.Name != "Miami" {
		t.Fatalf("expected Formula E race identity without a fake matchup, got %+v", race)
	}
	if !race.Live || race.Status != "airing" || race.StatusText != "On now" {
		t.Fatalf("expected unverified EPG race to be airing rather than live, got %+v", race)
	}

	replay := byName[programs[1].Title]
	if !replay.Live || replay.Status != "replay" || replay.StatusText != "Replay" {
		t.Fatalf("expected explicit EPG rebroadcast classification, got %+v", replay)
	}

	highlights := byName[programs[2].Title]
	if !highlights.Live || highlights.Status != "highlights" || highlights.StatusText != "Highlights" {
		t.Fatalf("expected highlights classification, got %+v", highlights)
	}
}

func TestSportsEventsFromGuideClassifiesNASCARCupHighlightsAsRaceAtLocation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	title := "NASCAR Cup Series Highlights (Erstausstrahlung) : NCS Race at New Hampshire (New Hampshire)"
	events := sportsEventsFromGuide(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "channel:sports", Name: "Motorvision TV", CategoryID: "sports", CategoryName: "Sports"}},
		Programs: []model.Program{{ID: "program:nascar", ChannelID: "channel:sports", Title: title, StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix()}},
		Content:  model.ContentState{LiveCategories: []model.Category{{ID: "sports", Name: "Sports", Kind: "live"}}},
	}}, now)
	if len(events) != 1 {
		t.Fatalf("expected one NASCAR Cup highlights event, got %+v", events)
	}
	event := events[0]
	if event.EventType != "race" || event.LeagueID != "nascar-cup-series" || event.LeagueName != "NASCAR Cup Series" || event.SportName != "Motorsport" {
		t.Fatalf("expected NASCAR Cup motorsport identity, got %+v", event)
	}
	if event.Away.Name != "NCS Race" || event.Home.Name != "New Hampshire" {
		t.Fatalf("expected NCS race and New Hampshire location without a versus matchup, got %+v", event)
	}
	if event.Status != "highlights" || event.StatusText != "Highlights" {
		t.Fatalf("expected NASCAR highlights status, got %+v", event)
	}
}

func TestMergeSportsGuideEventsMarksLaterCompletedMatchupAiringAsReplay(t *testing.T) {
	t.Parallel()

	providerStart := time.Date(2026, time.August, 23, 20, 0, 0, 0, time.UTC).Unix()
	guideStart := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC).Unix()
	provider := SportsEvent{
		ID: "sportarr:nfl-game", LeagueID: "nfl", Name: "San Francisco 49ers at Los Angeles Chargers",
		Away: SportsTeam{Name: "San Francisco 49ers"}, Home: SportsTeam{Name: "Los Angeles Chargers"},
		StartUnix: providerStart, Completed: true, Status: "completed", StatusText: "Final",
	}
	guide := SportsEvent{
		ID: "epg:nfl-rebroadcast", LeagueID: "nfl", Name: "NFL Football : San Francisco 49ers at Los Angeles Chargers",
		Away: SportsTeam{Name: "San Francisco 49ers"}, Home: SportsTeam{Name: "Los Angeles Chargers"},
		StartUnix: guideStart, Live: true, Status: "airing", StatusText: "On now",
	}

	merged := mergeSportsGuideEvents([]SportsEvent{provider}, []SportsEvent{guide})
	if len(merged) != 2 {
		t.Fatalf("expected completed event and later guide airing to remain separate, got %+v", merged)
	}
	if merged[1].Status != "replay" || merged[1].StatusText != "Replay" || !merged[1].Live {
		t.Fatalf("expected later same-matchup airing to be identified as replay, got %+v", merged[1])
	}
}

func TestSportsEventsFromGuideCapsFallbackSlate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	programs := make([]model.Program, 0, 300)
	for index := 0; index < 300; index++ {
		programs = append(programs, model.Program{
			ID:        fmt.Sprintf("p:%d", index),
			ChannelID: "ch:sports",
			Title:     fmt.Sprintf("WNBA: Team %03d vs Club %03d", index, index),
			StartUnix: now.Add(time.Duration(index%48) * time.Hour).Unix(),
			EndUnix:   now.Add(time.Duration(index%48+2) * time.Hour).Unix(),
		})
	}
	events := sportsEventsFromGuide(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "ch:sports", Name: "WNBA League Pass", CategoryID: "sports"}},
		Programs: programs,
		Content:  model.ContentState{LiveCategories: []model.Category{{ID: "sports", Name: "Sports", Kind: "live"}}},
	}}, now)
	if len(events) != 250 {
		t.Fatalf("expected bounded EPG fallback slate, got %d events", len(events))
	}
}

func TestSportsPayloadBoundsProviderWorkBelowPluginRouteDeadline(t *testing.T) {
	t.Parallel()

	provider := &deadlineSportsProvider{}
	server := newEnabledSportsTestServer(cache.NewStore())
	server.sportsProvider = provider
	payload := server.sportsPayload(context.Background(), false)
	if provider.maxRemaining <= 0 || provider.maxRemaining > sportsProviderFetchTimeout {
		t.Fatalf("expected bounded provider deadline, got %s", provider.maxRemaining)
	}
	if payload.Error == "" || payload.Source != "deadline-test" {
		t.Fatalf("expected provider error payload, got %+v", payload)
	}
}

func TestSportsRouteReturnsWhilePreparedPayloadBuildRuns(t *testing.T) {
	t.Parallel()

	provider := &blockingSportsProvider{
		events:  []SportsEvent{{ID: "event:ready", LeagueID: "nfl", LeagueName: "NFL", Name: "Jets at Giants", StartUnix: 1700000000, Away: SportsTeam{Name: "New York Jets", Abbreviation: "NYJ"}, Home: SportsTeam{Name: "New York Giants", Abbreviation: "NYG"}}},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "exact", Name: "NYJ vs NYG", CategoryID: "nfl", CategoryName: "NFL"}},
		Content:  model.ContentState{LiveCategories: []model.Category{{ID: "nfl", Name: "NFL", Kind: "live"}}},
	}})
	server := newEnabledSportsTestServer(store)
	server.sportsProvider = provider

	startedAt := time.Now()
	payload := fetchSportsPayloadOnce(t, server, false)
	if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
		t.Fatalf("sports route waited for background build: %s", elapsed)
	}
	if !payload.Refreshing || payload.Source != "blocking-test" {
		t.Fatalf("expected refreshing placeholder, got %+v", payload)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("sports payload build did not start")
	}
	close(provider.release)

	ready := fetchSportsPayload(t, server)
	if ready.Refreshing || len(ready.Events) != 1 || ready.Events[0].ID != "event:ready" {
		t.Fatalf("expected prepared sports payload, got %+v", ready)
	}
	if calls := provider.calls.Load(); calls != 1 {
		t.Fatalf("expected one background provider call, got %d", calls)
	}
}

func TestSportsRouteReturnsGuideFallbackWhileProviderBuildRuns(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Channels: []model.Channel{{ID: "channel:wnba", Name: "WNBA League Pass", CategoryID: "sports", CategoryName: "US Sports | WNBA"}},
			Programs: []model.Program{{ID: "program:wnba", ChannelID: "channel:wnba", Title: "Indiana Fever vs Las Vegas Aces", StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix()}},
		},
		Health: model.SyncHealth{EPGLastSuccessUnix: now.Add(-time.Minute).Unix()},
	})
	provider := &blockingSportsProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	server := newEnabledSportsTestServer(store)
	server.sportsProvider = provider

	payload := fetchSportsPayloadOnce(t, server, false)
	if !payload.Refreshing || payload.Source != "EPG fallback" || len(payload.Events) != 1 {
		t.Fatalf("expected immediate EPG fallback while provider builds, got %+v", payload)
	}
	if len(payload.Events[0].Channels) != 1 || payload.Events[0].Channels[0].ID != "channel:wnba" {
		t.Fatalf("expected guide channel match in fallback, got %+v", payload.Events[0].Channels)
	}
	close(provider.release)
}

func TestSportsRouteDoesNotWaitForCatalogHydration(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	store := cache.NewStore()
	syncer := &stubCatalogSyncer{store: store, block: block}
	settings := config.Settings{
		SourceMode:        config.SourceModeAPIKey,
		DispatcharrURL:    "https://dispatcharr.example.com",
		DispatcharrAPIKey: "secret",
	}
	server := NewHTTPRoutesServerWithSyncer(store, func() config.Settings { return settings }, syncer)

	startedAt := time.Now()
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/dispatcharr/api/sports"})
	if err != nil {
		close(block)
		t.Fatalf("sports route: %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
		close(block)
		t.Fatalf("sports route waited for catalog hydration: %s", elapsed)
	}
	if response.GetStatusCode() != http.StatusOK {
		close(block)
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	close(block)
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

func TestMatchSportsChannelsRejectsPositiveButLowConfidenceMatch(t *testing.T) {
	t.Parallel()

	snapshot := cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{
			ID: "ch:event-title", Name: "World Cup Quarterfinal", CategoryID: "general", CategoryName: "General TV",
		}},
		Content: model.ContentState{LiveCategories: []model.Category{{ID: "general", Name: "General TV", Kind: "live"}}},
	}}
	event := SportsEvent{
		ID:         "event:world-cup",
		LeagueID:   "world-cup",
		LeagueName: "World Cup",
		Name:       "World Cup Quarterfinal",
		Home:       SportsTeam{Name: "Mexico"},
		Away:       SportsTeam{Name: "Ecuador"},
	}

	assertNoSportsMatch(t, matchSportsChannels(event, snapshot), "ch:event-title")
}

func TestMatchSportsChannelsRejectsPartialSingleWordTeamNames(t *testing.T) {
	t.Parallel()

	snapshot := cache.Snapshot{
		Catalog: model.CatalogState{
			Channels: []model.Channel{
				{ID: "ch:new-england", Name: "New England Revolution", CategoryID: "mls", CategoryName: "US Sports | MLS Teams"},
				{ID: "ch:england", Name: "England Sports", CategoryID: "world", CategoryName: "International Sports | England"},
			},
			Content: model.ContentState{
				LiveCategories: []model.Category{
					{ID: "mls", Name: "US Sports | MLS Teams", Kind: "live"},
					{ID: "world", Name: "International Sports | England", Kind: "live"},
				},
			},
		},
	}
	event := SportsEvent{
		ID:         "event:world-cup",
		LeagueID:   "world-cup",
		LeagueName: "World Cup",
		Name:       "England vs Norway",
		ShortName:  "ENG vs NOR",
		StartUnix:  1700000000,
		Home:       SportsTeam{ID: "team:eng", Name: "England", Abbreviation: "ENG"},
		Away:       SportsTeam{ID: "team:nor", Name: "Norway", Abbreviation: "NOR"},
	}

	matches := matchSportsChannels(event, snapshot)
	assertSportsMatch(t, matches, "ch:england")
	assertNoSportsMatch(t, matches, "ch:new-england")
}

func TestMatchSportsChannelsRejectsAbbreviationOutsideSportsContext(t *testing.T) {
	t.Parallel()

	snapshot := cache.Snapshot{
		Catalog: model.CatalogState{
			Channels: []model.Channel{
				{ID: "ch:music", Name: "MC Dance EDM", CategoryID: "music", CategoryName: "International TV | Latino | Music"},
				{ID: "ch:edm", Name: "EDM Team Feed", CategoryID: "nhl", CategoryName: "US Sports | NHL Teams"},
			},
			Content: model.ContentState{
				LiveCategories: []model.Category{
					{ID: "music", Name: "International TV | Latino | Music", Kind: "live"},
					{ID: "nhl", Name: "US Sports | NHL Teams", Kind: "live"},
				},
			},
		},
	}
	event := SportsEvent{
		ID:         "event:nhl",
		LeagueID:   "nhl",
		LeagueName: "NHL",
		Name:       "Winnipeg Jets at Edmonton Oilers",
		ShortName:  "WPG @ EDM",
		StartUnix:  1700000000,
		Home:       SportsTeam{ID: "team:edm", Name: "Edmonton Oilers", Abbreviation: "EDM"},
		Away:       SportsTeam{ID: "team:wpg", Name: "Winnipeg Jets", Abbreviation: "WPG"},
	}

	matches := matchSportsChannels(event, snapshot)
	assertSportsMatch(t, matches, "ch:edm")
	assertNoSportsMatch(t, matches, "ch:music")
}

func TestMatchSportsChannelsRejectsAbbreviationOutsideEventLeague(t *testing.T) {
	t.Parallel()

	snapshot := cache.Snapshot{
		Catalog: model.CatalogState{
			Channels: []model.Channel{
				{ID: "ch:big-ten", Name: "Big Ten Network", CategoryID: "sports", CategoryName: "US TV | Sports"},
				{ID: "ch:ten", Name: "TEN Team Feed", CategoryID: "nfl", CategoryName: "US Sports | NFL Teams"},
			},
			Content: model.ContentState{
				LiveCategories: []model.Category{
					{ID: "sports", Name: "US TV | Sports", Kind: "live"},
					{ID: "nfl", Name: "US Sports | NFL Teams", Kind: "live"},
				},
			},
		},
	}
	event := SportsEvent{
		ID:         "event:nfl",
		LeagueID:   "nfl",
		LeagueName: "NFL",
		Name:       "Tennessee Titans at New York Jets",
		ShortName:  "TEN @ NYJ",
		StartUnix:  1700000000,
		Home:       SportsTeam{ID: "team:nyj", Name: "New York Jets", Abbreviation: "NYJ"},
		Away:       SportsTeam{ID: "team:ten", Name: "Tennessee Titans", Abbreviation: "TEN"},
	}

	matches := matchSportsChannels(event, snapshot)
	assertSportsMatch(t, matches, "ch:ten")
	assertNoSportsMatch(t, matches, "ch:big-ten")
}

func TestHTTPRoutesServerSportsUsesStaleCacheOnProviderError(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "exact", Name: "NYJ vs NYG", CategoryID: "nfl", CategoryName: "NFL"}},
		Content:  model.ContentState{LiveCategories: []model.Category{{ID: "nfl", Name: "NFL", Kind: "live"}}},
	}})
	server := newEnabledSportsTestServer(store)
	server.sportsProvider = staticSportsProvider{events: []SportsEvent{{ID: "event:cached", LeagueID: "nfl", LeagueName: "NFL", Name: "Jets at Giants", StartUnix: 1700000000, Away: SportsTeam{Name: "New York Jets", Abbreviation: "NYJ"}, Home: SportsTeam{Name: "New York Giants", Abbreviation: "NYG"}}}}
	first := fetchSportsPayload(t, server)
	if len(first.Events) != 1 {
		t.Fatalf("expected cached event seed, got %+v", first)
	}
	server.sportsProvider = staticSportsProvider{err: errors.New("provider down")}
	server.sportsCache.ExpiresAfter = time.Now().Add(-time.Second)
	server.sportsPrepared.ExpiresAfter = time.Now().Add(-time.Second)
	payload := fetchSportsPayload(t, server)
	if payload.Error == "" || len(payload.Events) != 1 || payload.Events[0].ID != "event:cached" {
		t.Fatalf("expected stale cached event with error, got %+v", payload)
	}
}

func TestSportarrSportsEventMapsCanonicalFields(t *testing.T) {
	t.Parallel()

	event := sportarrEvent{
		ID:                "event-uuid",
		ShortID:           "ev-401",
		Name:              "Panama vs Croatia",
		EventType:         "group_stage",
		LeagueID:          "league-uuid",
		LeagueName:        "World Cup",
		SeasonName:        "2026",
		Round:             "Group A",
		VenueName:         "MetLife Stadium",
		ScheduledStart:    "2026-06-26T22:35:00Z",
		ScheduledEnd:      "2026-06-27T00:35:00Z",
		BroadcastTimezone: "America/New_York",
		Status:            "in_progress",
		StatusText:        "Second half",
		Period:            sportarrString("2"),
		Clock:             sportarrString("67:14"),
		HomeTeamID:        "panama-id",
		HomeTeamName:      "Panama",
		AwayTeamID:        "croatia-id",
		AwayTeamName:      "Croatia",
		HomeScore:         sportarrString("1"),
		AwayScore:         sportarrString("2"),
	}
	converted := event.sportsEvent()
	expected := time.Date(2026, 6, 26, 22, 35, 0, 0, time.UTC).Unix()
	if converted.StartUnix != expected {
		t.Fatalf("expected parsed start %d, got %d", expected, converted.StartUnix)
	}
	if converted.ID != "sportarr:ev-401" || converted.ProviderID != "event-uuid" || converted.ProviderShortID != "ev-401" || converted.LeagueName != "World Cup" || converted.Home.Name != "Panama" || converted.Away.Name != "Croatia" {
		t.Fatalf("unexpected Sportarr mapping: %+v", converted)
	}
	if !converted.Live || converted.Completed || converted.StatusText != "Second half" || converted.Period != "2" || converted.Clock != "67:14" || converted.HomeScore != "1" || converted.AwayScore != "2" {
		t.Fatalf("unexpected Sportarr live state: %+v", converted)
	}
	if converted.EventType != "group_stage" || converted.Round != "Group A" || converted.Venue != "MetLife Stadium" || converted.BroadcastTimezone != "America/New_York" {
		t.Fatalf("expected canonical Sportarr metadata, got %+v", converted)
	}
}

func TestSportarrEventBadgesSurvivePreparedPayload(t *testing.T) {
	var source sportarrEvent
	if err := json.Unmarshal([]byte(`{"id":"atlas-atlante","leagueBadgeUrl":"https://sportarr.net/static/images/league/mx/badge.png","homeTeamBadgeUrl":"https://sportarr.net/static/images/team/atlas/badge.png","awayTeamBadgeUrl":"https://sportarr.net/static/images/team/atlante/badge.png"}`), &source); err != nil {
		t.Fatal(err)
	}
	server := &HTTPRoutesServer{sportsImages: newSportsImageCache(t.TempDir(), nil)}
	event := server.proxySportsEventImages([]SportsEvent{source.sportsEvent()})[0]
	if event.LeagueLogoURL != source.LeagueBadgeURL || event.Home.LogoURL != source.HomeTeamBadgeURL || event.Away.LogoURL != source.AwayTeamBadgeURL {
		t.Fatalf("event badges were lost or rewritten: %+v", event)
	}
	event.Home = applySportarrTeam(event.Home, sportarrTeam{LogoURL: "https://r2.thesportsdb.com/images/media/team/badge/atlas.png"})
	event = server.proxySportsEventImages([]SportsEvent{event})[0]
	if event.Home.LogoURL != "https://r2.thesportsdb.com/images/media/team/badge/atlas.png" {
		t.Fatalf("enriched team badge was rewritten: %s", event.Home.LogoURL)
	}
}

func TestSportarrSportsProviderLoadsPaginatedPublicEvents(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.URL.Path != "/events" {
			http.NotFound(response, request)
			return
		}
		if request.URL.Query().Get("page_size") != "100" || request.URL.Query().Get("from") == "" || request.URL.Query().Get("to") == "" {
			t.Errorf("unexpected Sportarr query: %s", request.URL.RawQuery)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.URL.Query().Get("page") == "2" {
			_, _ = response.Write([]byte(`{"items":[{"id":"uuid-2","shortId":"ev-2","name":"Team C vs Team D","leagueId":"league-2","leagueName":"League Two","scheduledStart":null,"status":"scheduled","homeTeamId":"team-c","homeTeamName":"Team C","awayTeamId":"team-d","awayTeamName":"Team D"}],"total":2,"page":2,"pageSize":100,"totalPages":2}`))
			return
		}
		_, _ = response.Write([]byte(`{"items":[{"id":"uuid-1","shortId":"ev-1","name":"Team A vs Team B","leagueId":"league-1","leagueName":"League One","scheduledStart":"2026-07-13T20:00:00Z","status":"completed","homeTeamId":"team-a","homeTeamName":"Team A","awayTeamId":"team-b","awayTeamName":"Team B","homeScore":3,"awayScore":"2"}],"total":-2,"page":1,"pageSize":100,"totalPages":2}`))
	})
	testServer := httptest.NewServer(handler)
	defer testServer.Close()

	provider := newSportarrSportsProvider(testServer.Client())
	provider.baseURL = testServer.URL
	events, err := provider.Events(context.Background(), time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Sportarr events: %v", err)
	}
	if provider.Source() != "sportarr" || len(events) != 2 {
		t.Fatalf("expected two Sportarr events, got %+v", events)
	}
	if !events[0].Completed || events[0].HomeScore != "3" || events[0].AwayScore != "2" {
		t.Fatalf("expected numeric and string scores to normalize, got %+v", events[0])
	}
	if events[1].StartUnix != 0 {
		t.Fatalf("unknown Sportarr start should stay 0, got %d", events[1].StartUnix)
	}
}

func TestSportarrSportsProviderLoadsCompleteLeagueRoster(t *testing.T) {
	t.Parallel()

	testServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.URL.Path != "/api/v2/json/list/teams/lg-wnba" {
			http.NotFound(response, request)
			return
		}
		_, _ = response.Write([]byte(`{"list":[{"idTeam":"tm-dream","strTeam":"Atlanta Dream","strTeamShort":"ATL","strTeamBadge":"https://images.example/dream.png","strColour1":"#aa1122","strColour2":"#223344"},{"idTeam":"tm-storm","strTeam":"Seattle Storm","strTeamShort":"SEA","strBadge":"https://images.example/storm.png"}],"_meta":{}}`))
	}))
	defer testServer.Close()

	provider := newSportarrSportsProvider(testServer.Client())
	provider.baseURL = testServer.URL + "/api/public/v1"
	teams, err := provider.LeagueTeams(context.Background(), "lg-wnba")
	if err != nil {
		t.Fatalf("Sportarr league teams: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected complete two-team roster, got %+v", teams)
	}
	if teams[0].ID != "tm-dream" || teams[0].Name != "Atlanta Dream" || teams[0].Abbreviation != "ATL" || teams[0].LogoURL != "https://images.example/dream.png" || teams[0].PrimaryColor != "#aa1122" || teams[0].SecondaryColor != "#223344" {
		t.Fatalf("unexpected primary roster team: %+v", teams[0])
	}
	if teams[1].LogoURL != "https://images.example/storm.png" {
		t.Fatalf("expected current strBadge alias to populate the logo, got %+v", teams[1])
	}
}

func TestSportsLeaguesKeepProviderLeagueIdentityForRosterLookup(t *testing.T) {
	t.Parallel()

	leagues := sportsLeagues([]SportsEvent{{
		LeagueID:         "wnba",
		ProviderLeagueID: "lg-wnba",
		LeagueName:       "WNBA",
		SportName:        "Basketball",
		Home:             SportsTeam{ID: "home", Name: "Atlanta Dream"},
		Away:             SportsTeam{ID: "away", Name: "Seattle Storm"},
	}})
	if len(leagues) != 1 || leagues[0].ProviderID != "lg-wnba" {
		t.Fatalf("expected canonical league to retain provider roster identity, got %+v", leagues)
	}
}

func TestSportsLeagueTeamsRouteMergesFullRosterWithAiringTeams(t *testing.T) {
	t.Parallel()

	server := newEnabledSportsTestServer(cache.NewStore())
	server.sportsProvider = rosterSportsProvider{
		staticSportsProvider: staticSportsProvider{},
		teams: []SportsTeam{
			{ID: "tm-dream", Name: "Atlanta Dream", Abbreviation: "ATL"},
			{ID: "tm-storm", Name: "Seattle Storm", Abbreviation: "SEA"},
		},
	}
	server.sportsPrepared = sportsPreparedCache{
		Ready:        true,
		ExpiresAfter: time.Now().Add(time.Hour),
		Payload: SportsPayload{
			Leagues: []SportsLeague{{ID: "wnba", ProviderID: "lg-wnba", Name: "WNBA", SportName: "Basketball"}},
			Events: []SportsEvent{{
				LeagueID: "wnba",
				Away:     SportsTeam{ID: "event-dream", Name: "Atlanta Dream", LogoURL: "https://images.example/live-dream.png"},
				Home:     SportsTeam{ID: "event-sparks", Name: "Los Angeles Sparks"},
			}},
		},
	}
	query, err := structpb.NewStruct(map[string]any{"league_id": "wnba", "provider_id": "lg-wnba"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: http.MethodGet,
		Path:   "/dispatcharr/api/sports/league-teams",
		Query:  query,
	})
	if err != nil {
		t.Fatalf("league teams route: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.GetStatusCode(), response.GetBody())
	}
	var payload SportsLeagueTeamsPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("decode league teams: %v", err)
	}
	if len(payload.Teams) != 3 {
		t.Fatalf("expected two roster teams plus the currently airing Sparks, got %+v", payload.Teams)
	}
	if payload.Teams[0].Name != "Atlanta Dream" || payload.Teams[0].ID != "event-dream" || payload.Teams[0].LogoFallbackURL != "https://images.example/live-dream.png" {
		t.Fatalf("expected airing identity to win while roster metadata fills gaps, got %+v", payload.Teams[0])
	}
}

func TestHTTPRoutesServerSportsDisabledByAdminFlag(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.SetAdminSettings([]byte(`{"sportsEnabled":false}`))
	server := newEnabledSportsTestServer(store)
	server.sportsProvider = staticSportsProvider{events: []SportsEvent{{ID: "game-1", Name: "Hidden"}}}
	server.sportsPrepared = sportsPreparedCache{
		Ready:        true,
		ExpiresAfter: time.Now().Add(time.Hour),
		Payload:      SportsPayload{Events: []SportsEvent{{ID: "game-1", Name: "Hidden"}}},
	}

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/dispatcharr/api/sports"})
	if err != nil {
		t.Fatalf("sports route: %v", err)
	}
	var payload SportsPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("decode sports: %v", err)
	}
	if len(payload.Events) != 0 {
		t.Fatalf("expected no sports events when admin disables sports, got %+v", payload.Events)
	}

	query, err := structpb.NewStruct(map[string]any{"league_id": "nfl"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	teams, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: http.MethodGet,
		Path:   "/dispatcharr/api/sports/league-teams",
		Query:  query,
	})
	if err != nil {
		t.Fatalf("league teams route: %v", err)
	}
	var teamPayload SportsLeagueTeamsPayload
	if err := json.Unmarshal(teams.GetBody(), &teamPayload); err != nil {
		t.Fatalf("decode league teams: %v", err)
	}
	if len(teamPayload.Teams) != 0 {
		t.Fatalf("expected no league teams when sports are disabled, got %+v", teamPayload.Teams)
	}
}

func TestMergeSportsLeagueRosterTeamsDeduplicatesUniqueTeamNicknameAliases(t *testing.T) {
	t.Parallel()

	teams := mergeSportsLeagueRosterTeams("nfl", "NFL", "Football",
		[]SportsTeam{
			{ID: "event-rams-full", Name: "Los Angeles Rams"},
			{ID: "event-rams-short", Name: "Rams"},
			{ID: "event-giants", Name: "New York Giants"},
		},
		[]SportsTeam{
			{ID: "roster-rams", Name: "Los Angeles Rams", Abbreviation: "LAR", LogoURL: "https://images.example/rams.png"},
			{ID: "roster-giants", Name: "New York Giants", Abbreviation: "NYG"},
		},
	)

	if len(teams) != 2 {
		t.Fatalf("expected one card per canonical team, got %+v", teams)
	}
	if teams[0].Name != "Los Angeles Rams" || teams[0].Abbreviation != "LAR" || teams[0].LogoFallbackURL != "https://images.example/rams.png" {
		t.Fatalf("expected the Rams alias to merge into the canonical roster identity, got %+v", teams[0])
	}
}

func TestSportarrSportsProviderCoalescesConcurrentTeamRequests(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	testServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"id":"team-id","name":"Team Name","abbreviation":"TM"}`))
	}))
	defer testServer.Close()

	provider := newSportarrSportsProvider(testServer.Client())
	provider.baseURL = testServer.URL
	var wait sync.WaitGroup
	errs := make(chan error, 12)
	for index := 0; index < 12; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := provider.team(context.Background(), "team-id")
			errs <- err
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("team metadata request: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one coalesced metadata request, got %d", calls.Load())
	}
}

func TestSportarrSportsProviderCoalescesAndCachesEventImages(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	testServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"images":[{"image_type":"backdrop","url":"https://images.example/event.jpg","is_primary":true}]}`))
	}))
	defer testServer.Close()

	provider := newSportarrSportsProvider(testServer.Client())
	provider.baseURL = testServer.URL
	var wait sync.WaitGroup
	errs := make(chan error, 12)
	for index := 0; index < 12; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			imageURL, err := provider.eventImage(context.Background(), "event-id")
			if imageURL != "https://images.example/event.jpg" && err == nil {
				err = fmt.Errorf("unexpected image URL %q", imageURL)
			}
			errs <- err
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("event image request: %v", err)
		}
	}
	if _, err := provider.eventImage(context.Background(), "event-id"); err != nil {
		t.Fatalf("cached event image request: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one coalesced and cached image request, got %d", calls.Load())
	}
}

func TestSportarrSportsProviderBoundsMetadataCaches(t *testing.T) {
	t.Parallel()

	provider := newSportarrSportsProvider(nil)
	for index := 0; index < sportarrTeamCacheLimit+8; index++ {
		id := fmt.Sprintf("team-%d", index)
		provider.storeTeamLocked(id, sportarrTeamCacheEntry{Team: sportarrTeam{ID: id}, ExpiresAt: time.Now().Add(time.Duration(index) * time.Minute)})
	}
	for index := 0; index < sportarrLeagueCacheLimit+8; index++ {
		id := fmt.Sprintf("league-%d", index)
		provider.storeLeagueLocked(id, sportarrLeagueCacheEntry{League: sportarrLeague{ID: id}, ExpiresAt: time.Now().Add(time.Duration(index) * time.Minute)})
	}
	for index := 0; index < sportarrImageCacheLimit+8; index++ {
		id := fmt.Sprintf("event-%d", index)
		provider.storeImageLocked(id, sportarrImageCacheEntry{ImageURL: "https://images.example/" + id + ".jpg", ExpiresAt: time.Now().Add(time.Duration(index) * time.Minute)})
	}
	if len(provider.teams) != sportarrTeamCacheLimit || len(provider.leagues) != sportarrLeagueCacheLimit || len(provider.images) != sportarrImageCacheLimit {
		t.Fatalf("expected bounded metadata caches, got %d teams, %d leagues, and %d images", len(provider.teams), len(provider.leagues), len(provider.images))
	}
}

func TestSportarrSportsProviderEnrichesMatchedEvents(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/teams/home-id":
			_, _ = response.Write([]byte(`{"id":"home-id","name":"New York Liberty","abbreviation":"NYL","logoUrl":"https://images.example/home.png","primaryColor":"#112233"}`))
		case "/teams/away-id":
			_, _ = response.Write([]byte(`{"id":"away-id","name":"Las Vegas Aces","abbreviation":"LVA","logoUrl":"https://images.example/away.png"}`))
		case "/leagues/league-id":
			_, _ = response.Write([]byte(`{"id":"league-id","name":"WNBA","sportName":"Basketball","description":"Professional women's basketball.","logoUrl":"https://images.example/league.png"}`))
		case "/api/v1/images/entity/event/event-id":
			if request.URL.Query().Get("completed_only") != "true" {
				t.Errorf("expected completed_only=true, got %q", request.URL.Query().Get("completed_only"))
			}
			_, _ = response.Write([]byte(`{"images":[{"image_type":"poster","url":"https://images.example/poster.jpg","width":680,"height":1000,"is_primary":true,"priority":1},{"image_type":"backdrop","url":"https://images.example/backdrop.jpg","width":1920,"height":1080,"is_primary":true,"priority":2}]}`))
		default:
			http.NotFound(response, request)
		}
	})
	testServer := httptest.NewServer(handler)
	defer testServer.Close()

	provider := newSportarrSportsProvider(testServer.Client())
	provider.baseURL = testServer.URL
	events := []SportsEvent{{
		ProviderID: "event-id",
		LeagueID:   "league-id",
		Home:       SportsTeam{ID: "home-id", Name: "Liberty"},
		Away:       SportsTeam{ID: "away-id", Name: "Aces"},
		Channels:   []SportsChannelMatch{{ID: "channel-1"}},
	}}
	enriched := provider.EnrichEvents(context.Background(), events, 1)
	waitForSportarrEnrichment(t, provider)
	enriched = provider.EnrichEvents(context.Background(), events, 1)
	waitForSportarrEnrichment(t, provider)
	if enriched[0].SportName != "Basketball" || enriched[0].LeagueLogoURL == "" || enriched[0].LeagueDescription == "" {
		t.Fatalf("expected league enrichment, got %+v", enriched[0])
	}
	if enriched[0].Home.Abbreviation != "NYL" || enriched[0].Home.LogoURL == "" || enriched[0].Away.Abbreviation != "LVA" {
		t.Fatalf("expected team enrichment, got %+v", enriched[0])
	}
	if enriched[0].ImageURL != "https://images.example/backdrop.jpg" {
		t.Fatalf("expected canonical Sportarr event artwork, got %+v", enriched[0])
	}
	if enriched[0].Artwork == nil || enriched[0].Artwork.Backdrop == nil || enriched[0].Artwork.Backdrop.Width != 1920 || enriched[0].Artwork.Backdrop.Height != 1080 {
		t.Fatalf("expected typed backdrop dimensions, got %+v", enriched[0].Artwork)
	}
	if enriched[0].Artwork.Poster == nil || enriched[0].Artwork.Poster.URL != "https://images.example/poster.jpg" {
		t.Fatalf("expected typed poster artwork, got %+v", enriched[0].Artwork)
	}
}

func TestSportarrEnrichmentPreservesExistingIdentityFallbacks(t *testing.T) {
	t.Parallel()

	provider := newSportarrSportsProvider(nil)
	provider.leagues["nba"] = sportarrLeagueCacheEntry{League: sportarrLeague{ID: "nba", Name: "NBA"}}
	provider.teams["heat"] = sportarrTeamCacheEntry{Team: sportarrTeam{ID: "heat", Name: "Miami Heat"}}
	event := SportsEvent{
		LeagueID:          "nba",
		LeagueName:        "NBA",
		LeagueLogoURL:     "https://game-thumbs.swvn.io/nba/logo.png",
		LeagueDescription: "Existing description",
		SportName:         "Basketball",
		Away: SportsTeam{
			ID:             "heat",
			Name:           "Heat",
			LogoURL:        "https://game-thumbs.swvn.io/nba/heat/logo.png?variant=dark",
			PrimaryColor:   "#aa0000",
			SecondaryColor: "#000000",
		},
	}

	got := provider.applyCachedDetails(event)
	if got.LeagueLogoURL != event.LeagueLogoURL || got.LeagueDescription != event.LeagueDescription || got.SportName != event.SportName {
		t.Fatalf("empty Sportarr league metadata replaced existing identity: %+v", got)
	}
	if got.Away.LogoURL != event.Away.LogoURL || got.Away.PrimaryColor != event.Away.PrimaryColor || got.Away.SecondaryColor != event.Away.SecondaryColor {
		t.Fatalf("empty Sportarr team metadata replaced existing identity: %+v", got.Away)
	}
}

func TestSportarrEnrichmentDoesNotBlockSportsResponse(t *testing.T) {
	t.Parallel()

	testServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		time.Sleep(150 * time.Millisecond)
		response.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(request.URL.Path, "/images/entity/event/"):
			_, _ = response.Write([]byte(`{"images":[]}`))
		case strings.Contains(request.URL.Path, "/leagues/"):
			_, _ = response.Write([]byte(`{"id":"league-id","name":"League"}`))
		default:
			_, _ = response.Write([]byte(`{"id":"team-id","name":"Team"}`))
		}
	}))
	defer testServer.Close()

	provider := newSportarrSportsProvider(testServer.Client())
	provider.baseURL = testServer.URL
	events := []SportsEvent{{
		ProviderID: "event-id",
		LeagueID:   "league-id",
		Home:       SportsTeam{ID: "home-id"},
		Away:       SportsTeam{ID: "away-id"},
		Channels:   []SportsChannelMatch{{ID: "channel-id"}},
	}}
	started := time.Now()
	_ = provider.EnrichEvents(context.Background(), events, 1)
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("expected non-blocking enrichment, took %s", elapsed)
	}
	waitForSportarrEnrichment(t, provider)
}

func waitForSportarrEnrichment(t *testing.T, provider *sportarrSportsProvider) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		provider.metadataMu.Lock()
		running := provider.enriching
		provider.metadataMu.Unlock()
		if !running {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for Sportarr enrichment")
}

func TestSportarrSportsProviderRetriesTransientResponses(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	testServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Cache-Control") != "no-cache, no-store" || request.Header.Get("Pragma") != "no-cache" {
			t.Errorf("expected no-cache request headers")
		}
		if calls.Add(1) == 1 {
			response.Header().Set("Retry-After", "0")
			response.WriteHeader(http.StatusTooManyRequests)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"id":"team-id","name":"Recovered Team"}`))
	}))
	defer testServer.Close()

	provider := newSportarrSportsProvider(testServer.Client())
	provider.baseURL = testServer.URL
	team, err := provider.team(context.Background(), "team-id")
	if err != nil {
		t.Fatalf("expected transient response to recover: %v", err)
	}
	if calls.Load() != 2 || team.Name != "Recovered Team" {
		t.Fatalf("expected one retry and recovered payload, got %d calls and %+v", calls.Load(), team)
	}
}

func TestWaitForSportarrRetryHonorsContextForLongRetryAfter(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := waitForSportarrRetry(ctx, "5", 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("expected context to bound long Retry-After, took %s", elapsed)
	}
}

func TestPickSportarrEventImagePrefersBackdropAndPriority(t *testing.T) {
	t.Parallel()

	images := []sportarrEntityImage{
		{ImageType: "poster", URL: "https://images.example/poster.jpg", IsPrimary: true, Priority: 100},
		{ImageType: "backdrop", URL: "https://images.example/lower.jpg", Priority: 999},
		{ImageType: "backdrop", URL: "https://images.example/best.jpg", IsPrimary: true, Priority: 2},
		{ImageType: "backdrop", URL: "javascript:alert(1)", IsPrimary: true, Priority: 999},
	}
	if got := preferredSportsArtworkURL(pickSportarrEventArtwork(images)); got != "https://images.example/best.jpg" {
		t.Fatalf("expected preferred backdrop, got %q", got)
	}
}

func TestPickSportarrEventArtworkPreservesTypedSlotsAndDimensions(t *testing.T) {
	t.Parallel()

	images := []sportarrEntityImage{
		{ImageType: "poster", URL: "https://images.example/poster-low.jpg", Width: 340, Height: 500, Priority: 1},
		{ImageType: "poster", URL: "https://images.example/poster.jpg", Width: 680, Height: 1000, IsPrimary: true, Priority: 2},
		{ImageType: "backdrop", URL: "https://images.example/backdrop.jpg", Width: 1920, Height: 1080, IsPrimary: true},
		{ImageType: "thumbnail", URL: "https://images.example/thumb.jpg", Width: 640, Height: 360, Priority: 4},
		{ImageType: "logo", URL: "https://images.example/logo.png", Width: 800, Height: 300, IsPrimary: true},
		{ImageType: "banner", URL: "javascript:alert(1)", IsPrimary: true, Priority: 999},
	}

	artwork := pickSportarrEventArtwork(images)
	if artwork == nil || artwork.Poster == nil || artwork.Backdrop == nil || artwork.Thumbnail == nil || artwork.Logo == nil {
		t.Fatalf("expected all safe typed artwork slots, got %+v", artwork)
	}
	if artwork.Poster.URL != "https://images.example/poster.jpg" || artwork.Poster.Width != 680 || artwork.Poster.Height != 1000 {
		t.Fatalf("expected primary poster with dimensions, got %+v", artwork.Poster)
	}
	if artwork.Banner != nil {
		t.Fatalf("expected unsafe banner to be rejected, got %+v", artwork.Banner)
	}
}

func fetchSportsPayload(t *testing.T, server *HTTPRoutesServer) SportsPayload {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		payload := fetchSportsPayloadOnce(t, server, false)
		if !payload.Refreshing {
			return payload
		}
		if time.Now().After(deadline) {
			t.Fatalf("sports payload did not finish preparing: %+v", payload)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func fetchSportsPayloadOnce(t *testing.T, server *HTTPRoutesServer, refresh bool) SportsPayload {
	t.Helper()
	path := "/dispatcharr/api/sports"
	if refresh {
		path += "?refresh=1"
	}
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: path})
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

func TestRankSportsEventExplainsEditorialSignals(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	event := SportsEvent{
		Name:        "#3 Tigers vs #8 Bears Championship Rivalry",
		Round:       "Championship",
		Description: "A classic rivalry projected as a toss-up.",
		StartUnix:   now.Add(90 * time.Minute).Unix(),
		Channels:    []SportsChannelMatch{{ID: "channel:1"}},
	}
	ranking := rankSportsEvent(event, now)

	if ranking.Score <= 0 || ranking.Score > 10 {
		t.Fatalf("ranking score = %v, want within (0, 10]", ranking.Score)
	}
	keys := map[string]bool{}
	for _, signal := range ranking.Signals {
		keys[signal.Key] = true
	}
	for _, key := range []string{"soon", "stage", "rivalry", "rank", "close", "coverage"} {
		if !keys[key] {
			t.Fatalf("ranking signals = %#v, missing %q", ranking.Signals, key)
		}
	}
}

func TestSportsRankingDoesNotTreatMatchNumberAsTeamRank(t *testing.T) {
	t.Parallel()

	ranking := rankSportsEvent(SportsEvent{Name: "Leeds vs Super Giants - 10th Match"}, time.Unix(1_700_000_000, 0))
	for _, signal := range ranking.Signals {
		if signal.Key == "rank" {
			t.Fatalf("match number produced a rank signal: %#v", ranking.Signals)
		}
	}
}

func TestSportsRankingDoesNotTreatLeagueNameAsChampionshipStage(t *testing.T) {
	t.Parallel()

	ranking := rankSportsEvent(SportsEvent{Name: "EFL Championship: Leeds vs Derby"}, time.Unix(1_700_000_000, 0))
	for _, signal := range ranking.Signals {
		if signal.Key == "stage" {
			t.Fatalf("league name produced a stage signal: %#v", ranking.Signals)
		}
	}
}

func TestSportsRankingPrefersTypedProviderRankAndSpreadSignals(t *testing.T) {
	t.Parallel()

	spread := 2.5
	ranking := rankSportsEvent(SportsEvent{
		Name: "Team A vs Team B", HomeRank: 4, AwayRank: 9, Spread: &spread,
	}, time.Unix(1_700_000_000, 0))
	signals := map[string]SportsRankingSignal{}
	for _, signal := range ranking.Signals {
		signals[signal.Key] = signal
	}
	if !strings.Contains(signals["rank"].Detail, "provider-ranked") {
		t.Fatalf("rank signal = %#v, want typed provider evidence", signals["rank"])
	}
	if !strings.Contains(signals["close"].Detail, "three points") {
		t.Fatalf("close signal = %#v, want typed provider spread", signals["close"])
	}
	if ranking.Knee != sportsRankingKnee {
		t.Fatalf("ranking knee = %v, want %v", ranking.Knee, sportsRankingKnee)
	}
}

func TestStableSportsEventIdentitySurvivesReschedule(t *testing.T) {
	t.Parallel()

	base := SportsEvent{
		LeagueID: "nfl",
		Season:   "2026",
		Round:    "Week 4",
		Name:     "Jets at Giants",
		Home:     normalizeSportsTeam(SportsTeam{Name: "Giants"}),
		Away:     normalizeSportsTeam(SportsTeam{Name: "Jets"}),
	}
	first := base
	first.StartUnix = 1_700_000_000
	second := base
	second.StartUnix = 1_700_086_400

	if got, want := stableSportsEventIdentity(first), stableSportsEventIdentity(second); got != want {
		t.Fatalf("stable identity changed across reschedule: %q != %q", got, want)
	}
	first.ProviderID = "provider-event-42"
	second.ProviderID = "provider-event-42"
	if got, want := stableSportsEventIdentity(first), stableSportsEventIdentity(second); got != want {
		t.Fatalf("provider identity changed across reschedule: %q != %q", got, want)
	}
	second.ProviderSource = "another-provider"
	if got, want := stableSportsEventIdentity(first), stableSportsEventIdentity(second); got == want {
		t.Fatalf("provider identity collided across sources: %q == %q", got, want)
	}
}

func TestSportsChannelMatchingReportsEvidenceConfidenceAndGuardrails(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.August, 31, 20, 0, 0, 0, time.UTC).Unix()
	event := SportsEvent{
		LeagueID: "nfl", LeagueName: "NFL", Name: "Jets at Giants", StartUnix: start,
		Away: SportsTeam{Name: "New York Jets", Abbreviation: "NYJ"},
		Home: SportsTeam{Name: "New York Giants", Abbreviation: "NYG"},
	}
	snapshot := cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{
			{ID: "exact", Name: "NYJ vs NYG", CategoryID: "nfl", CategoryName: "NFL"},
			{ID: "split", Name: "NYJ studio", CategoryID: "nfl", CategoryName: "NYG | NFL"},
			{ID: "preview", Name: "NYJ vs NYG Pregame Preview", CategoryID: "nfl", CategoryName: "NFL"},
			{ID: "stale", Name: "NYJ vs NYG 2026-08-30", CategoryID: "nfl", CategoryName: "NFL"},
			{ID: "epg", Name: "National Sports", CategoryID: "nfl", CategoryName: "NFL"},
		},
		Programs: []model.Program{{ID: "program", ChannelID: "epg", Title: "NFL: New York Jets vs New York Giants", StartUnix: start, EndUnix: start + 7200}},
		Content:  model.ContentState{LiveCategories: []model.Category{{ID: "nfl", Name: "NFL", Kind: "live"}}},
	}}

	matches := matchSportsChannels(event, snapshot)
	byID := map[string]SportsChannelMatch{}
	for _, match := range matches {
		byID[match.ID] = match
	}
	if match := byID["exact"]; match.Confidence != "high" || match.Evidence != "channel" {
		t.Fatalf("exact match = %#v, want high-confidence channel evidence", match)
	}
	if match := byID["epg"]; match.Confidence != "high" || match.Evidence != "epg" {
		t.Fatalf("EPG match = %#v, want high-confidence EPG evidence", match)
	}
	if _, ok := byID["split"]; ok {
		t.Fatalf("split-segment teams should not create a match: %#v", byID["split"])
	}
	if _, ok := byID["stale"]; ok {
		t.Fatalf("stale dated feed should not create a match: %#v", byID["stale"])
	}
	if preview, ok := byID["preview"]; ok && preview.Score >= byID["exact"].Score {
		t.Fatalf("preview score %d should be below exact score %d", preview.Score, byID["exact"].Score)
	}
	_, diagnostics := newSportsChannelIndex(snapshot).MatchDetailed(event)
	rejections := map[string]string{}
	for _, diagnostic := range diagnostics {
		if !diagnostic.Accepted {
			rejections[diagnostic.ChannelID] = diagnostic.Reason
		}
	}
	for _, channelID := range []string{"split", "stale"} {
		if rejections[channelID] == "" {
			t.Fatalf("diagnostics = %#v, missing rejection for %q", diagnostics, channelID)
		}
	}
}

func TestSportsPayloadOmitsUnmatchedAndLowConfidenceEvents(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.September, 2, 20, 0, 0, 0, time.UTC).Unix()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{
			{ID: "exact", Name: "NYJ vs NYG", CategoryID: "nfl", CategoryName: "NFL"},
			{ID: "preview", Name: "NYJ vs NYG Pregame Preview", CategoryID: "nfl", CategoryName: "NFL"},
			{ID: "music", Name: "AyazTV Persian Music", CategoryID: "music", CategoryName: "International TV | Music"},
		},
		Content: model.ContentState{LiveCategories: []model.Category{{ID: "nfl", Name: "NFL", Kind: "live"}, {ID: "music", Name: "International TV | Music", Kind: "live"}}},
	}})
	server := newEnabledSportsTestServer(store)
	server.sportsProvider = staticSportsProvider{events: []SportsEvent{
		{
			ID: "event:playable", LeagueID: "nfl", LeagueName: "NFL", Name: "Jets at Giants", StartUnix: start,
			Away: SportsTeam{Name: "New York Jets", Abbreviation: "NYJ"},
			Home: SportsTeam{Name: "New York Giants", Abbreviation: "NYG"},
		},
		{
			ID: "event:unmatched", LeagueID: "pga", LeagueName: "PGA Tour", Name: "Korn Ferry Championship", StartUnix: start,
			Away: SportsTeam{Name: "Korn Ferry"}, Home: SportsTeam{Name: "PGA Tour"},
		},
	}}

	payload := fetchSportsPayload(t, server)
	if len(payload.Events) != 1 || payload.Events[0].ID != "event:playable" {
		t.Fatalf("expected only the confident channel match, got %+v", payload.Events)
	}
	for _, match := range payload.Events[0].Channels {
		if match.Confidence == "low" {
			t.Fatalf("playable sports events must not keep low-confidence feeds: %+v", payload.Events[0].Channels)
		}
	}
	assertSportsMatch(t, payload.Events[0].Channels, "exact")
	assertNoSportsMatch(t, payload.Events[0].Channels, "preview")
	assertNoSportsMatch(t, payload.Events[0].Channels, "music")
}

func TestSportsWeakTeamAliasRejectsGenericNames(t *testing.T) {
	t.Parallel()

	if got := sportsWeakTeamAlias("Manchester United"); got != "" {
		t.Fatalf("generic alias = %q, want empty", got)
	}
	if got := sportsWeakTeamAlias("Huddersfield Town"); got != "" {
		t.Fatalf("short alias = %q, want empty", got)
	}
	if got := sportsWeakTeamAlias("Portland Timbers"); got != "timbers" {
		t.Fatalf("specific alias = %q, want timbers", got)
	}
}
