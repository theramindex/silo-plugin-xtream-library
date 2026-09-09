package plugin

import (
	"reflect"
	"strings"
	"testing"
)

func TestGameThumbsBroadLeagueMappings(t *testing.T) {
	cases := []struct{ title, slug string }{
		{"Women's College Field Hockey: Michigan at Duke", "ncaawfh"},
		{"Women's College Volleyball: Stanford at Oregon", "ncaawvb"},
		{"Men's College Soccer: Michigan at Notre Dame", "ncaas"},
		{"Women's College Gymnastics: UCLA at Utah", "ncaa"},
		{"Canadian Premier League Soccer: Inter Toronto at Forge", "can.1"},
		{"Scottish Premier League: Celtic at Rangers", "spfl"},
		{"NWSL Soccer: Washington Spirit at Portland Thorns", "usa.nwsl"},
		{"FIBA Women's Basketball World Cup: USA vs Italy", "fiba-women"},
		{"Rugby World Cup: France vs Italy", "rwc"},
		{"Women's Rugby World Cup: France vs Italy", "wrwc"},
		{"Formula 1: Italian Grand Prix", "F1"},
		{"Ontario Hockey League: London Knights at Ottawa 67s", "ohl"},
		{"Korean Baseball Organization: Samsung Lions at Kia Tigers", "kbo"},
		{"ATP Tour: Djokovic vs Alcaraz", "atp"},
		{"UEFA Women's Champions League: Chelsea at Barcelona", "uefa.wchampions"},
		{"Caribbean Premier League: Guyana at St Kitts", "cpl"},
		{"Mystery Sporting Event: College Park at New York", ""},
		{"Women's Volleyball Nations League: Canada vs Dominican Republic", ""},
		{"Men's Volleyball Nations League: Canada vs Argentina", ""},
		{"Women's Soccer: Borussia Dortmund vs AS Roma", ""},
	}
	for _, tc := range cases {
		if got := gameThumbsLeagueSlugForEvent(SportsEvent{Name: tc.title}); got != tc.slug {
			t.Errorf("%q: got %q, want %q", tc.title, got, tc.slug)
		}
	}
}

func TestGameThumbsArtworkPreservesFallbacksAndIdentity(t *testing.T) {
	input := SportsEvent{ID: "original-id", StableID: "stable-id", LeagueID: "nba", LeagueName: "NBA", Name: "Lakers at Celtics", LeagueLogoURL: "https://example.com/nba.png", Home: SportsTeam{ID: "home", Name: "Celtics", LogoURL: "https://example.com/celtics.png"}, Away: SportsTeam{ID: "away", Name: "Lakers"}}
	got := applySportsIdentityFallbacks(input)
	if got.ID != input.ID || got.StableID != input.StableID || got.Home.ID != input.Home.ID || got.LeagueID != input.LeagueID {
		t.Fatal("artwork changes must preserve event, team, and league identities")
	}
	if got.Home.LogoFallbackURL != input.Home.LogoURL || got.LeagueLogoFallbackURL != input.LeagueLogoURL || got.Home.LogoURL != gameThumbsTeamLogoURL("nba", "Celtics") {
		t.Fatalf("missing preferred/fallback logo: %+v", got)
	}
	if !strings.Contains(got.GameThumbsBackgroundURL, "/nba/lakers/celtics/thumb.png?") {
		t.Fatal("expected generated matchup background")
	}
	if again := applySportsIdentityFallbacks(got); !reflect.DeepEqual(got, again) {
		t.Fatal("repeated normalization must preserve preferred and fallback artwork")
	}
	program := applySportsIdentityFallbacks(SportsEvent{LeagueID: "boxing", EventType: "event", Name: "Two boxing bouts", Home: SportsTeam{Name: "Not a fighter"}})
	if strings.Contains(program.GameThumbsBackgroundURL, "not-a-fighter") {
		t.Fatal("programs must use league backgrounds rather than invented matchups")
	}
}

