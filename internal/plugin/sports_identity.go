package plugin

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const gameThumbsPublicBaseURL = "https://game-thumbs.swvn.io"
const sportsLogosRawBaseURL = "https://raw.githubusercontent.com/alexanderthebadatcoding/Sports-Logos/main"

const (
	aflLeagueLogoURL        = "https://r2.thesportsdb.com/images/media/league/badge/wvx4721525519372.png"
	aflTeamLogoBase         = "https://squiggle.com.au/wp-content/themes/squiggle/assets/images/"
	formulaELeagueLogoURL   = "https://upload.wikimedia.org/wikipedia/commons/8/8c/Formula-e-logo-championship_2023.svg"
	formulaOneLeagueLogoURL = "https://upload.wikimedia.org/wikipedia/commons/2/2d/Formula_One_logo.svg"
	ncaaTeamLogoBase        = "https://a.espncdn.com/i/teamlogos/ncaa/500/"
	ncaaLeagueLogoURL       = "https://upload.wikimedia.org/wikipedia/commons/d/dd/NCAA_logo.svg"
	ceblLeagueLogoURL       = "https://irp.cdn-website.com/d8d53c44/dms3rep/multi/CEBL_Primary-Logo_Full-Wordmark.svg"
	iccLeagueLogoURL        = "https://images.icc-cricket.com/image/private/t_q-best/v1763015137/prd/assets/app-nav-dropdown/default-icc-logo.png"
	fibaWomensLeagueLogoURL = "https://assets.fiba.basketball/image/upload/w_400,h_128,c_fit/q_auto/f_auto/v1723639691/.asset_mainlogo--competition_208875"
)

type sportsIdentityRoute struct {
	pattern *regexp.Regexp
	slug    string
}

var gameThumbsLeagueRoutes = []sportsIdentityRoute{
	{regexp.MustCompile(`(?i)\bnascar\s+cup\s+series\b|\bncs\s+race\b`), "NASCAR"},
	{regexp.MustCompile(`(?i)\bindian premier league\b|\bipl\b`), "ipl"},
	{regexp.MustCompile(`(?i)\bbangladesh premier league\b|\bbpl\b`), "bpl"},
	{regexp.MustCompile(`(?i)\bcaribbean premier league\b|\bcpl\b`), "cpl"},
	{regexp.MustCompile(`(?i)\bmajor league cricket\b|\bmlc\b`), "mlc"},
	{regexp.MustCompile(`(?i)\befl championship\b|\benglish championship\b|\beng\.2\b`), "championship"},
	{regexp.MustCompile(`(?i)\befl league (?:one|1)\b|\benglish league (?:one|1)\b|\beng\.3\b`), "league-one"},
	{regexp.MustCompile(`(?i)\befl league (?:two|2)\b|\benglish league (?:two|2)\b|\beng\.4\b`), "league-two"},
	{regexp.MustCompile(`(?i)\benglish premier league\b|\bpremier league\b|\bepl\b`), "epl"},
	{regexp.MustCompile(`(?i)\bnational women'?s soccer league\b|\bnwsl\b`), "usa.nwsl"},
	{regexp.MustCompile(`(?i)\bmajor league soccer\b|\bmls\b`), "mls"},
	{regexp.MustCompile(`(?i)\buefa champions league\b|\bchampions league\b`), "uefa"},
	{regexp.MustCompile(`(?i)\bla ?liga\b`), "laliga"},
	{regexp.MustCompile(`(?i)\bbundesliga\b`), "bundesliga"},
	{regexp.MustCompile(`(?i)\bserie a\b`), "seriea"},
	{regexp.MustCompile(`(?i)\bligue 1\b`), "ligue1"},
	{regexp.MustCompile(`(?i)\bbrazil(?:ian)? (?:serie a|série a)\b|\bbra\.1\b`), "bra.1"},
	{regexp.MustCompile(`(?i)\bcollege football\b|\bncaa football\b|\bncaaf\b`), "ncaaf"},
	{regexp.MustCompile(`(?i)\bnational football league\b|\bnfl\b`), "nfl"},
	{regexp.MustCompile(`(?i)\bnational hockey league\b|\bnhl\b`), "nhl"},
	{regexp.MustCompile(`(?i)\bwomen'?s national basketball association\b|\bwnba\b`), "wnba"},
	{regexp.MustCompile(`(?i)\bnational basketball association\b|\bnba\b`), "nba"},
	{regexp.MustCompile(`(?i)\bmajor league baseball\b|\bmlb\b`), "mlb"},
	{regexp.MustCompile(`(?i)\bultimate fighting championship\b|\bufc\b`), "ufc"},
	{regexp.MustCompile(`(?i)\bprofessional fighters league\b|\bpfl\b`), "pfl"},
	{regexp.MustCompile(`(?i)\bbellator\b`), "bellator"},
	{regexp.MustCompile(`(?i)\bboxing\b`), "boxing"},
}

