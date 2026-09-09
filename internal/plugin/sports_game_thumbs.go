package plugin

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"strings"
)

// The registry covers Game Thumbs' built-in and Teamarr-backed league namespaces.
// See data/README.md for source revisions. No discovery requests run during sync.
//
//go:embed data/game_thumbs_leagues.json
var gameThumbsLeagueData []byte

type gameThumbsLeagueDefinition struct {
	Slug  string   `json:"slug"`
	Names []string `json:"names"`
	terms []string
}

var gameThumbsLeagues = func() []gameThumbsLeagueDefinition {
	var values []gameThumbsLeagueDefinition
	if err := json.Unmarshal(gameThumbsLeagueData, &values); err != nil {
		panic("invalid embedded Game Thumbs league registry: " + err.Error())
	}
	for i := range values {
		for _, name := range append([]string{values[i].Slug}, values[i].Names...) {
			term := gameThumbsMatchText(name)
			if len(term) < 3 || strings.Trim(term, "0123456789 ") == "" {
				continue
			}
			// Gender plus sport alone is not evidence of a college competition.
			// Imported aliases such as "womens volleyball" also describe national teams.
			if strings.HasPrefix(values[i].Slug, "ncaa") && !strings.HasPrefix(term, "ncaa") && !containsMatchTerm(term, "college") {
				continue
			}
			switch term {
			case "sports", "ncaa", "country", "flags", "football", "basketball", "hockey", "soccer", "premier", "england", "championship", "national league", "super league", "us open", "olympic", "olympics", "boxing", "mma", "tennis", "fifa", "world cup":
				continue
			}
			values[i].terms = append(values[i].terms, term)
		}
	}
	return values
}()

func gameThumbsMatchText(value string) string {
	return normalizeMatchText(strings.NewReplacer("'", "", "’", "", "`", "").Replace(value))
}

func gameThumbsKnownLeague(event SportsEvent) string {
	value := strings.Join([]string{event.LeagueID, event.LeagueName, event.SportName, event.Name, event.ShortName}, " ")
	if id, _, _ := guideCollegeCompetition(value); id != "" {
		competition := strings.TrimPrefix(id, "college-")
		if slug := map[string]string{
			"football": "ncaaf", "mens-football": "ncaaf",
			"basketball": "ncaam", "mens-basketball": "ncaam", "womens-basketball": "ncaaw",
			"baseball": "ncaabb", "mens-baseball": "ncaabb", "softball": "ncaasbw", "womens-softball": "ncaasbw",
			"soccer": "ncaas", "mens-soccer": "ncaas", "womens-soccer": "ncaaws",
			"hockey": "ncaah", "mens-hockey": "ncaah", "womens-hockey": "ncaawh",
			"volleyball": "ncaavb", "mens-volleyball": "ncaavb", "womens-volleyball": "ncaawvb",
			"lacrosse": "ncaalax", "mens-lacrosse": "ncaalax", "womens-lacrosse": "ncaawlax",
			"field-hockey": "ncaawfh", "womens-field-hockey": "ncaawfh",
		}[competition]; slug != "" {
			return slug
		}
		// School identities remain useful for college sports without a dedicated namespace.
		return "ncaa"
	}
	// Keep distinct World Cup competitions from being captured by the generic FIFA alias.
	if id, _, _ := guideWorldCupCompetition(value); id != "" {
		switch id {
		case "fiba-womens-world-cup":
			return "fiba-women"
		case "fiba-world-cup":
			return "fiba"
		case "fifa-womens-world-cup":
			return "fifa.wwc"
		case "fifa-womens-u20-world-cup", "fifa-u20-world-cup":
			// Do not replace a youth competition's identity with the senior tournament.
			return ""
		}
	}
	text := gameThumbsMatchText(value)
	best, longest := "", 0
	for _, league := range gameThumbsLeagues {
		// Exact provider IDs are supported without interpreting arbitrary EPG words as codes.
		if strings.EqualFold(event.LeagueID, league.Slug) {
			return league.Slug
		}
		for _, term := range league.terms {
			if len(term) > longest && containsMatchTerm(text, term) {
				best, longest = league.Slug, len(term)
			}
		}
	}
	return best
}

func preferGameThumbsLogo(current, fallback *string, generated string) {
	if generated == "" || *current == generated {
		return
	}
	if *current != "" && (!strings.HasPrefix(*current, gameThumbsPublicBaseURL+"/") || *fallback == "") {
		*fallback = *current
	}
	*current = generated
}

func applyGameThumbsArtwork(event SportsEvent, slug string) SportsEvent {
	if slug == "" {
		return event
	}
	if event.LeagueID != "sports" {
		preferGameThumbsLogo(&event.LeagueLogoURL, &event.LeagueLogoFallbackURL, gameThumbsLeagueLogoURL(slug))
	}
	for _, team := range []*SportsTeam{&event.Away, &event.Home} {
		if !usableSportsIdentityName(team.Name) {
			continue
		}
		// Multi-bout/program labels are not individual fighters or teams.
		if event.EventType == "event" || event.EventType == "race" {
			continue
		}
		if team.LogoFallbackURL == "" && strings.HasPrefix(slug, "ncaa") && slug != "ncaa" {
			team.LogoFallbackURL = gameThumbsTeamLogoURL("ncaa", team.Name)
		}
		preferGameThumbsLogo(&team.LogoURL, &team.LogoFallbackURL, gameThumbsTeamLogoURL(gameThumbsLeagueSlugForTeam(*team, slug), team.Name))
	}
	path := "/" + url.PathEscape(slug)
	if event.EventType != "event" && event.EventType != "race" && usableSportsIdentityName(event.Away.Name) && usableSportsIdentityName(event.Home.Name) {
		// A single-league composite cannot resolve visiting clubs from another
		// league. Keep their individually resolved logos on the local backdrop.
		if gameThumbsLeagueSlugForTeam(event.Away, slug) != slug || gameThumbsLeagueSlugForTeam(event.Home, slug) != slug {
			event.GameThumbsBackgroundURL = ""
			return event
		}
		path += "/" + url.PathEscape(gameThumbsTeamKey(gameThumbsCanonicalTeamName(event.Away.Name))) + "/" + url.PathEscape(gameThumbsTeamKey(gameThumbsCanonicalTeamName(event.Home.Name)))
	}
	event.GameThumbsBackgroundURL = gameThumbsPublicBaseURL + path + "/thumb.png?style=6&logo=false&fallback=true"
	return event
}
