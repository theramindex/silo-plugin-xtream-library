package plugin

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

const (
	// A program is near an event when it overlaps [start-6h, start+8h].
	sportsProgramLeadSeconds  = 6 * 3600
	sportsProgramTrailSeconds = 8 * 3600
	sportsProgramBucketWidth  = int64(3600)
	// Programs spanning more buckets than this are checked for every event
	// rather than copied into dozens of hour buckets.
	sportsProgramMaxBuckets = 48
)

type sportsTerm struct {
	Text         string
	Reason       string
	Weight       int
	TeamName     bool
	Abbreviation bool
	Weak         bool
	norm         string
}

type sportsIndexedChannel struct {
	Channel      model.Channel
	CategoryName string
	ChannelText  string
	CategoryText string
	RawText      string
	Segments     []string
	Programs     []sportsIndexedProgram
	// Precomputed per channel so each event does not rescan the labels.
	contextText  string
	earliestDate time.Time
	hasDate      bool
	ancillary    bool
}

type sportsIndexedProgram struct {
	Program  model.Program
	Text     string
	Segments []string
}

type sportsProgramRef struct {
	channel int32
	program int32
}

type sportsChannelIndex struct {
	Channels []sportsIndexedChannel
	// Hour buckets of program references. A program is listed in every hour it
	// overlaps, so one event only visits the guide around its start time.
	buckets map[int64][]sportsProgramRef
	// Programs with unusable or very long spans are always checked.
	unbucketed []sportsProgramRef
}

func newSportsChannelIndex(snapshot cache.Snapshot) sportsChannelIndex {
	categoryNames := map[string]string{}
	for _, category := range liveCategories(snapshot) {
		categoryNames[category.ID] = category.Name
	}
	programsByChannel := map[string][]sportsIndexedProgram{}
	for _, program := range snapshot.Catalog.Programs {
		programsByChannel[program.ChannelID] = append(programsByChannel[program.ChannelID], sportsIndexedProgram{
			Program:  program,
			Text:     normalizeMatchText(strings.Join([]string{program.Title, program.Summary}, " ")),
			Segments: sportsMatchSegments(program.Title, program.Summary),
		})
	}
	channels := make([]sportsIndexedChannel, 0, len(snapshot.Catalog.Channels))
	seenChannels := map[string]bool{}
	for _, channel := range snapshot.Catalog.Channels {
		if channel.ID == "" || seenChannels[channel.ID] {
			continue
		}
		seenChannels[channel.ID] = true
		categoryName := firstNonEmpty(categoryNames[channel.CategoryID], channel.CategoryName)
		channels = append(channels, newSportsIndexedChannel(channel, categoryName, programsByChannel[channel.ID]))
	}
	index := sportsChannelIndex{Channels: channels, buckets: map[int64][]sportsProgramRef{}}
	for channelPosition := range channels {
		for programPosition, program := range channels[channelPosition].Programs {
			ref := sportsProgramRef{channel: int32(channelPosition), program: int32(programPosition)}
			start, end := sportsProgramSpan(program.Program)
			first, last := sportsBucketOf(start), sportsBucketOf(end)
			if end < start || last-first > sportsProgramMaxBuckets {
				index.unbucketed = append(index.unbucketed, ref)
				continue
			}
			for bucket := first; bucket <= last; bucket++ {
				index.buckets[bucket] = append(index.buckets[bucket], ref)
			}
		}
	}
	return index
}

func newSportsIndexedChannel(channel model.Channel, categoryName string, programs []sportsIndexedProgram) sportsIndexedChannel {
	indexed := sportsIndexedChannel{
		Channel:      channel,
		CategoryName: firstNonEmpty(categoryName, channel.CategoryName),
		ChannelText:  normalizeMatchText(strings.Join([]string{channel.Name, channel.Number}, " ")),
		CategoryText: normalizeMatchText(strings.Join([]string{categoryName, channel.CategoryName}, " ")),
		RawText:      strings.Join([]string{channel.Name, channel.Number, categoryName, channel.CategoryName}, " "),
		Segments:     sportsMatchSegments(channel.Name, categoryName, channel.CategoryName),
		Programs:     programs,
	}
	indexed.contextText = indexed.ChannelText + " " + indexed.CategoryText
	indexed.earliestDate, indexed.hasDate = sportsEarliestTextDate(indexed.RawText)
	indexed.ancillary = sportsAncillaryBroadcast(indexed.RawText)
	return indexed
}

