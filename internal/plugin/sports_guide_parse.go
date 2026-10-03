package plugin

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

var sportsMatchupSeparator = regexp.MustCompile(`(?i)\s+(?:vs\.?|v\.?|at|@)\s+`)
var formulaERacePattern = regexp.MustCompile(`(?i)\bformul[ae]\s+e\b`)
var nascarCupRacePattern = regexp.MustCompile(`(?i)\b(?:nascar\s+cup\s+series|ncs\s+race)\b`)
var raceLocationPrefix = regexp.MustCompile(`(?i)^\s*(?:v(?:s\.)?|at|@|:|-)\s*`)
var guideSportsTimestampSuffix = regexp.MustCompile(`(?i)\s*\(\d{4}-\d{2}-\d{2}(?:[ t]\d{1,2}:\d{2}(?::\d{2})?)?\)\s*$`)
var guideSportsVenueSuffix = regexp.MustCompile(`\s+_\s+([^_]+?)\s*$`)
var guideSportsNextGameSuffix = regexp.MustCompile(`(?i)\s+on\s+\d{4}-\d{2}-\d{2}\s+at\s+\d{1,2}:\d{2}\s*(?:am|pm)?(?:\s+[a-z]{2,5})?\s*$`)
var sportsISODatePattern = regexp.MustCompile(`\b(20\d{2})[-_/](\d{1,2})[-_/](\d{1,2})\b`)
var sportsUSDatePattern = regexp.MustCompile(`\b(\d{1,2})[-_/](\d{1,2})[-_/](20\d{2})\b`)
var guideSportsMatchNumberSuffix = regexp.MustCompile(`(?i)\s*(?:,\s*match\s+\d+|[-,]?\s*\d+(?:st|nd|rd|th)\s+match)\s*$`)
var guideSportsStageSuffix = regexp.MustCompile(`(?i)\s+[-–—]\s+(?:qualifier|eliminator|semi[- ]?final|final)(?:\s+\d+)?\s*$`)
var guideSportsCompetitionSuffix = regexp.MustCompile(`(?i)\s+-\s+(?:uefa\s+(?:champions|europa|conference)\s+league)\b.*$`)
var guideSportsClockName = regexp.MustCompile(`(?i)^\d{1,2}(?::\d{2})?\s*(?:am|pm)(?:\s+[a-z]{2,5})?$`)
var guideSportsNonMatchTitle = regexp.MustCompile(`(?i)\b(?:good morning|outdoor magazine|the verdict|the case for)\b`)

func guideSportsTitleHasScoreLookupHint(title string) bool {
	return guideSportsTitleIsLive(title) || strings.Contains(title, "ᴺᵉʷ")
}

func guideSportsTitleIsLive(title string) bool {
	return strings.Contains(title, "ᴸᶦᵛᵉ") || containsMatchTerm(normalizeMatchText(title), "live")
}

func sportsEventsFromGuide(snapshot cache.Snapshot, now time.Time) []SportsEvent {
	events, _ := sportsEventsFromGuideWithScoreHints(snapshot, now)
	return events
}

