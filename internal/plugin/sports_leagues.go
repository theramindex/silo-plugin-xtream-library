package plugin

import (
	"regexp"
	"strings"
)

// sportsLeagueDefinition is the one place a guide competition's identity
// lives. Guide recognition walks sportsLeagueRegistry in order.
type sportsLeagueDefinition struct {
	ID    string
	Name  string
	Sport string
	// Terms are normalized guide phrases that identify the competition. Entries
	// without terms are reached through dedicated parsers (tennis and golf
	// majors).
	Terms []string
	// Tournament competitions are listed one-sided ("2026 US Open Tennis
	// Championships", "Formula 1: Italian Grand Prix"), so a guide listing that
	// names them is an event even without an "A vs B" title.
	Tournament bool
}

// Guide recognition runs in this order; earlier entries win.
var sportsLeagueRegistry = []sportsLeagueDefinition{
	{ID: "wnba", Name: "WNBA", Sport: "Basketball", Terms: []string{"wnba"}},
	{ID: "nba", Name: "NBA", Sport: "Basketball", Terms: []string{"nba"}},
	{ID: "nfl", Name: "NFL", Sport: "Football", Terms: []string{"nfl"}},
	{ID: "college-football", Name: "College Football", Sport: "Football", Terms: []string{"cfp", "college football"}},
	{ID: "mlb", Name: "MLB", Sport: "Baseball", Terms: []string{"mlb", "cubs classics"}},
	{ID: "cebl", Name: "CEBL", Sport: "Basketball", Terms: []string{"cebl", "canadian elite basketball league"}},
	{ID: "nhl", Name: "NHL", Sport: "Hockey", Terms: []string{"nhl"}},
	{ID: "mls", Name: "MLS", Sport: "Soccer", Terms: []string{"mls"}},
	{ID: "uefa-champions-league", Name: "UEFA Champions League", Sport: "Soccer", Terms: []string{"uefa champions league", "champions league"}},
	{ID: "premier-league", Name: "Premier League", Sport: "Soccer", Terms: []string{"premier league"}},
	{ID: "world-cup", Name: "World Cup", Sport: "Soccer", Terms: []string{"world cup", "fifa"}},
	{ID: "mma", Name: "MMA", Sport: "Combat Sports", Terms: []string{"ufc", "mma"}},
	{ID: "boxing", Name: "Boxing", Sport: "Combat Sports", Terms: []string{"boxing", "boxeo"}},
	{ID: "formula-e", Name: "Formula E", Sport: "Motorsport", Terms: []string{"formula e", "formule e"}, Tournament: true},
	{ID: "formula-1", Name: "Formula 1", Sport: "Motorsport", Terms: []string{"formula 1", "f1"}, Tournament: true},
	{ID: "nascar-cup-series", Name: "NASCAR Cup Series", Sport: "Motorsport", Terms: []string{"nascar cup series", "ncs race"}, Tournament: true},
	{ID: "indycar", Name: "IndyCar", Sport: "Motorsport", Terms: []string{"indycar", "indy 500", "indianapolis 500"}, Tournament: true},
	{ID: "motogp", Name: "MotoGP", Sport: "Motorsport", Terms: []string{"motogp"}, Tournament: true},
	{ID: "afl", Name: "AFL", Sport: "Australian Football", Terms: []string{"afl premiership", "australian football league"}},
	// Majors are recognized by guideSportsMajorTournament, which separates
	// the tennis and golf events that share a name.
	{ID: "us-open-tennis", Name: "US Open", Sport: "Tennis", Tournament: true},
	{ID: "wimbledon", Name: "Wimbledon", Sport: "Tennis", Tournament: true},
	{ID: "roland-garros", Name: "Roland Garros", Sport: "Tennis", Tournament: true},
	{ID: "australian-open", Name: "Australian Open", Sport: "Tennis", Tournament: true},
	{ID: "masters-tournament", Name: "The Masters", Sport: "Golf", Tournament: true},
	{ID: "pga-championship", Name: "PGA Championship", Sport: "Golf", Tournament: true},
	{ID: "us-open-golf", Name: "U.S. Open", Sport: "Golf", Tournament: true},
	{ID: "the-open", Name: "The Open", Sport: "Golf", Tournament: true},
	{ID: "tennis", Name: "Tennis", Sport: "Tennis", Terms: []string{"tennis"}},
	{ID: "golf", Name: "Golf", Sport: "Golf", Terms: []string{"golf", "pga"}},
	{ID: "cricket", Name: "Cricket", Sport: "Cricket", Terms: []string{"cricket"}},
}

var sportsLeaguesByID = func() map[string]sportsLeagueDefinition {
	byID := make(map[string]sportsLeagueDefinition, len(sportsLeagueRegistry))
	for _, league := range sportsLeagueRegistry {
		if _, duplicate := byID[league.ID]; duplicate {
			panic("duplicate sports league registry ID: " + league.ID)
		}
		byID[league.ID] = league
	}
	return byID
}()

