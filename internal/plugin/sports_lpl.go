package plugin

import "strings"

// Official 2026 league and franchise marks from lankapremierleaguet20.com/teams/.
const lplLogoBaseURL = "https://www.lankapremierleaguet20.com/wp-content/uploads/2026/06/"
const lplLeagueLogoURL = lplLogoBaseURL + "LPL-2026-Logo-W-300x169.png"

var lplTeams = []struct {
	name    string
	aliases []string
	logo    string
}{
	{"Galle Gallants", []string{"galle", "gallants", "galle gallants"}, "Group-3.png"},
	{"Colombo Kaps", []string{"colombo", "kaps", "colombo kaps"}, "Group-1.png"},
	{"Dambulla Sixers", []string{"dambulla", "sixers", "dambulla sixers"}, "Group-5.png"},
	{"Kandy Royals", []string{"kandy", "royals", "kandy royals"}, "Group-2.png"},
	{"Jaffna Kings", []string{"jaffna", "kings", "jaffna kings", "sc jaffna kings"}, "Group-4.png"},
}

func normalizeLPLTeams(event SportsEvent) SportsEvent {
	if event.LeagueID != "lanka-premier-league" {
		return event
	}
	if event.Round == "" {
		event.Round = strings.Trim(guideSportsStageSuffix.FindString(event.Name), " -–—")
	}
	if event.LeagueLogoURL == "" {
		event.LeagueLogoURL = lplLeagueLogoURL
	}
	for _, team := range []*SportsTeam{&event.Away, &event.Home} {
		name := normalizeSportsIdentityText(cleanGuideSportsTeamName(team.Name))
		for _, known := range lplTeams {
			for _, alias := range known.aliases {
				if name == alias {
					team.Name = known.name
					if team.LogoURL == "" {
						team.LogoURL = lplLogoBaseURL + known.logo
					}
				}
			}
		}
	}
	return event
}
