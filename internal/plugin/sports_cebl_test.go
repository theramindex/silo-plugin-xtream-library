package plugin

import (
	"testing"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
)

func TestCEBLTeamsUseOfficialLogosOnlyWithinTheirLeague(t *testing.T) {
	for _, name := range []string{"Calgary", "Calgary Surge", "Winnipeg", "Niagara River Lions", "Edmonton", "Scarborough", "Montréal Alliance", "Ottawa", "Vancouver", "Saskatoon Mamba", "Brampton"} {
		event := applySportsIdentityFallbacks(SportsEvent{LeagueID: "cebl", Away: SportsTeam{Name: name}, Home: SportsTeam{Name: "TBD"}})
		if !isCEBLTeamLogo(event.Away.LogoURL) || event.LeagueLogoURL != ceblLeagueLogoURL || event.Home.LogoURL != "" {
			t.Fatalf("CEBL team %q must use its own logo: %+v", name, event)
		}
	}
	otherSport := applySportsIdentityFallbacks(SportsEvent{LeagueID: "nhl", Away: SportsTeam{Name: "Calgary"}, Home: SportsTeam{Name: "Winnipeg"}})
	if isCEBLTeamLogo(otherSport.Away.LogoURL) || isCEBLTeamLogo(otherSport.Home.LogoURL) || ceblTeamLogo("Calgary Flames") != "" {
		t.Fatal("CEBL identities must not leak into other teams or sports")
	}
	provided := applySportsIdentityFallbacks(SportsEvent{LeagueID: "cebl", Away: SportsTeam{Name: "Calgary", LogoURL: "https://example.com/provider.png"}})
	if provided.Away.LogoURL != "https://example.com/provider.png" {
		t.Fatal("preserve an existing provider logo")
	}
}

func TestInternationalCricketUsesICCWithoutBorrowingDomesticLogo(t *testing.T) {
	event := applySportsIdentityFallbacks(SportsEvent{LeagueID: "cricket", LeagueName: "Cricket", Name: "Cricket Highlights : England vs Pakistan: 2nd Test, Day 4", Away: SportsTeam{Name: "England"}, Home: SportsTeam{Name: "Pakistan"}})
	if event.LeagueLogoURL != iccLeagueLogoURL || event.Away.LogoURL == "" || event.Home.LogoURL == "" {
		t.Fatalf("international cricket needs ICC and country flags: %+v", event)
	}
	domestic := applySportsIdentityFallbacks(SportsEvent{LeagueID: "cricket", Name: "The Hundred: Leeds vs Super Giants", Away: SportsTeam{Name: "Leeds"}, Home: SportsTeam{Name: "Super Giants"}})
	if domestic.LeagueLogoURL == iccLeagueLogoURL || domestic.LeagueLogoURL == "" {
		t.Fatal("a known domestic competition keeps its own logo")
	}
	unknown := applySportsIdentityFallbacks(SportsEvent{LeagueID: "cricket", Away: SportsTeam{Name: "Gallants"}, Home: SportsTeam{Name: "Kaps"}})
	if unknown.LeagueLogoURL == iccLeagueLogoURL {
		t.Fatal("unidentified clubs must not be represented as an international series")
	}
	server := newEnabledSportsTestServer(cache.NewStore())
	server.sportsImages = newSportsImageCache(t.TempDir(), nil)
	cebl := applySportsIdentityFallbacks(SportsEvent{LeagueID: "cebl", Away: SportsTeam{Name: "Calgary"}, Home: SportsTeam{Name: "Winnipeg"}})
	proxied := server.proxySportsEventImages([]SportsEvent{event, cebl})
	if proxied[0].LeagueLogoURL != event.LeagueLogoURL || proxied[1].Away.LogoURL != cebl.Away.LogoURL || proxied[1].Home.LogoURL != cebl.Home.LogoURL {
		t.Fatal("provider refresh must retain the verified public logos")
	}
}