func sportsProgramSpan(program model.Program) (int64, int64) {
	end := program.EndUnix
	if end == 0 {
		end = program.StartUnix + 2*3600
	}
	return program.StartUnix, end
}

func sportsBucketOf(unix int64) int64 {
	bucket := unix / sportsProgramBucketWidth
	if unix < 0 && unix%sportsProgramBucketWidth != 0 {
		bucket--
	}
	return bucket
}

// nearPrograms returns references to programs overlapping the event window,
// ordered by channel then program position. It returns nil, false when the
// event has no start time, meaning every program is near.
func (index sportsChannelIndex) nearPrograms(event SportsEvent) ([]sportsProgramRef, bool) {
	if event.StartUnix == 0 {
		return nil, false
	}
	windowStart := event.StartUnix - sportsProgramLeadSeconds
	windowEnd := event.StartUnix + sportsProgramTrailSeconds
	firstBucket, lastBucket := sportsBucketOf(windowStart), sportsBucketOf(windowEnd)
	refs := make([]sportsProgramRef, 0, 64)
	for bucket := firstBucket; bucket <= lastBucket; bucket++ {
		for _, ref := range index.buckets[bucket] {
			program := index.Channels[ref.channel].Programs[ref.program].Program
			start, _ := sportsProgramSpan(program)
			// A multi-hour program is listed in several buckets; keep it once.
			if max(sportsBucketOf(start), firstBucket) != bucket || !programNearSportsEvent(program, event) {
				continue
			}
			refs = append(refs, ref)
		}
	}
	for _, ref := range index.unbucketed {
		if programNearSportsEvent(index.Channels[ref.channel].Programs[ref.program].Program, event) {
			refs = append(refs, ref)
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].channel != refs[j].channel {
			return refs[i].channel < refs[j].channel
		}
		return refs[i].program < refs[j].program
	})
	return refs, true
}

func matchSportsChannels(event SportsEvent, snapshot cache.Snapshot) []SportsChannelMatch {
	return newSportsChannelIndex(snapshot).Match(event)
}

func (index sportsChannelIndex) Match(event SportsEvent) []SportsChannelMatch {
	matches, _ := index.MatchDetailed(event)
	return matches
}

func (index sportsChannelIndex) MatchDetailed(event SportsEvent) ([]SportsChannelMatch, []SportsMatchDiagnostic) {
	return index.MatchDetailedContext(context.Background(), event)
}

func (index sportsChannelIndex) MatchDetailedContext(ctx context.Context, event SportsEvent) ([]SportsChannelMatch, []SportsMatchDiagnostic) {
	query := newSportsMatchQuery(event)
	if len(query.terms) == 0 {
		return []SportsChannelMatch{}, []SportsMatchDiagnostic{}
	}
	refs, bucketed := index.nearPrograms(event)
	matches := make([]SportsChannelMatch, 0)
	diagnostics := make([]SportsMatchDiagnostic, 0)
	nearPrograms := make([]*sportsIndexedProgram, 0, 32)
	cursor := 0
	for position := range index.Channels {
		if ctx.Err() != nil {
			break
		}
		indexed := &index.Channels[position]
		nearPrograms = nearPrograms[:0]
		if bucketed {
			for cursor < len(refs) && int(refs[cursor].channel) < position {
				cursor++
			}
			for cursor < len(refs) && int(refs[cursor].channel) == position {
				nearPrograms = append(nearPrograms, &indexed.Programs[refs[cursor].program])
				cursor++
			}
		} else {
			for program := range indexed.Programs {
				nearPrograms = append(nearPrograms, &indexed.Programs[program])
			}
		}
		result := query.scoreChannel(indexed, nearPrograms)
		if result.Score < sportsChannelMinimumScore {
			if result.RejectedReason != "" {
				diagnostics = append(diagnostics, SportsMatchDiagnostic{ChannelID: indexed.Channel.ID, ChannelName: indexed.Channel.Name, Reason: result.RejectedReason})
			}
			continue
		}
		matches = append(matches, SportsChannelMatch{
			ID:           indexed.Channel.ID,
			Name:         indexed.Channel.Name,
			CategoryName: indexed.CategoryName,
			LogoURL:      indexed.Channel.LogoURL,
			Reason:       result.Reason,
			Evidence:     result.Evidence,
			Confidence:   result.Confidence,
			Score:        result.Score,
		})
		diagnostics = append(diagnostics, SportsMatchDiagnostic{ChannelID: indexed.Channel.ID, ChannelName: indexed.Channel.Name, Accepted: true, Evidence: result.Evidence, Confidence: result.Confidence, Reason: result.Reason, Score: result.Score})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Name < matches[j].Name
	})
	if len(matches) > 6 {
		matches = matches[:6]
	}
	sort.SliceStable(diagnostics, func(i, j int) bool {
		if diagnostics[i].Accepted != diagnostics[j].Accepted {
			return diagnostics[i].Accepted
		}
		if diagnostics[i].Score != diagnostics[j].Score {
			return diagnostics[i].Score > diagnostics[j].Score
		}
		return diagnostics[i].ChannelName < diagnostics[j].ChannelName
	})
	if len(diagnostics) > 12 {
		diagnostics = diagnostics[:12]
	}
	return matches, diagnostics
}