func sportsLeagueByID(id string) (sportsLeagueDefinition, bool) {
	league, ok := sportsLeaguesByID[strings.ToLower(strings.TrimSpace(id))]
	return league, ok
}

// sportsLeagueMatch is a guide classification. Tournament is set only when a
// specific registry competition matched, never for the generic fallback.
type sportsLeagueMatch struct {
	ID, Name, Sport string
	OK              bool
	Tournament      bool
}

func (league sportsLeagueDefinition) match() sportsLeagueMatch {
	return sportsLeagueMatch{ID: league.ID, Name: league.Name, Sport: league.Sport, OK: true, Tournament: league.Tournament}
}

func registrySportsLeague(id string) sportsLeagueMatch {
	league, ok := sportsLeagueByID(id)
	if !ok {
		return sportsLeagueMatch{}
	}
	return league.match()
}

// Words that mark a non-tennis "Open" (the U.S. Open Cup is soccer).
var sportsNonTennisOpenTerms = []string{"cup", "squash", "badminton", "surfing", "bowling", "pool", "snooker", "darts", "racquetball", "table tennis", "polo", "chess", "volleyball"}

// guideSportsMajorTournament recognizes tennis and golf majors. "US Open"
// means tennis unless the listing, channel or category says golf.
func guideSportsMajorTournament(text string) (sportsLeagueMatch, bool) {
	has := func(terms ...string) bool {
		for _, term := range terms {
			if containsMatchTerm(text, term) {
				return true
			}
		}
		return false
	}
	golf := has("golf", "usga", "pga", "lpga", "augusta")
	tennis := has("tennis", "atp", "wta")
	otherOpen := has(sportsNonTennisOpenTerms...)
	tennisMajor := func(id string) (sportsLeagueMatch, bool) {
		if otherOpen || (golf && !tennis) {
			return sportsLeagueMatch{}, false
		}
		return registrySportsLeague(id), true
	}
	switch {
	case has("us open", "u s open"):
		if golf && !tennis {
			return registrySportsLeague("us-open-golf"), true
		}
		return tennisMajor("us-open-tennis")
	case has("wimbledon"):
		return tennisMajor("wimbledon")
	case has("roland garros", "french open"):
		return tennisMajor("roland-garros")
	case has("australian open"):
		return tennisMajor("australian-open")
	case has("masters tournament", "augusta national") || (golf && has("the masters")):
		return registrySportsLeague("masters-tournament"), true
	case has("pga championship"):
		return registrySportsLeague("pga-championship"), true
	case has("open championship", "british open") || (golf && has("the open")):
		return registrySportsLeague("the-open"), true
	}
	return sportsLeagueMatch{}, false
}

// Studio and magazine shows about a tournament are not the event itself.
var guideSportsTournamentShowPattern = regexp.MustCompile(`(?i)\b(?:preview|review|recap|magazine|countdown|show|notebook|press\s+conference|tonight|today|daily|classics?|documentary|weekly|insider|uncut|analysis|studio|debrief|reaction|draw|highlights?|talk|podcast|central|report|pre[\s-]?game|post[\s-]?game)\b`)

func guideSportsTournamentListing(title string) bool {
	return !guideSportsTournamentShowPattern.MatchString(title) && !guideSportsNonMatchTitle.MatchString(title)
}

// Known tournament names that precede the first competitor without a colon
// ("US Open Sinner vs Alcaraz").
var guideSportsKnownTournamentPrefix = regexp.MustCompile(`(?i)^(?:(?:19|20)\d{2}\s+)?(?:the\s+)?(?:u\.?\s?s\.?\s+open|wimbledon|roland[\s-]+garros|french\s+open|australian\s+open|masters\s+tournament|pga\s+championship|open\s+championship|british\s+open)(?:\s+(?:tennis|golf))?(?:\s+championships?)?(?:\s+(?:19|20)\d{2})?\s+`)

var collegeCompetitionPattern = regexp.MustCompile(`\b(?:(womens|mens|women|men) )?(?:college|ncaa) (?:(womens|mens|women|men) )?(football|basketball|soccer|baseball|softball|volleyball|field hockey|hockey|lacrosse|gymnastics|wrestling)\b`)

func guideCollegeCompetition(value string) (string, string, string) {
	text := strings.NewReplacer("women s", "womens", "men s", "mens").Replace(normalizeMatchText(value))
	var chosen []string
	for _, match := range collegeCompetitionPattern.FindAllStringSubmatch(text, -1) {
		if chosen == nil || match[1] != "" || match[2] != "" {
			chosen = match
		}
		if match[1] != "" || match[2] != "" {
			break
		}
	}
	if chosen == nil {
		return "", "", ""
	}
	sport := chosen[3]
	sportName := strings.ToUpper(sport[:1]) + sport[1:]
	if sport == "field hockey" {
		sport = "field-hockey"
		sportName = "Field Hockey"
	}
	gender := firstNonEmpty(chosen[1], chosen[2])
	if gender != "" {
		if strings.HasPrefix(gender, "women") {
			return "college-womens-" + sport, "Women's College " + sportName, sportName
		}
		return "college-mens-" + sport, "Men's College " + sportName, sportName
	}
	return "college-" + sport, "College " + sportName, sportName
}

