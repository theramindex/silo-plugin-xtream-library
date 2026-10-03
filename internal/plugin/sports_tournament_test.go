package plugin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

func tournamentGuideEvents(t *testing.T, now time.Time, channelName, categoryName string, programs ...model.Program) map[string]SportsEvent {
	t.Helper()
	for index := range programs {
		programs[index].ChannelID = "channel:tournament"
		if programs[index].ID == "" {
			programs[index].ID = "program:" + programs[index].Title
		}
		if programs[index].StartUnix == 0 {
			programs[index].StartUnix = now.Add(-time.Hour).Unix()
			programs[index].EndUnix = now.Add(2 * time.Hour).Unix()
		}
	}
	snapshot := cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "channel:tournament", Name: channelName, CategoryID: "category", CategoryName: categoryName}},
		Programs: programs,
	}}
	if categoryName != "" {
		snapshot.Catalog.Content.LiveCategories = []model.Category{{ID: "category", Name: categoryName, Kind: "live"}}
	}
	events := sportsEventsFromGuide(snapshot, now)
	byName := map[string]SportsEvent{}
	for _, event := range events {
		byName[event.Name] = event
	}
	return byName
}

func sportsEventNames(events map[string]SportsEvent) []string {
	names := make([]string, 0, len(events))
	for name := range events {
		names = append(names, name)
	}
	return names
}

// US Open listings on a general channel with no guide categories: the tennis
// major, its matchups, the golf major and the soccer Open Cup must each land in
// the right competition.
func TestGuideUSOpenListingsOnUncategorizedChannel(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.September, 5, 18, 0, 0, 0, time.UTC)
	events := tournamentGuideEvents(t, now, "ESPN", "",
		model.Program{Title: "2026 US Open Tennis Championships"},
		model.Program{Title: "U.S. Open Tennis"},
		model.Program{Title: "2026 US Open: Quarterfinals"},
		model.Program{Title: "US Open: Sinner vs. Alcaraz"},
		// Normalizes to the same text as the colon form, so it airs separately
		// to be its own listing rather than another channel of that one.
		model.Program{Title: "US Open Sinner vs Alcaraz", StartUnix: now.Add(20 * time.Hour).Unix(), EndUnix: now.Add(23 * time.Hour).Unix()},
		model.Program{Title: "2026 U.S. Open Golf: Final Round"},
		model.Program{Title: "US Open Cup Soccer: Sounders vs Galaxy"},
	)
	for _, title := range []string{"2026 US Open Tennis Championships", "U.S. Open Tennis", "2026 US Open: Quarterfinals"} {
		event, ok := events[title]
		if !ok {
			t.Fatalf("one-sided tennis listing %q was dropped; got %v", title, sportsEventNames(events))
		}
		if event.LeagueID != "us-open-tennis" || event.SportName != "Tennis" || event.EventType != "event" {
			t.Fatalf("%q classified as %q/%q/%q", title, event.LeagueID, event.SportName, event.EventType)
		}
		if event.Home.ID != "" || event.Away.ID != "" {
			t.Fatalf("one-sided listing %q must not create team identities: %+v %+v", title, event.Home, event.Away)
		}
	}
	for _, title := range []string{"US Open: Sinner vs. Alcaraz", "US Open Sinner vs Alcaraz"} {
		event, ok := events[title]
		if !ok {
			t.Fatalf("tennis matchup %q was dropped; got %v", title, sportsEventNames(events))
		}
		if event.LeagueID != "us-open-tennis" || event.SportName != "Tennis" {
			t.Fatalf("%q classified as %q/%q", title, event.LeagueID, event.SportName)
		}
		if event.Away.Name != "Sinner" || event.Home.Name != "Alcaraz" {
			t.Fatalf("%q parsed competitors %q / %q", title, event.Away.Name, event.Home.Name)
		}
	}
	golf, ok := events["2026 U.S. Open Golf: Final Round"]
	if !ok || golf.LeagueID != "us-open-golf" || golf.SportName != "Golf" {
		t.Fatalf("golf U.S. Open = %+v (present %v)", golf, ok)
	}
	if cup, ok := events["US Open Cup Soccer: Sounders vs Galaxy"]; ok && (cup.SportName == "Tennis" || cup.LeagueID == "us-open-tennis") {
		t.Fatalf("the U.S. Open Cup is soccer, got %q/%q", cup.LeagueID, cup.SportName)
	}
}