func sportsMatchTerms(event SportsEvent) []sportsTerm {
	var terms []sportsTerm
	add := func(text, reason string, weight int, teamName, abbreviation, weak bool) {
		text = strings.TrimSpace(text)
		if text == "" || len([]rune(text)) < 3 {
			return
		}
		normalized := normalizeMatchText(text)
		for _, existing := range terms {
			if existing.norm == normalized {
				return
			}
		}
		terms = append(terms, sportsTerm{Text: text, Reason: reason, Weight: weight, TeamName: teamName, Abbreviation: abbreviation, Weak: weak, norm: normalized})
	}
	add(event.Home.Name, event.Home.Name, 60, true, false, false)
	add(event.Away.Name, event.Away.Name, 60, true, false, false)
	add(event.Home.Abbreviation, event.Home.Abbreviation, 28, false, true, false)
	add(event.Away.Abbreviation, event.Away.Abbreviation, 28, false, true, false)
	add(sportsWeakTeamAlias(event.Home.Name), event.Home.Name+" alias", 18, false, false, true)
	add(sportsWeakTeamAlias(event.Away.Name), event.Away.Name+" alias", 18, false, false, true)
	// League names are too broad for channel matching; "NFL" or "MLB" would pull in every team group.
	add(event.Name, "event title", 22, false, false, false)
	add(event.ShortName, "event title", 22, false, false, false)
	return terms
}

type sportsChannelScore struct {
	Score          int
	Reason         string
	Evidence       string
	Confidence     string
	RejectedReason string
}

// sportsMatchQuery holds one event's normalized names so channel and program
// checks do not renormalize them for every candidate.
type sportsMatchQuery struct {
	event                SportsEvent
	terms                []sportsTerm
	name, shortName      string
	homeName, awayName   string
	homeAbbr, awayAbbr   string
	leagueName, leagueID string
	eventDay             time.Time
	dated                bool
}

func newSportsMatchQuery(event SportsEvent) sportsMatchQuery {
	query := sportsMatchQuery{
		event:      event,
		terms:      sportsMatchTerms(event),
		name:       normalizeMatchText(event.Name),
		shortName:  normalizeMatchText(event.ShortName),
		homeName:   normalizeMatchText(event.Home.Name),
		awayName:   normalizeMatchText(event.Away.Name),
		homeAbbr:   normalizeMatchText(event.Home.Abbreviation),
		awayAbbr:   normalizeMatchText(event.Away.Abbreviation),
		leagueName: normalizeMatchText(event.LeagueName),
		leagueID:   normalizeMatchText(event.LeagueID),
	}
	if event.StartUnix > 0 {
		query.dated = true
		query.eventDay = time.Unix(event.StartUnix, 0).UTC().Truncate(24 * time.Hour)
	}
	return query
}