func TestVolleyballNationsLeagueKeepsNationalTeamArtwork(t *testing.T) {
	for _, tc := range []struct{ title, league string }{
		{"(CA) (CBC 01) | 2026 Women`s Volleyball Nations League: Canada vs Dominican Republic _ Hong Kong (2026-07-12 04:15:00)", "womens-volleyball-nations-league"},
		{"2026 Men's Volleyball Nations League: Canada vs Dominican Republic", "mens-volleyball-nations-league"},
	} {
		events := normalizeSportsEvents([]SportsEvent{{Name: tc.title, LeagueID: "sports", LeagueName: "Sports", SportName: "Sports", Away: SportsTeam{Name: "Canada"}, Home: SportsTeam{Name: "Dominican Republic"}}})
		if len(events) != 1 {
			t.Fatal("expected a normalized event")
		}
		event := events[0]
		if event.LeagueID != tc.league || event.SportName != "Volleyball" {
			t.Fatalf("wrong competition: %+v", event)
		}
		for _, artwork := range []string{event.LeagueLogoURL, event.GameThumbsBackgroundURL, event.Away.LogoURL, event.Home.LogoURL} {
			if strings.Contains(strings.ToLower(artwork), "ncaa") {
				t.Fatalf("international volleyball received college artwork: %s", artwork)
			}
		}
		if event.Away.LogoURL == "" || event.Home.LogoURL == "" {
			t.Fatal("national teams must retain their country flags")
		}
	}
}

func TestCrossLeagueMatchupKeepsIndividualClubArtwork(t *testing.T) {
	event := applySportsIdentityFallbacks(SportsEvent{LeagueID: "sports", LeagueName: "Sports", Name: "NWSL Soccer : Teal Rising Cup: Palmeiras vs. Chicago Stars", Away: SportsTeam{Name: "Palmeiras"}, Home: SportsTeam{Name: "Chicago Stars"}})
	if event.Away.LogoURL != gameThumbsTeamLogoURL("bra.1", "Palmeiras") || event.Home.LogoURL != gameThumbsTeamLogoURL("usa.nwsl", "Chicago Stars") {
		t.Fatalf("cross-league clubs need their own namespaces: %+v", event)
	}
	if event.GameThumbsBackgroundURL != "" {
		t.Fatalf("single-league composite would replace a visiting club with a placeholder: %s", event.GameThumbsBackgroundURL)
	}
}

func TestGenericGuideNHLMatchupUsesClubClassification(t *testing.T) {
	input := SportsEvent{ID: "epg:devils", LeagueID: "sports", LeagueName: "Sports", SportName: "Sports", Name: "Best of Devils : 2026: New Jersey Devils at Minnesota Wild", Away: SportsTeam{Name: "New Jersey Devils"}, Home: SportsTeam{Name: "Minnesota Wild"}}
	event := normalizeSportsEvents([]SportsEvent{input})[0]
	if event.LeagueID != "nhl" || event.LeagueName != "NHL" || event.SportName != "Hockey" || event.ID != input.ID {
		t.Fatalf("expected an NHL event with its existing source ID: %+v", event)
	}
	input.Home.Name = "Unknown opponent"
	if event := canonicalizeKnownSportsLeague(input); event.LeagueID != "sports" {
		t.Fatal("one recognized club is insufficient to infer a league")
	}
	input.Home.Name = "Minnesota Wild"
	input.LeagueID, input.LeagueName = "international-exhibition", "International Exhibition"
	if event := canonicalizeKnownSportsLeague(input); event.LeagueID != "international-exhibition" {
		t.Fatal("club inference must preserve an explicitly identified competition")
	}
}

func TestCanadianGuideCompetitionsRetainTheirSports(t *testing.T) {
	for _, tc := range []struct{ title, league, name, sport string }{
		{"(CA) (CBC 12) | HoopQueens Basketball: REIGN vs CHARGE (2026-07-12 21:00:00)", "hoopqueens", "HoopQueens Basketball", "Basketball"},
		{"Hoop Queens: TIDE vs BLAZE", "hoopqueens", "HoopQueens Basketball", "Basketball"},
		{"(CA) (CBC 09) | Canada Cup International Softball Championship: TBD vs TBD (2026-07-12 16:00:05)", "canada-cup-softball", "Canada Cup Softball", "Softball"},
	} {
		event := normalizeSportsEvents([]SportsEvent{{Name: tc.title, LeagueID: "sports", LeagueName: "Sports", SportName: "Sports"}})[0]
		if event.LeagueID != tc.league || event.LeagueName != tc.name || event.SportName != tc.sport {
			t.Fatalf("wrong competition for %q: %+v", tc.title, event)
		}
		if event.GameThumbsBackgroundURL != "" || event.LeagueLogoURL != "" {
			t.Fatalf("unsupported competitions must not borrow another league's artwork: %+v", event)
		}
	}
	if id, _, _, _ := guideSportsLeague("Canada Cup Ice Hockey"); id == "canada-cup-softball" {
		t.Fatal("Canada Cup alone does not identify softball")
	}
}