func sportsEventsFromGuideWithScoreHints(snapshot cache.Snapshot, now time.Time) ([]SportsEvent, bool) {
	categoryNames := map[string]string{}
	for _, category := range liveCategories(snapshot) {
		categoryNames[category.ID] = category.Name
	}
	channels := map[string]model.Channel{}
	for _, channel := range snapshot.Catalog.Channels {
		if channel.ID != "" {
			channels[channel.ID] = channel
		}
	}

	fromUnix := now.Add(-24 * time.Hour).Unix()
	toUnix := now.Add(72 * time.Hour).Unix()
	byKey := map[string]*SportsEvent{}
	refreshScores := false
	for _, program := range snapshot.Catalog.Programs {
		if program.EndUnix < fromUnix || program.StartUnix > toUnix {
			continue
		}
		channel, ok := channels[program.ChannelID]
		if !ok {
			continue
		}
		categoryName := firstNonEmpty(categoryNames[channel.CategoryID], channel.CategoryName)
		displayTitle := cleanGuideSportsAnnotations(program.Title)
		metadataSports, excludeProgram := guideSportsMetadata(program.Categories)
		if excludeProgram {
			continue
		}
		league := guideSportsLeagueMatch(strings.Join([]string{displayTitle, strings.Join(program.Categories, " "), channel.Name, categoryName}, " "))
		leagueID, leagueName, sportName := league.ID, league.Name, league.Sport
		sportsContext := league.OK || metadataSports
		awayName, homeName, matchup := guideSportsMatchup(displayTitle)
		eventType := ""
		if series, location, race := guideSportsRace(displayTitle); race {
			awayName, homeName, matchup = series, location, true
			eventType = "race"
		}
		boutCard := leagueID == "boxing" && guideSportsMultipleBouts(displayTitle)
		// Tournaments and races are listed one-sided; a specific registry
		// competition admits them, the generic "Sports" fallback does not.
		tournament := !matchup && league.Tournament && guideSportsTournamentListing(displayTitle)
		if !sportsContext || (!matchup && !metadataSports && !boutCard && !tournament) {
			continue
		}
		if !matchup {
			eventType = "event"
		}
		if program.StartUnix <= now.Unix() && program.EndUnix > now.Unix() && guideSportsTitleHasScoreLookupHint(program.Title) {
			refreshScores = true
		}

		startBucket := program.StartUnix / (15 * 60)
		key := normalizeMatchText(displayTitle) + "|" + fmt.Sprintf("%d", startBucket)
		event := byKey[key]
		if event == nil {
			endUnix := program.EndUnix
			if endUnix <= program.StartUnix {
				endUnix = program.StartUnix + 3*3600
			}
			live, completed, status, statusText := guideSportsBroadcastStatus(program, endUnix, now)
			shortName := strings.TrimSpace(awayName + " vs " + homeName)
			if eventType == "event" {
				shortName = displayTitle
			}
			if eventType == "race" {
				shortName = strings.Trim(strings.Join([]string{awayName, homeName}, " · "), " ·")
			}
			value := SportsEvent{
				ID:         "epg:" + sportsHash(key),
				LeagueID:   leagueID,
				LeagueName: leagueName,
				SportName:  sportName,
				Name:       displayTitle,
				ShortName:  shortName,
				EventType:  eventType,
				Venue:      guideSportsVenue(displayTitle),
				StartUnix:  program.StartUnix,
				EndUnix:    endUnix,
				Live:       live,
				Completed:  completed,
				Status:     status,
				StatusText: statusText,
				Away:       SportsTeam{Name: awayName, Abbreviation: sportsTeamInitials(awayName)},
				Home:       SportsTeam{Name: homeName, Abbreviation: sportsTeamInitials(homeName)},
			}
			event = &value
			byKey[key] = event
		}
		if !sportsEventHasChannel(event, channel.ID) {
			event.Channels = append(event.Channels, SportsChannelMatch{
				ID:           channel.ID,
				Name:         channel.Name,
				CategoryName: categoryName,
				LogoURL:      channel.LogoURL,
				Reason:       "guide: exact program",
				Score:        100,
			})
		}
	}

	events := make([]SportsEvent, 0, len(byKey))
	for _, event := range byKey {
		events = append(events, *event)
	}
	events = normalizeSportsEvents(events)
	sort.Slice(events, func(i, j int) bool {
		if events[i].Live != events[j].Live {
			return events[i].Live
		}
		leftFuture := events[i].StartUnix >= now.Unix()
		rightFuture := events[j].StartUnix >= now.Unix()
		if leftFuture != rightFuture {
			return leftFuture
		}
		if leftFuture {
			return events[i].StartUnix < events[j].StartUnix
		}
		return events[i].StartUnix > events[j].StartUnix
	})
	return capSportsEventsByLeague(events, sportsGuideEventLeagueLimit, sportsGuideEventLimit), refreshScores
}

const (
	sportsGuideEventLimit       = 250
	sportsGuideEventLeagueLimit = 60
)

// capSportsEventsByLeague keeps at most total events from a list already in
// priority order. While the list is over the cap, no league may take more than
// perLeague slots until every league has had its share; live games are always
// kept first. Order is preserved.
func capSportsEventsByLeague(events []SportsEvent, perLeague, total int) []SportsEvent {
	if len(events) <= total {
		return events
	}
	keep := make([]bool, len(events))
	kept := 0
	perLeagueCount := map[string]int{}
	leagueKey := func(event SportsEvent) string {
		return firstNonEmpty(event.LeagueID, "sports")
	}
	for index, event := range events {
		if kept >= total {
			break
		}
		if event.Live {
			keep[index] = true
			kept++
			perLeagueCount[leagueKey(event)]++
		}
	}
	for index, event := range events {
		if kept >= total {
			break
		}
		key := leagueKey(event)
		if keep[index] || perLeagueCount[key] >= perLeague {
			continue
		}
		keep[index] = true
		kept++
		perLeagueCount[key]++
	}
	// Leftover room goes to the over-share leagues in priority order.
	for index := range events {
		if kept >= total {
			break
		}
		if !keep[index] {
			keep[index] = true
			kept++
		}
	}
	selected := make([]SportsEvent, 0, kept)
	for index, event := range events {
		if keep[index] {
			selected = append(selected, event)
		}
	}
	return selected
}

func guideSportsMetadata(categories []string) (bool, bool) {
	sportsMetadata := false
	sportsTalk := false
	for _, category := range categories {
		category = strings.TrimSpace(category)
		if category == "" {
			continue
		}
		text := normalizeMatchText(category)
		if text == "sports talk" {
			sportsTalk = true
			continue
		}
		if text == "sports event" || text == "sporting event" {
			sportsMetadata = true
			continue
		}
		if _, _, _, sportsContext := guideSportsLeague(category); sportsContext {
			sportsMetadata = true
		}
	}
	return sportsMetadata, sportsTalk
}