// scoreChannel scores one channel against the event using only the programs
// near its start time.
func (query sportsMatchQuery) scoreChannel(channel *sportsIndexedChannel, programs []*sportsIndexedProgram) sportsChannelScore {
	if query.dated && channel.hasDate && channel.earliestDate.Before(query.eventDay) {
		return sportsChannelScore{RejectedReason: "Feed is dated before this fixture"}
	}
	score := 0
	structuralMatch := false
	strongGuideMatch := false
	reasons := map[string]bool{}
	// Abbreviations such as TEN and EDM are too ambiguous on their own. A
	// channel needs to identify the event's league unless its guide confirms it.
	hasAbbreviationContext := containsNormalizedTerm(channel.contextText, query.leagueName) || containsNormalizedTerm(channel.contextText, query.leagueID)
	channelBothSides := query.segmentsContainBothSides(channel.Segments)
	if !channelBothSides && query.segmentsContainSide(channel.Segments, query.homeName, query.homeAbbr) && query.segmentsContainSide(channel.Segments, query.awayName, query.awayAbbr) {
		return sportsChannelScore{RejectedReason: "Teams appear in separate provider-label segments"}
	}
	for _, term := range query.terms {
		if (term.Abbreviation || term.Weak) && !hasAbbreviationContext {
			continue
		}
		if containsPreparedStructuralTerm(channel.ChannelText, term) {
			score += term.Weight
			structuralMatch = true
			reasons["channel: "+term.Reason] = true
		}
		if containsPreparedStructuralTerm(channel.CategoryText, term) {
			score += term.Weight / 2
			structuralMatch = true
			reasons["group: "+term.Reason] = true
		}
	}
	for _, program := range programs {
		if program.Text == "" {
			continue
		}
		if !strongGuideMatch && query.strongGuideMatch(program.Text) && query.segmentsContainBothSides(program.Segments) {
			strongGuideMatch = true
		}
		for _, term := range query.terms {
			if containsNormalizedTerm(program.Text, term.norm) {
				score += term.Weight + 20
				reasons["guide: "+term.Reason] = true
			}
		}
	}
	if score == 0 {
		return sportsChannelScore{}
	}
	if !structuralMatch && !strongGuideMatch {
		return sportsChannelScore{RejectedReason: "Candidate lacks strong structural or EPG evidence"}
	}
	if channel.ancillary {
		score -= 45
		reasons["ancillary feed"] = true
	}
	if score < sportsChannelMinimumScore {
		return sportsChannelScore{RejectedReason: "Ancillary or low-confidence feed was demoted"}
	}
	evidence := "channel"
	confidence := "medium"
	if strongGuideMatch {
		evidence = "epg"
		confidence = "high"
	} else if channelBothSides {
		evidence = "channel"
		confidence = "high"
	} else if channel.ancillary {
		confidence = "low"
	}
	return sportsChannelScore{Score: score, Reason: joinMatchReasons(reasons), Evidence: evidence, Confidence: confidence}
}

func (query sportsMatchQuery) segmentsContainBothSides(segments []string) bool {
	for _, segment := range segments {
		homeMatch := containsNormalizedTerm(segment, query.homeName) || containsNormalizedTerm(segment, query.homeAbbr)
		awayMatch := containsNormalizedTerm(segment, query.awayName) || containsNormalizedTerm(segment, query.awayAbbr)
		if homeMatch && awayMatch {
			return true
		}
	}
	return false
}

func (query sportsMatchQuery) segmentsContainSide(segments []string, name, abbreviation string) bool {
	for _, segment := range segments {
		if containsNormalizedTerm(segment, name) || containsNormalizedTerm(segment, abbreviation) {
			return true
		}
	}
	return false
}

// strongGuideMatch is strongSportsGuideMatch with the event names normalized once.
func (query sportsMatchQuery) strongGuideMatch(programText string) bool {
	if containsNormalizedTerm(programText, query.name) || containsNormalizedTerm(programText, query.shortName) {
		return true
	}
	homeName := containsNormalizedTerm(programText, query.homeName)
	awayName := containsNormalizedTerm(programText, query.awayName)
	if homeName && awayName {
		return true
	}
	homeAbbr := containsNormalizedTerm(programText, query.homeAbbr)
	awayAbbr := containsNormalizedTerm(programText, query.awayAbbr)
	if (homeName || homeAbbr) && (awayName || awayAbbr) {
		return true
	}
	return query.leagueName != "" && containsNormalizedTerm(programText, query.leagueName) && (homeName || awayName || homeAbbr || awayAbbr)
}

// containsNormalizedTerm matches whole words like containsMatchTerm, but both
// values are already normalized, so it neither renormalizes nor allocates.
func containsNormalizedTerm(text, term string) bool {
	if term == "" {
		return false
	}
	for offset := 0; offset <= len(text)-len(term); {
		found := strings.Index(text[offset:], term)
		if found < 0 {
			return false
		}
		start := offset + found
		end := start + len(term)
		if (start == 0 || text[start-1] == ' ') && (end == len(text) || text[end] == ' ') {
			return true
		}
		offset = start + 1
	}
	return false
}

