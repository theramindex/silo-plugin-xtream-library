package plugin

import "testing"

func TestBasketballClubPairClassifiesGenericGuideEvent(t *testing.T) {
	input := SportsEvent{ID: "epg:knicks", LeagueID: "sports", LeagueName: "Sports", SportName: "Sports", Name: "Best of the Knicks : 2025-2026: New York Knicks at Washington Wizards", Away: SportsTeam{Name: "New York Knicks"}, Home: SportsTeam{Name: "Washington Wizards"}}
	event := normalizeSportsEvents([]SportsEvent{input})[0]
	if event.LeagueID != "nba" || event.LeagueName != "NBA" || event.SportName != "Basketball" {
		t.Fatalf("expected NBA basketball classification, got %s/%s/%s", event.LeagueID, event.LeagueName, event.SportName)
	}
	if event.ID != input.ID || event.Name != input.Name || event.Away.Name != input.Away.Name || event.Home.Name != input.Home.Name {
		t.Fatal("classification changed the guide identity or matchup")
	}
	for _, home := range []string{"TBD", "Washington", "Washington Nationals"} {
		input.Home.Name = home
		if got := canonicalizeKnownSportsLeague(input); got.LeagueID != "sports" {
			t.Fatalf("must require both basketball clubs, %s became %s", home, got.LeagueID)
		}
	}
	input.Home.Name = "Washington Wizards"
	input.LeagueID = "international-exhibition"
	if got := canonicalizeKnownSportsLeague(input); got.LeagueID != input.LeagueID {
		t.Fatal("must preserve a specific competition")
	}
}