var sportsCountryNames = map[string]string{
	"afghanistan":          "Afghanistan",
	"argentina":            "Argentina",
	"australia":            "Australia",
	"bangladesh":           "Bangladesh",
	"canada":               "Canada",
	"cuba":                 "Cuba",
	"dominican republic":   "Dominican Republic",
	"england":              "England",
	"india":                "India",
	"ireland":              "Ireland",
	"namibia":              "Namibia",
	"nepal":                "Nepal",
	"netherlands":          "Netherlands",
	"new zealand":          "New Zealand",
	"pakistan":             "Pakistan",
	"scotland":             "Scotland",
	"south africa":         "South Africa",
	"sri lanka":            "Sri Lanka",
	"united arab emirates": "United Arab Emirates",
	"united states":        "United States",
	"usa":                  "USA",
	"zimbabwe":             "Zimbabwe",
}

var ncaaTeamLogoIDs = map[string]string{
	"michigan":                   "130",
	"michigan wolverines":        "130",
	"western michigan":           "2711",
	"western michigan broncos":   "2711",
	"central michigan":           "2117",
	"central michigan chippewas": "2117",
	"florida":                    "57",
	"florida gators":             "57",
	"florida state":              "52",
	"florida state seminoles":    "52",
	"fsu":                        "52",
	"indiana":                    "84",
	"indiana hoosiers":           "84",
	"oregon":                     "2483",
	"oregon ducks":               "2483",
}

var aflTeamLogoFiles = map[string]string{
	"adelaide":               "Adelaide.png",
	"brisbane":               "Brisbane.png",
	"brisbane lions":         "Brisbane.png",
	"carlton":                "Carlton.png",
	"collingwood":            "Collingwood.png",
	"essendon":               "Essendon.png",
	"fremantle":              "Fremantle.png",
	"geelong":                "Geelong.png",
	"geelong cats":           "Geelong.png",
	"gold coast":             "GoldCoast.png",
	"gold coast suns":        "GoldCoast.png",
	"greater western sydney": "Giants.png",
	"gws":                    "Giants.png",
	"gws giants":             "Giants.png",
	"hawthorn":               "Hawthorn.png",
	"melbourne":              "Melbourne.png",
	"north melbourne":        "NorthMelbourne.png",
	"port adelaide":          "PortAdelaide.png",
	"richmond":               "Richmond.png",
	"st kilda":               "StKilda.png",
	"sydney":                 "Sydney.png",
	"sydney swans":           "Sydney.png",
	"west coast":             "WestCoast.png",
	"west coast eagles":      "WestCoast.png",
	"western bulldogs":       "Bulldogs.png",
}

var referencedSportsTeamLogoPaths = map[string]string{
	"austin fc": "/MLS/ATX.png",
	"toluca":    "/MLS/TOL.png",
}