// Single-word national teams should not match a longer club name merely because
// the country is one word inside it (for example, England vs New England Revolution).
func containsPreparedStructuralTerm(text string, term sportsTerm) bool {
	if !containsNormalizedTerm(text, term.norm) {
		return false
	}
	if !term.TeamName || strings.Contains(term.norm, " ") {
		return true
	}
	return text == term.norm || strings.HasPrefix(text, term.norm+" ") || strings.HasSuffix(text, " "+term.norm)
}

func sportsMatchSegments(values ...string) []string {
	segments := make([]string, 0, len(values)*2)
	for _, value := range values {
		for _, segment := range strings.FieldsFunc(value, func(r rune) bool { return r == ':' || r == '|' }) {
			normalized := normalizeMatchText(segment)
			if normalized != "" {
				segments = append(segments, normalized)
			}
		}
	}
	return segments
}

func sportsWeakTeamAlias(name string) string {
	words := strings.Fields(normalizeMatchText(name))
	if len(words) < 2 {
		return ""
	}
	alias := words[len(words)-1]
	if len(alias) < 5 {
		return ""
	}
	for _, generic := range []string{"united", "city", "county", "state", "football", "basketball", "women", "men", "team", "club"} {
		if alias == generic {
			return ""
		}
	}
	if _, err := strconv.Atoi(alias); err == nil {
		return ""
	}
	return alias
}

func sportsAncillaryBroadcast(value string) bool {
	value = strings.ToLower(value)
	for _, term := range []string{"pregame", "pre-game", "preview", "press conference", "prelims", "preliminary", "multiview", "multi-view", "countdown", "studio show"} {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

// sportsEarliestTextDate finds the earliest valid ISO or US date in a label.
// A label is dated before an event when that date precedes the event day.
func sportsEarliestTextDate(value string) (time.Time, bool) {
	var earliest time.Time
	found := false
	consider := func(yearText, monthText, dayText string) {
		parsed, ok := sportsParsedDate(yearText, monthText, dayText)
		if ok && (!found || parsed.Before(earliest)) {
			earliest, found = parsed, true
		}
	}
	for _, match := range sportsISODatePattern.FindAllStringSubmatch(value, -1) {
		if len(match) == 4 {
			consider(match[1], match[2], match[3])
		}
	}
	for _, match := range sportsUSDatePattern.FindAllStringSubmatch(value, -1) {
		if len(match) == 4 {
			consider(match[3], match[1], match[2])
		}
	}
	return earliest, found
}

func sportsParsedDate(yearText, monthText, dayText string) (time.Time, bool) {
	year, yearErr := strconv.Atoi(yearText)
	month, monthErr := strconv.Atoi(monthText)
	day, dayErr := strconv.Atoi(dayText)
	if yearErr != nil || monthErr != nil || dayErr != nil || month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), true
}

func strongSportsGuideMatch(programText string, event SportsEvent) bool {
	if containsMatchTerm(programText, event.Name) || containsMatchTerm(programText, event.ShortName) {
		return true
	}
	homeName := containsMatchTerm(programText, event.Home.Name)
	awayName := containsMatchTerm(programText, event.Away.Name)
	if homeName && awayName {
		return true
	}
	homeAbbr := containsMatchTerm(programText, event.Home.Abbreviation)
	awayAbbr := containsMatchTerm(programText, event.Away.Abbreviation)
	if (homeName || homeAbbr) && (awayName || awayAbbr) {
		return true
	}
	leagueName := strings.TrimSpace(event.LeagueName)
	if leagueName != "" && containsMatchTerm(programText, leagueName) && (homeName || awayName || homeAbbr || awayAbbr) {
		return true
	}
	return false
}

func programNearSportsEvent(program model.Program, event SportsEvent) bool {
	if event.StartUnix == 0 {
		return true
	}
	start := event.StartUnix - sportsProgramLeadSeconds
	end := event.StartUnix + sportsProgramTrailSeconds
	programStart, programEnd := sportsProgramSpan(program)
	return programEnd >= start && programStart <= end
}

func joinMatchReasons(reasons map[string]bool) string {
	values := make([]string, 0, len(reasons))
	for reason := range reasons {
		values = append(values, reason)
	}
	sort.Strings(values)
	if len(values) > 3 {
		values = values[:3]
	}
	return strings.Join(values, ", ")
}
