package plugin

import (
	"strings"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

func TestLPLGuideIdentifiesTeamsCompetitionAndNumberedRound(t *testing.T) {
	now := time.Now()
	title := "Cricket Highlights : LPL 2026: Gallants vs Kaps - Qualifier 2"
	events := sportsEventsFromGuide(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "willow", Name: "Willow 2"}},
		Programs: []model.Program{{ID: "lpl", ChannelID: "willow", Title: title, StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix()}},
	}}, now)
	if len(events) != 1 {
		t.Fatalf("expected one LPL broadcast, got %d", len(events))
	}
	e := events[0]
	if e.LeagueID != "lanka-premier-league" || e.LeagueName != "Lanka Premier League" || e.SportName != "Cricket" || e.Round != "Qualifier 2" || e.Status != "highlights" {
		t.Fatalf("incorrect competition or round: %+v", e)
	}
	if e.Away.Name != "Galle Gallants" || e.Home.Name != "Colombo Kaps" || e.LeagueLogoURL != lplLeagueLogoURL || !strings.HasSuffix(e.Away.LogoURL, "/Group-3.png") || !strings.HasSuffix(e.Home.LogoURL, "/Group-1.png") || e.GameThumbsBackgroundURL != "" {
		t.Fatalf("LPL must use its own teams and artwork: %+v", e)
	}
	if len(e.Channels) != 1 || e.Channels[0].ID != "willow" {
		t.Fatal("normalizing team names must retain the playable channel")
	}
	for _, suffix := range []string{" - Qualifier 1", " – Qualifier 02", " — Eliminator", " - Semi-Final 2"} {
		_, home, ok := guideSportsMatchup("Cricket: Gallants vs Kaps" + suffix)
		if !ok || home != "Kaps" {
			t.Fatalf("round suffix must not enter the team name: %q", suffix)
		}
	}
}

func TestLPLIdentityDoesNotBorrowOtherLeagues(t *testing.T) {
	if id, _, _, _ := guideSportsLeague("League of Legends LPL: Team A vs Team B"); id == "lanka-premier-league" {
		t.Fatal("LPL esports is not cricket")
	}
	for _, known := range lplTeams {
		e := normalizeSportsEvents([]SportsEvent{{LeagueID: "lanka-premier-league", Away: SportsTeam{Name: known.aliases[1]}, Home: SportsTeam{Name: "TBD"}}})[0]
		if e.Away.Name != known.name || e.Away.LogoURL != lplLogoBaseURL+known.logo || e.Home.LogoURL != "" {
			t.Fatalf("incorrect LPL franchise identity: %+v", e)
		}
	}
	other := normalizeLPLTeams(SportsEvent{LeagueID: "ipl", Away: SportsTeam{Name: "Royals"}, Home: SportsTeam{Name: "Kings"}})
	if other.Away.Name != "Royals" || other.Home.Name != "Kings" {
		t.Fatal("franchise nicknames must only expand within LPL")
	}
	server := newEnabledSportsTestServer(cache.NewStore())
	server.sportsImages = newSportsImageCache(t.TempDir(), nil)
	e := normalizeSportsEvents([]SportsEvent{{LeagueID: "lanka-premier-league", Away: SportsTeam{Name: "Gallants"}, Home: SportsTeam{Name: "Kaps"}}})[0]
	proxied := server.proxySportsEventImages([]SportsEvent{e})[0]
	if proxied.LeagueLogoURL != e.LeagueLogoURL || proxied.Home.LogoURL != e.Home.LogoURL || proxied.Away.LogoURL != e.Away.LogoURL {
		t.Fatal("provider refresh must retain the verified official artwork")
	}
}