var gameThumbsTeamLeagueRoutes = []sportsIdentityRoute{
	{regexp.MustCompile(`(?i)\b(?:atlanta hawks|boston celtics|brooklyn nets|charlotte hornets|chicago bulls|cleveland cavaliers|dallas mavericks|denver nuggets|detroit pistons|golden state warriors|houston rockets|indiana pacers|(?:la|los angeles) clippers|(?:la|los angeles) lakers|memphis grizzlies|miami heat|milwaukee bucks|minnesota timberwolves|new orleans pelicans|new york knicks|oklahoma city thunder|orlando magic|philadelphia 76ers|phoenix suns|portland trail blazers|sacramento kings|san antonio spurs|toronto raptors|utah jazz|washington wizards)\b`), "nba"},
	{regexp.MustCompile(`(?i)\b(?:anaheim ducks|boston bruins|buffalo sabres|calgary flames|carolina hurricanes|chicago blackhawks|colorado avalanche|columbus blue jackets|dallas stars|detroit red wings|edmonton oilers|florida panthers|los angeles kings|minnesota wild|montreal canadiens|nashville predators|new jersey devils|new york islanders|new york rangers|ottawa senators|philadelphia flyers|pittsburgh penguins|san jose sharks|seattle kraken|st louis blues|st\. louis blues|tampa bay lightning|toronto maple leafs|utah mammoth|utah hockey club|vancouver canucks|vegas golden knights|washington capitals|winnipeg jets)\b`), "nhl"},
	// Game Thumbs' epl namespace resolves the English pyramid through its configured feeder leagues.
	{regexp.MustCompile(`(?i)\b(?:chelsea(?:\s+(?:u21|under[ -]?21s?))?|bristol rovers)\b`), "epl"},
	{regexp.MustCompile(`(?i)\b(?:palmeiras|flamengo|fluminense|corinthians|santos|botafogo|vasco da gama|sao paulo|são paulo|gremio|grêmio|internacional|cruzeiro|atletico mineiro|atlético mineiro)\b`), "bra.1"},
}

var gameThumbsChelseaYouthSuffix = regexp.MustCompile(`(?i)\bchelsea\s+(?:u21|under[ -]?21s?)\b`)
var collegeSportsIdentity = regexp.MustCompile(`(?i)\b(?:cfp|ncaa|ncaaf|college[- ]+(?:football|soccer|basketball|baseball|volleyball|softball|field[- ]+hockey|hockey))\b`)

func applySportsIdentityFallbacks(event SportsEvent) SportsEvent {
	event = applySpecialSportsIdentityFallbacks(event)
	leagueSlug := gameThumbsLeagueSlugForEvent(event)
	if leagueSlug == "" {
		awayLeagueSlug := gameThumbsLeagueSlugForTeam(event.Away, "")
		homeLeagueSlug := gameThumbsLeagueSlugForTeam(event.Home, "")
		if awayLeagueSlug != "" && awayLeagueSlug == homeLeagueSlug {
			leagueSlug = awayLeagueSlug
		}
	}
	if event.LeagueLogoURL == "" && leagueSlug != "" {
		event.LeagueLogoURL = gameThumbsLeagueLogoURL(leagueSlug)
	}
	event.Away = applySportsTeamIdentityFallback(event.Away, leagueSlug)
	event.Home = applySportsTeamIdentityFallback(event.Home, leagueSlug)
	return applyGameThumbsArtwork(event, leagueSlug)
}

