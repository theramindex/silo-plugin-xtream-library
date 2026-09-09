package plugin

import (
	"strings"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

func TestGuideBoxingCardKeepsBothBoutsWithoutInventingTeams(t *testing.T) {
	title := "Boxeo de Primera : Baltazar Noria vs. Lorenzo Gerez; Braian Argüello vs. Maximiliano Bonilla"
	if away, home, match := guideSportsMatchup(title); match || away != "" || home != "" {
		t.Fatal("a two-bout broadcast must not become one matchup")
	}
	now := time.Now()
	events := sportsEventsFromGuide(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "boxing", Name: "TyC Sports"}},
		Programs: []model.Program{{ID: "card", ChannelID: "boxing", Title: title, StartUnix: now.Add(-time.Hour).Unix(), EndUnix: now.Add(time.Hour).Unix()}},
	}}, now)
	if len(events) != 1 || events[0].LeagueID != "boxing" || events[0].EventType != "event" || events[0].Name != title || events[0].Home.Name != "" || events[0].Away.Name != "" {
		t.Fatalf("expected one boxing broadcast preserving both bouts: %+v", events)
	}
	if away, home, match := guideSportsMatchup("Boxeo: Noria vs. Gerez"); !match || away != "Noria" || home != "Gerez" {
		t.Fatal("single boxing bouts must still render as matchups")
	}
}

func TestSportsCardsUseScopedTeamLogosAndNationalFlags(t *testing.T) {
	events := normalizeSportsEvents([]SportsEvent{
		{ID: "classic", Name: "Cubs Classics : 2007: Dodgers at Cubs", Away: SportsTeam{Name: "Dodgers"}, Home: SportsTeam{Name: "Cubs"}},
		{ID: "cebl", Name: "CEBL: Brampton at Ottawa", Away: SportsTeam{Name: "Brampton"}, Home: SportsTeam{Name: "Ottawa"}},
		{ID: "countries", Name: "Volleyball: Argentina vs Cuba", Away: SportsTeam{Name: "Argentina"}, Home: SportsTeam{Name: "Cuba"}},
		{ID: "fiba", Name: "2026 FIBA Women's Basketball World Cup: Puerto Rico vs Belgium"},
	})
	if events[0].LeagueID != "mlb" || !strings.HasSuffix(events[0].Away.LogoFallbackURL, "/lad.png") || !strings.HasSuffix(events[0].Home.LogoFallbackURL, "/chc.png") {
		t.Fatalf("baseball classics must retain MLB artwork: %+v", events[0])
	}
	if events[1].LeagueID != "cebl" || !strings.Contains(events[1].Away.LogoURL, "BramptonHoneyBadgers") {
		t.Fatalf("CEBL Brampton must have its club logo: %+v", events[1])
	}
	if events[2].Away.LogoURL != "https://flagcdn.com/w160/ar.png" || events[2].Home.LogoURL != "https://flagcdn.com/w160/cu.png" {
		t.Fatal("national teams must receive their country flags")
	}
	if events[1].LeagueLogoURL != ceblLeagueLogoURL || events[3].LeagueLogoFallbackURL != fibaWomensLeagueLogoURL || events[3].LeagueLogoURL != gameThumbsLeagueLogoURL("fiba-women") {
		t.Fatal("CEBL and FIBA women's basketball must receive their official league marks")
	}
	if team := applyCountryTeamIdentity(SportsTeam{Name: "Brampton"}); team.LogoURL != "" {
		t.Fatal("a city club must not receive a national flag")
	}
	server := newEnabledSportsTestServer(cache.NewStore())
	server.sportsImages = newSportsImageCache(t.TempDir(), nil)
	refreshed := server.proxySportsEventImages(events)
	for i, event := range refreshed {
		if event.Home.LogoURL != events[i].Home.LogoURL || event.Away.LogoURL != events[i].Away.LogoURL || event.LeagueLogoURL != events[i].LeagueLogoURL {
			t.Fatal("provider refresh must preserve the working public identity URLs")
		}
	}
}