func TestGuideAdmitsOneSidedTennisMajorListings(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.September, 5, 18, 0, 0, 0, time.UTC)
	events := tournamentGuideEvents(t, now, "ESPN", "Entertainment",
		model.Program{Title: "Wimbledon: Gentlemen's Final"},
		model.Program{Title: "Roland-Garros: Day 14"},
		model.Program{Title: "Australian Open: Women's Semifinals"},
	)
	for title, leagueID := range map[string]string{
		"Wimbledon: Gentlemen's Final":        "wimbledon",
		"Roland-Garros: Day 14":               "roland-garros",
		"Australian Open: Women's Semifinals": "australian-open",
	} {
		event, ok := events[title]
		if !ok {
			t.Fatalf("one-sided tournament listing %q was dropped; got %v", title, sportsEventNames(events))
		}
		if event.LeagueID != leagueID || event.SportName != "Tennis" || event.EventType != "event" {
			t.Fatalf("%q classified as %q/%q/%q", title, event.LeagueID, event.SportName, event.EventType)
		}
	}
}

func TestGuideKeepsGenericOneSidedListingsOut(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.September, 5, 18, 0, 0, 0, time.UTC)
	events := tournamentGuideEvents(t, now, "ESPN", "Sports",
		model.Program{Title: "SportsCenter"},
		model.Program{Title: "NBA Today"},
		model.Program{Title: "US Open Preview Show"},
		model.Program{Title: "Wimbledon Highlights"},
		model.Program{Title: "Tennis Tonight"},
		model.Program{Title: "Lamar Hunt U.S. Open Cup Final"},
		model.Program{Title: "F1 Pre-Game Show"},
	)
	if len(events) != 0 {
		t.Fatalf("generic or studio listings must stay out of sports events, got %v", sportsEventNames(events))
	}
}

func TestGuideAdmitsGolfMajorsAndMotorsportWithContext(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.June, 20, 18, 0, 0, 0, time.UTC)
	golf := tournamentGuideEvents(t, now, "NBC", "Golf",
		model.Program{Title: "2026 U.S. Open: Third Round"},
		model.Program{Title: "Masters Tournament: Final Round"},
		model.Program{Title: "PGA Championship: Second Round"},
		model.Program{Title: "The Open Championship: Final Round"},
	)
	for title, leagueID := range map[string]string{
		"2026 U.S. Open: Third Round":        "us-open-golf",
		"Masters Tournament: Final Round":    "masters-tournament",
		"PGA Championship: Second Round":     "pga-championship",
		"The Open Championship: Final Round": "the-open",
	} {
		if event, ok := golf[title]; !ok || event.LeagueID != leagueID || event.SportName != "Golf" {
			t.Fatalf("golf major %q = %+v (present %v)", title, event, ok)
		}
	}
	motorsport := tournamentGuideEvents(t, now, "ESPN2", "Entertainment",
		model.Program{Title: "Formula 1: Italian Grand Prix"},
	)
	if event, ok := motorsport["Formula 1: Italian Grand Prix"]; !ok || event.LeagueID != "formula-1" {
		t.Fatalf("one-sided Grand Prix listing must be admitted, got %v", sportsEventNames(motorsport))
	}
}

func TestGuideTennisMajorMatchupsResolveLeagueAndCompetitors(t *testing.T) {
	t.Parallel()
	for _, title := range []string{"US Open: Sinner vs. Alcaraz", "US Open Sinner vs Alcaraz", "2026 US Open Tennis Sinner vs Alcaraz", "Wimbledon Sinner vs. Alcaraz"} {
		away, home, ok := guideSportsMatchup(title)
		if !ok || away != "Sinner" || home != "Alcaraz" {
			t.Fatalf("%q parsed competitors %q / %q (%v)", title, away, home, ok)
		}
		match := guideSportsLeagueMatch(title)
		if !match.OK || match.Sport != "Tennis" || match.ID == "sports" {
			t.Fatalf("%q must resolve to a tennis major, got %+v", title, match)
		}
	}
	if id, _, _, _ := guideSportsLeague("U.S. Open Golf: Scheffler vs McIlroy"); id != "us-open-golf" {
		t.Fatalf("golf context must select the golf U.S. Open, got %q", id)
	}
	if match := guideSportsLeagueMatch("Lamar Hunt U.S. Open Cup: Sounders vs Galaxy"); match.Tournament || match.Sport == "Tennis" {
		t.Fatalf("the U.S. Open Cup is soccer, got %+v", match)
	}
	if match := guideSportsLeagueMatch("NFL Football: Jets at Giants"); match.Tournament {
		t.Fatalf("team leagues must not admit one-sided listings, got %+v", match)
	}
	if match := guideSportsLeagueMatch("Sports Tonight"); match.Tournament || match.ID != "sports" {
		t.Fatalf("generic fallback must never be a tournament, got %+v", match)
	}
}