func guideSportsMatchup(title string) (string, string, bool) {
	title = cleanGuideSportsAnnotations(title)
	if guideSportsNonMatchTitle.MatchString(title) || guideSportsMultipleBouts(title) {
		return "", "", false
	}
	title = guideSportsNextGameSuffix.ReplaceAllString(title, "")
	title = guideSportsTimestampSuffix.ReplaceAllString(title, "")
	locations := sportsMatchupSeparator.FindAllStringIndex(title, -1)
	if len(locations) == 0 {
		return "", "", false
	}
	location := locations[len(locations)-1]
	left := strings.TrimSpace(title[:location[0]])
	right := strings.TrimSpace(title[location[1]:])
	if colon := strings.LastIndex(left, ":"); colon >= 0 {
		left = strings.TrimSpace(left[colon+1:])
	}
	if trimmed := strings.TrimSpace(guideSportsKnownTournamentPrefix.ReplaceAllString(left, "")); trimmed != "" {
		left = trimmed
	}
	if colon := strings.Index(right, ":"); colon >= 0 {
		right = strings.TrimSpace(right[:colon])
	}
	left = cleanGuideSportsTeamName(strings.Trim(left, " -:|,."))
	right = cleanGuideSportsTeamName(strings.Trim(right, " -:|,."))
	if guideSportsClockName.MatchString(left) || guideSportsClockName.MatchString(right) {
		return "", "", false
	}
	if len([]rune(normalizeMatchText(left))) < 2 || len([]rune(normalizeMatchText(right))) < 2 {
		return "", "", false
	}
	return left, right, true
}

// A semicolon-separated fight card is one broadcast, not one pair of fighters.
func guideSportsMultipleBouts(title string) bool {
	parts := strings.Split(title, ";")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if len(sportsMatchupSeparator.FindAllStringIndex(part, -1)) != 1 {
			return false
		}
	}
	return true
}

func cleanGuideSportsAnnotations(value string) string {
	value = strings.NewReplacer("ᴸᶦᵛᵉ", "", "ᴺᵉʷ", "").Replace(value)
	return strings.TrimSpace(value)
}

func cleanGuideSportsTeamName(value string) string {
	value = cleanGuideSportsAnnotations(value)
	value = guideSportsTimestampSuffix.ReplaceAllString(value, "")
	value = guideSportsVenueSuffix.ReplaceAllString(value, "")
	value = guideSportsMatchNumberSuffix.ReplaceAllString(value, "")
	value = guideSportsStageSuffix.ReplaceAllString(value, "")
	value = guideSportsCompetitionSuffix.ReplaceAllString(value, "")
	value = strings.TrimSpace(value)
	if open := strings.LastIndex(value, " ("); open > 0 && strings.HasSuffix(value, ")") {
		base := strings.TrimSpace(value[:open])
		parenthetical := strings.TrimSpace(value[open+2 : len(value)-1])
		if normalizeMatchText(base) == normalizeMatchText(parenthetical) {
			return base
		}
	}
	return strings.TrimSpace(value)
}

func guideSportsVenue(title string) string {
	title = cleanGuideSportsAnnotations(title)
	title = guideSportsTimestampSuffix.ReplaceAllString(title, "")
	match := guideSportsVenueSuffix.FindStringSubmatch(title)
	if len(match) != 2 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(match[1]), " -:|,.")
}

func guideSportsRace(title string) (string, string, bool) {
	if nascarCupRacePattern.MatchString(title) {
		series, location, race := guideSportsMatchup(title)
		if race {
			return series, location, true
		}
	}
	location := formulaERacePattern.FindStringIndex(title)
	if location == nil {
		return "", "", false
	}
	venue := raceLocationPrefix.ReplaceAllString(strings.TrimSpace(title[location[1]:]), "")
	venue = strings.Trim(venue, " -:|,.")
	if venue == "" {
		venue = "Race"
	}
	return "Formula E", venue, true
}

func guideSportsBroadcastStatus(program model.Program, endUnix int64, now time.Time) (bool, bool, string, string) {
	live := program.StartUnix <= now.Unix() && endUnix > now.Unix()
	completed := endUnix <= now.Unix()
	text := normalizeMatchText(cleanGuideSportsAnnotations(program.Title) + " " + program.Summary)
	if strings.HasPrefix(text, "next game ") {
		return false, false, "scheduled", "Upcoming"
	}
	if containsMatchTerm(text, "highlight") || containsMatchTerm(text, "highlights") {
		return live, completed, "highlights", "Highlights"
	}
	for _, term := range []string{"rebroadcast", "replay", "re-air", "reair", "encore", "previously recorded", "tape delayed", "tape-delayed"} {
		if containsMatchTerm(text, term) {
			return live, completed, "replay", "Replay"
		}
	}
	if live {
		if guideSportsTitleIsLive(program.Title) {
			return true, false, "live", "Live"
		}
		return true, false, "airing", "On now"
	}
	if completed {
		return false, true, "ended", "Ended"
	}
	return false, false, "scheduled", ""
}

func sportsTeamInitials(name string) string {
	parts := strings.Fields(name)
	var builder strings.Builder
	for _, part := range parts {
		for _, r := range part {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				builder.WriteRune(r)
				break
			}
		}
		if builder.Len() == 3 {
			break
		}
	}
	return strings.ToUpper(builder.String())
}