func applySpecialSportsIdentityFallbacks(event SportsEvent) SportsEvent {
	if event.LeagueLogoURL == "" {
		switch event.LeagueID {
		case "cebl":
			event.LeagueLogoURL = ceblLeagueLogoURL
		case "fiba-womens-world-cup":
			event.LeagueLogoURL = fibaWomensLeagueLogoURL
		case "cricket":
			// International series have no franchise-league badge. Use the
			// governing body's mark without borrowing a domestic competition.
			if gameThumbsLeagueSlugForEvent(event) == "" && sportsCountryNames[normalizeSportsIdentityText(event.Away.Name)] != "" && sportsCountryNames[normalizeSportsIdentityText(event.Home.Name)] != "" {
				event.LeagueLogoURL = iccLeagueLogoURL
			}
		}
	}
	identityText := normalizeSportsIdentityText(strings.Join([]string{event.LeagueID, event.LeagueName, event.SportName, event.Name}, " "))
	for _, team := range []*SportsTeam{&event.Away, &event.Home} {
		if team.LogoURL != "" {
			continue
		}
		name := normalizeSportsIdentityText(team.Name)
		if event.LeagueID == "mlb" {
			if code := map[string]string{"dodgers": "lad", "los angeles dodgers": "lad", "cubs": "chc", "chicago cubs": "chc"}[name]; code != "" {
				team.LogoURL = "https://a.espncdn.com/i/teamlogos/mlb/500/" + code + ".png"
			}
		}
		if event.LeagueID == "cebl" {
			team.LogoURL = ceblTeamLogo(name)
		}
	}
	if leagueID, _, sport := guideCollegeCompetition(identityText); leagueID != "" && sport != "Football" {
		if event.LeagueLogoURL == "" {
			event.LeagueLogoURL = ncaaLeagueLogoURL
		}
		// Football keeps its existing IDs so saved passes remain valid. Other
		// college teams belong to a competition, not just a school.
		for _, team := range []*SportsTeam{&event.Home, &event.Away} {
			if team.Name != "" {
				team.ID = "college-team:" + sportsHash(leagueID+"|"+normalizeMatchText(team.Name))
			}
		}
	}
	if event.LeagueLogoURL == "" && formulaERacePattern.MatchString(identityText) {
		event.LeagueLogoURL = formulaELeagueLogoURL
	}
	if leagueID, _, _, _ := guideSportsLeague(identityText); event.LeagueLogoURL == "" && leagueID == "formula-1" {
		event.LeagueLogoURL = formulaOneLeagueLogoURL
	}
	if collegeSportsIdentity.MatchString(identityText) {
		event.Away = applyNCAATeamIdentity(event.Away)
		event.Home = applyNCAATeamIdentity(event.Home)
	}
	event.Away = applyCountryTeamIdentity(event.Away)
	event.Home = applyCountryTeamIdentity(event.Home)
	event.Away = applyReferencedSportsTeamIdentity(event.Away)
	event.Home = applyReferencedSportsTeamIdentity(event.Home)
	if strings.Contains(identityText, "afl") || strings.Contains(identityText, "australian football") || strings.Contains(identityText, "afl premiership") {
		if event.LeagueLogoURL == "" {
			event.LeagueLogoURL = aflLeagueLogoURL
		}
		event.Away = applyAFLTeamIdentity(event.Away)
		event.Home = applyAFLTeamIdentity(event.Home)
	}
	return event
}

func applyReferencedSportsTeamIdentity(team SportsTeam) SportsTeam {
	if team.LogoURL != "" {
		return team
	}
	if logoPath := referencedSportsTeamLogoPaths[normalizeSportsIdentityText(team.Name)]; logoPath != "" {
		team.LogoURL = sportsLogosRawBaseURL + logoPath
	}
	return team
}

func applyNCAATeamIdentity(team SportsTeam) SportsTeam {
	if team.LogoURL != "" && !strings.HasPrefix(team.LogoURL, gameThumbsPublicBaseURL+"/") {
		return team
	}
	if teamID := ncaaTeamLogoIDs[normalizeSportsIdentityText(team.Name)]; teamID != "" {
		team.LogoURL = ncaaTeamLogoBase + teamID + ".png"
	}
	return team
}

func applyAFLTeamIdentity(team SportsTeam) SportsTeam {
	if team.LogoURL != "" {
		return team
	}
	if filename := aflTeamLogoFiles[normalizeSportsIdentityText(team.Name)]; filename != "" {
		team.LogoURL = aflTeamLogoBase + filename
	}
	return team
}