var under20CompetitionPattern = regexp.MustCompile(`\b(?:u|under|sub) ?20\b`)

func guideWorldCupCompetition(value string) (string, string, string) {
	text := normalizeMatchText(value)
	if !containsMatchTerm(text, "world cup") && !containsMatchTerm(text, "copa mundial") {
		return "", "", ""
	}
	womens := false
	for _, term := range []string{"women", "womens", "femenina", "femenino"} {
		womens = womens || containsMatchTerm(text, term)
	}
	if containsMatchTerm(text, "fiba") || containsMatchTerm(text, "basketball") {
		if womens {
			return "fiba-womens-world-cup", "FIBA Women's Basketball World Cup", "Basketball"
		}
		return "fiba-world-cup", "FIBA Basketball World Cup", "Basketball"
	}
	if containsMatchTerm(text, "fifa") || containsMatchTerm(text, "soccer") {
		if under20CompetitionPattern.MatchString(text) {
			if womens {
				return "fifa-womens-u20-world-cup", "FIFA Women's U-20 World Cup", "Soccer"
			}
			return "fifa-u20-world-cup", "FIFA U-20 World Cup", "Soccer"
		}
		if womens {
			return "fifa-womens-world-cup", "FIFA Women's World Cup", "Soccer"
		}
	}
	return "", "", ""
}

func guideSportsLeague(value string) (string, string, string, bool) {
	match := guideSportsLeagueMatch(value)
	return match.ID, match.Name, match.Sport, match.OK
}

func guideSportsLeagueMatch(value string) sportsLeagueMatch {
	competitionText := normalizeMatchText(value)
	if containsMatchTerm(competitionText, "lanka premier league") || (containsMatchTerm(competitionText, "lpl") && containsMatchTerm(competitionText, "cricket")) {
		return sportsLeagueMatch{ID: "lanka-premier-league", Name: "Lanka Premier League", Sport: "Cricket", OK: true}
	}
	if containsMatchTerm(competitionText, "hoopqueens") || containsMatchTerm(competitionText, "hoop queens") {
		return sportsLeagueMatch{ID: "hoopqueens", Name: "HoopQueens Basketball", Sport: "Basketball", OK: true}
	}
	if containsMatchTerm(competitionText, "canada cup") && containsMatchTerm(competitionText, "softball") {
		return sportsLeagueMatch{ID: "canada-cup-softball", Name: "Canada Cup Softball", Sport: "Softball", OK: true}
	}
	if text := gameThumbsMatchText(value); containsMatchTerm(text, "volleyball nations league") {
		if containsMatchTerm(text, "womens") || containsMatchTerm(text, "women") {
			return sportsLeagueMatch{ID: "womens-volleyball-nations-league", Name: "Women's Volleyball Nations League", Sport: "Volleyball", OK: true}
		}
		if containsMatchTerm(text, "mens") || containsMatchTerm(text, "men") {
			return sportsLeagueMatch{ID: "mens-volleyball-nations-league", Name: "Men's Volleyball Nations League", Sport: "Volleyball", OK: true}
		}
		return sportsLeagueMatch{ID: "volleyball-nations-league", Name: "Volleyball Nations League", Sport: "Volleyball", OK: true}
	}
	if id, name, sport := guideWorldCupCompetition(value); id != "" {
		return sportsLeagueMatch{ID: id, Name: name, Sport: sport, OK: true}
	}
	if id, name, sport := guideCollegeCompetition(value); id != "" {
		return sportsLeagueMatch{ID: id, Name: name, Sport: sport, OK: true}
	}
	text := competitionText
	major, majorOK := guideSportsMajorTournament(text)
	for _, league := range sportsLeagueRegistry {
		// Majors outrank the generic tennis and golf entries that follow them.
		if majorOK && (league.ID == "tennis" || league.ID == "golf") {
			return major
		}
		for _, term := range league.Terms {
			if containsMatchTerm(text, term) {
				return league.match()
			}
		}
	}
	if majorOK {
		return major
	}
	if containsMatchTerm(text, "sports") || containsMatchTerm(text, "soccer") || containsMatchTerm(text, "football") || containsMatchTerm(text, "basketball") || containsMatchTerm(text, "baseball") || containsMatchTerm(text, "hockey") {
		return sportsLeagueMatch{ID: "sports", Name: "Sports", Sport: "Sports", OK: true}
	}
	return sportsLeagueMatch{}
}