func TestNamelessTeamsCarryNoIdentity(t *testing.T) {
	t.Parallel()
	// The phantom ID earlier builds gave every nameless side: guide listings
	// built them as {Name: "", Abbreviation: initials("") == ""}.
	if got := stableSportsTeamID(SportsTeam{Name: "", Abbreviation: sportsTeamInitials("")}); got != legacyEmptySportsTeamID {
		t.Fatalf("legacy phantom team ID changed: %q", got)
	}
	if team := normalizeSportsTeam(SportsTeam{ID: "provider:1", Abbreviation: "X"}); team.ID != "" {
		t.Fatalf("nameless team kept ID %q", team.ID)
	}
	finalized := finalizeSportsTeamIdentity(SportsTeam{})
	if finalized.ID != "" || finalized.FollowIDs != nil || finalized.LegacyIDs != nil {
		t.Fatalf("nameless team must not be followable: %+v", finalized)
	}
	encoded, err := json.Marshal(finalized)
	if err != nil || strings.Contains(string(encoded), "followIds") || strings.Contains(string(encoded), "legacyIds") {
		t.Fatalf("nameless team JSON must omit follow identities: %s (%v)", encoded, err)
	}
	events := normalizeSportsEvents([]SportsEvent{
		{Name: "2026 US Open Tennis Championships", EventType: "event", StartUnix: 1},
		{Name: "Formula 1: Italian Grand Prix", EventType: "event", StartUnix: 2},
	})
	for _, event := range events {
		if event.Home.ID != "" || event.Away.ID != "" {
			t.Fatalf("one-sided event emitted team IDs: %+v", event)
		}
	}
	// Event identities keep the key earlier builds derived from the shared
	// empty-name hash, so saved event reminders still resolve.
	legacy := "sports-event:" + sportsHash(strings.Join([]string{"formula-1", "", "", "event", legacyEmptySportsTeamID, legacyEmptySportsTeamID, "formula 1: italian grand prix"}, "|"))
	if events[1].StableID != legacy {
		t.Fatalf("stable event identity changed: %q != %q", events[1].StableID, legacy)
	}
	// A roster never lists a nameless side.
	if teams := mergeSportsLeagueRosterTeams("formula-1", "Formula 1", "Motorsport", []SportsTeam{{}, {Abbreviation: "F1"}}); len(teams) != 0 {
		t.Fatalf("nameless roster entries must be dropped: %+v", teams)
	}
}

func TestSportsTeamLegacyIDsCoverMergedAndShortNames(t *testing.T) {
	t.Parallel()
	// Xtream keeps the guide copy's identity when a provider game merges in.
	guide := normalizeSportsTeam(SportsTeam{Name: "Yankees", Abbreviation: sportsTeamInitials("Yankees")})
	provider := normalizeSportsTeam(SportsTeam{ID: "sportarr-team:10", Name: "New York Yankees", Abbreviation: "NYY"})
	merged := finalizeSportsTeamIdentity(mergeSportsTeamIdentity(guide, provider))
	if merged.ID != guide.ID || merged.Name != "New York Yankees" {
		t.Fatalf("merge kept the wrong identity: %+v", merged)
	}
	legacy := strings.Join(merged.LegacyIDs, ",")
	if !strings.Contains(legacy, provider.ID) {
		t.Fatalf("provider copy's ID %q missing from legacyIds %v", provider.ID, merged.LegacyIDs)
	}
	for _, id := range append([]string{merged.ID}, merged.FollowIDs...) {
		if strings.Contains(legacy, id) {
			t.Fatalf("legacyIds must not repeat id/followIds (%q): %v", id, merged.LegacyIDs)
		}
	}
	if strings.Contains(legacy, legacyEmptySportsTeamID) {
		t.Fatal("legacyIds must never include the phantom empty-name ID")
	}

	// A Sportarr-only team still resolves follows saved on guide short names.
	alone := finalizeSportsTeamIdentity(normalizeSportsTeam(SportsTeam{ID: "sportarr-team:20", Name: "Michigan Wolverines"}))
	shortID := stableSportsTeamID(SportsTeam{Name: "Michigan", Abbreviation: "M"})
	nicknameID := stableSportsTeamID(SportsTeam{Name: "Wolverines", Abbreviation: "W"})
	ids := strings.Join(alone.LegacyIDs, ",")
	if !strings.Contains(ids, shortID) || !strings.Contains(ids, nicknameID) {
		t.Fatalf("short-name legacy IDs missing: %v", alone.LegacyIDs)
	}
	if len(alone.LegacyIDs) > sportsTeamLegacyIDLimit {
		t.Fatalf("legacyIds must stay bounded, got %d", len(alone.LegacyIDs))
	}
	seen := map[string]bool{}
	for _, id := range alone.LegacyIDs {
		if seen[id] {
			t.Fatalf("legacyIds must not repeat: %v", alone.LegacyIDs)
		}
		seen[id] = true
	}
	if generic := sportsTeamShortNames("Manchester United"); len(generic) != 1 || generic[0] != "Manchester" {
		t.Fatalf("generic nicknames must not become legacy names: %v", generic)
	}
}