func applyCountryTeamIdentity(team SportsTeam) SportsTeam {
	if team.LogoURL != "" {
		return team
	}
	if code := map[string]string{"argentina": "ar", "cuba": "cu"}[normalizeSportsIdentityText(team.Name)]; code != "" {
		team.LogoURL = "https://flagcdn.com/w160/" + code + ".png"
		return team
	}
	if country := sportsCountryNames[normalizeSportsIdentityText(team.Name)]; country != "" {
		team.LogoURL = gameThumbsTeamLogoURL("country", country)
	}
	return team
}

func normalizeSportsIdentityText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func applySportsTeamIdentityFallback(team SportsTeam, eventLeagueSlug string) SportsTeam {
	if team.LogoURL != "" || !usableSportsIdentityName(team.Name) {
		return team
	}
	if eventLeagueSlug == "boxing" {
		team.LogoURL = gameThumbsLeagueLogoURL(eventLeagueSlug)
		return team
	}
	leagueSlug := gameThumbsLeagueSlugForTeam(team, eventLeagueSlug)
	if leagueSlug != "" {
		team.LogoURL = gameThumbsTeamLogoURL(leagueSlug, team.Name)
	}
	return team
}

func gameThumbsLeagueSlugForEvent(event SportsEvent) string {
	if event.LeagueID == "lanka-premier-league" {
		// This competition has its own official assets, and must not match EPL.
		return ""
	}
	if slug := gameThumbsKnownLeague(event); slug != "" {
		return slug
	}
	value := strings.Join([]string{event.LeagueID, event.LeagueName, event.SportName, event.Name, event.ShortName}, " ")
	return firstSportsIdentityRoute(value, gameThumbsLeagueRoutes)
}

func gameThumbsLeagueSlugForTeam(team SportsTeam, eventLeagueSlug string) string {
	if slug := firstSportsIdentityRoute(strings.Join([]string{team.ID, team.Name, team.Abbreviation}, " "), gameThumbsTeamLeagueRoutes); slug != "" {
		return slug
	}
	return eventLeagueSlug
}

func firstSportsIdentityRoute(value string, routes []sportsIdentityRoute) string {
	for _, route := range routes {
		if route.pattern.MatchString(value) {
			return route.slug
		}
	}
	return ""
}

func usableSportsIdentityName(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value != "" && value != "team" && value != "tbd" && value != "unknown"
}

func gameThumbsLeagueLogoURL(leagueSlug string) string {
	if leagueSlug == "" {
		return ""
	}
	return gameThumbsPublicBaseURL + "/" + url.PathEscape(leagueSlug) + "/logo.png"
}

func gameThumbsTeamLogoURL(leagueSlug, teamName string) string {
	teamKey := gameThumbsTeamKey(gameThumbsCanonicalTeamName(teamName))
	if leagueSlug == "" || teamKey == "" {
		return ""
	}
	return gameThumbsPublicBaseURL + "/" + url.PathEscape(leagueSlug) + "/" + url.PathEscape(teamKey) + "/logo.png?variant=dark"
}

func gameThumbsCanonicalTeamName(value string) string {
	value = strings.TrimSpace(value)
	if gameThumbsChelseaYouthSuffix.MatchString(value) {
		return "Chelsea"
	}
	return value
}

func gameThumbsTeamKey(value string) string {
	var result strings.Builder
	separator := false
	for _, character := range norm.NFD.String(strings.ToLower(strings.TrimSpace(value))) {
		if unicode.Is(unicode.Mn, character) {
			continue
		}
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			result.WriteRune(character)
			separator = false
			continue
		}
		if result.Len() > 0 && !separator {
			result.WriteByte('-')
			separator = true
		}
	}
	return strings.Trim(result.String(), "-")
}
