package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

type sportsProvider interface {
	Events(context.Context, time.Time) ([]SportsEvent, error)
	Source() string
}

type sportsEventEnricher interface {
	EnrichEvents(context.Context, []SportsEvent, int) []SportsEvent
}

type sportsLeagueTeamProvider interface {
	LeagueTeams(context.Context, string) ([]SportsTeam, error)
}

const (
	sportsChannelMinimumScore  = 28
	sportsProviderFetchTimeout = 24 * time.Second
	sportsBuildTimeout         = 30 * time.Second
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

type sportsEventCache struct {
	Events       []SportsEvent
	UpdatedUnix  int64
	Source       string
	ExpiresAfter time.Time
}

type sportsPreparedCache struct {
	Payload          SportsPayload
	ExpiresAfter     time.Time
	GuideUpdatedUnix int64
	Ready            bool
	Refreshing       bool
}

type SportsPayload struct {
	UpdatedAtUnix int64          `json:"updatedAtUnix"`
	Source        string         `json:"source"`
	Leagues       []SportsLeague `json:"leagues"`
	Events        []SportsEvent  `json:"events"`
	FavoriteTeams []string       `json:"favoriteTeams"`
	Refreshing    bool           `json:"refreshing,omitempty"`
	Error         string         `json:"error,omitempty"`
}

type SportsLeague struct {
	ID              string `json:"id"`
	ProviderID      string `json:"providerId,omitempty"`
	Name            string `json:"name"`
	SportName       string `json:"sportName,omitempty"`
	LogoURL         string `json:"logoUrl,omitempty"`
	LogoFallbackURL string `json:"logoFallbackUrl,omitempty"`
	Description     string `json:"description,omitempty"`
	LiveCount       int    `json:"liveCount"`
	UpcomingCount   int    `json:"upcomingCount"`
}

type SportsTeam struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Abbreviation    string `json:"abbreviation,omitempty"`
	LogoURL         string `json:"logoUrl,omitempty"`
	LogoFallbackURL string `json:"logoFallbackUrl,omitempty"`
	PrimaryColor    string `json:"primaryColor,omitempty"`
	SecondaryColor  string `json:"secondaryColor,omitempty"`
	Favorite        bool   `json:"favorite,omitempty"`
}

type SportsImage struct {
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

type SportsArtwork struct {
	Poster    *SportsImage `json:"poster,omitempty"`
	Backdrop  *SportsImage `json:"backdrop,omitempty"`
	Logo      *SportsImage `json:"logo,omitempty"`
	Banner    *SportsImage `json:"banner,omitempty"`
	Thumbnail *SportsImage `json:"thumbnail,omitempty"`
}

type SportsEvent struct {
	ID                      string                  `json:"id"`
	StableID                string                  `json:"stableId"`
	ProviderSource          string                  `json:"providerSource,omitempty"`
	ProviderID              string                  `json:"providerId,omitempty"`
	ProviderShortID         string                  `json:"providerShortId,omitempty"`
	ProviderLeagueID        string                  `json:"providerLeagueId,omitempty"`
	LeagueID                string                  `json:"leagueId"`
	LeagueName              string                  `json:"leagueName"`
	LeagueLogoURL           string                  `json:"leagueLogoUrl,omitempty"`
	LeagueLogoFallbackURL   string                  `json:"leagueLogoFallbackUrl,omitempty"`
	GameThumbsBackgroundURL string                  `json:"gameThumbsBackgroundUrl,omitempty"`
	LeagueDescription       string                  `json:"leagueDescription,omitempty"`
	SportName               string                  `json:"sportName,omitempty"`
	Name                    string                  `json:"name"`
	ShortName               string                  `json:"shortName,omitempty"`
	EventType               string                  `json:"eventType,omitempty"`
	Season                  string                  `json:"season,omitempty"`
	Round                   string                  `json:"round,omitempty"`
	Venue                   string                  `json:"venue,omitempty"`
	BroadcastTimezone       string                  `json:"broadcastTimezone,omitempty"`
	ImageURL                string                  `json:"imageUrl,omitempty"`
	Artwork                 *SportsArtwork          `json:"artwork,omitempty"`
	Description             string                  `json:"description,omitempty"`
	Status                  string                  `json:"status"`
	StatusText              string                  `json:"statusText,omitempty"`
	Period                  string                  `json:"period,omitempty"`
	Clock                   string                  `json:"clock,omitempty"`
	StartUnix               int64                   `json:"startUnix"`
	EndUnix                 int64                   `json:"endUnix,omitempty"`
	Home                    SportsTeam              `json:"home"`
	Away                    SportsTeam              `json:"away"`
	HomeScore               string                  `json:"homeScore,omitempty"`
	AwayScore               string                  `json:"awayScore,omitempty"`
	HomeRank                int                     `json:"homeRank,omitempty"`
	AwayRank                int                     `json:"awayRank,omitempty"`
	Spread                  *float64                `json:"spread,omitempty"`
	Live                    bool                    `json:"live"`
	Completed               bool                    `json:"completed"`
	Channels                []SportsChannelMatch    `json:"channels"`
	Ranking                 SportsEventRanking      `json:"ranking"`
	MatchDiagnostics        []SportsMatchDiagnostic `json:"matchDiagnostics,omitempty"`
}

type SportsChannelMatch struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CategoryName string `json:"categoryName,omitempty"`
	LogoURL      string `json:"logoUrl,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Evidence     string `json:"evidence,omitempty"`
	Confidence   string `json:"confidence,omitempty"`
	Score        int    `json:"score"`
}

type SportsEventRanking struct {
	Score   float64               `json:"score"`
	Raw     float64               `json:"raw"`
	Knee    float64               `json:"knee"`
	Signals []SportsRankingSignal `json:"signals"`
}

type SportsRankingSignal struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Detail string  `json:"detail,omitempty"`
	Points float64 `json:"points"`
}

type SportsMatchDiagnostic struct {
	ChannelID   string `json:"channelId,omitempty"`
	ChannelName string `json:"channelName,omitempty"`
	Accepted    bool   `json:"accepted"`
	Evidence    string `json:"evidence,omitempty"`
	Confidence  string `json:"confidence,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Score       int    `json:"score,omitempty"`
}

func (s *HTTPRoutesServer) handleSports(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if !s.sportsFeatureEnabled() {
		return s.respondJSON(http.StatusOK, SportsPayload{})
	}
	if queryValue(request, "game_stats") != "" {
		return s.handleSportsGameStats(ctx, request)
	}
	if request.GetMethod() != "" && request.GetMethod() != http.MethodGet {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed"), nil
	}
	payload := s.preparedSportsPayload(queryValue(request, "refresh") == "1")
	return s.respondJSON(http.StatusOK, payload)
}

type SportsLeagueTeamsPayload struct {
	Teams []SportsTeam `json:"teams"`
}

func (s *HTTPRoutesServer) handleSportsLeagueTeams(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if !s.sportsFeatureEnabled() {
		return s.respondJSON(http.StatusOK, SportsLeagueTeamsPayload{Teams: []SportsTeam{}})
	}
	if request.GetMethod() != "" && request.GetMethod() != http.MethodGet {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed"), nil
	}
	leagueID := strings.TrimSpace(queryValue(request, "league_id"))
	if leagueID == "" {
		return textResponse(http.StatusBadRequest, "league_id is required"), nil
	}
	payload := s.preparedSportsPayload(false)
	providerID := ""
	leagueName := leagueID
	sportName := ""
	for _, league := range payload.Leagues {
		if league.ID != leagueID {
			continue
		}
		providerID = league.ProviderID
		leagueName = firstNonEmpty(league.Name, leagueID)
		sportName = league.SportName
		break
	}
	teams := sportsLeagueEventTeams(payload.Events, leagueID)
	if provider, ok := s.sportsProvider.(sportsLeagueTeamProvider); ok && providerID != "" {
		rosterCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		providerTeams, err := provider.LeagueTeams(rosterCtx, providerID)
		cancel()
		if err == nil {
			teams = mergeSportsLeagueRosterTeams(leagueID, leagueName, sportName, teams, providerTeams)
		}
	}
	return s.respondJSON(http.StatusOK, SportsLeagueTeamsPayload{Teams: teams})
}

func sportsLeagueEventTeams(events []SportsEvent, leagueID string) []SportsTeam {
	teams := make([]SportsTeam, 0)
	for _, event := range events {
		if event.LeagueID != leagueID {
			continue
		}
		teams = append(teams, event.Away, event.Home)
	}
	return mergeSportsLeagueRosterTeams(leagueID, leagueID, "", teams)
}

func mergeSportsLeagueRosterTeams(leagueID, leagueName, sportName string, groups ...[]SportsTeam) []SportsTeam {
	identities := make([]SportsTeam, 0)
	for _, group := range groups {
		for _, team := range group {
			team = normalizeSportsTeam(team)
			if strings.TrimSpace(team.Name) == "" {
				continue
			}
			identities = append(identities, applySportsIdentityFallbacks(SportsEvent{
				LeagueID: leagueID, LeagueName: leagueName, SportName: sportName, Home: team,
			}).Home)
		}
	}
	nicknameAliases := uniqueSportsTeamNicknameAliases(identities)
	byKey := map[string]SportsTeam{}
	order := make([]string, 0)
	for _, identity := range identities {
		key := normalizeMatchText(identity.Name)
		if canonical := nicknameAliases[key]; canonical != "" {
			key = canonical
		}
		if key == "" {
			key = identity.ID
		}
		if existing, ok := byKey[key]; ok {
			identity = mergeSportsTeamIdentity(existing, identity)
		} else {
			order = append(order, key)
		}
		byKey[key] = identity
	}
	teams := make([]SportsTeam, 0, len(order))
	for _, key := range order {
		teams = append(teams, byKey[key])
	}
	sort.Slice(teams, func(i, j int) bool { return teams[i].Name < teams[j].Name })
	return teams
}

func uniqueSportsTeamNicknameAliases(teams []SportsTeam) map[string]string {
	candidates := map[string]map[string]bool{}
	for _, team := range teams {
		fullName := normalizeMatchText(team.Name)
		parts := strings.Fields(fullName)
		for start := 1; start < len(parts); start++ {
			alias := strings.Join(parts[start:], " ")
			if candidates[alias] == nil {
				candidates[alias] = map[string]bool{}
			}
			candidates[alias][fullName] = true
		}
	}
	aliases := map[string]string{}
	for alias, fullNames := range candidates {
		if len(fullNames) != 1 {
			continue
		}
		for fullName := range fullNames {
			aliases[alias] = fullName
		}
	}
	return aliases
}

func mergeSportsTeamIdentity(primary, supplemental SportsTeam) SportsTeam {
	primary.LogoFallbackURL = firstNonEmpty(primary.LogoFallbackURL, supplemental.LogoFallbackURL)
	primary.ID = firstNonEmpty(primary.ID, supplemental.ID)
	if primary.Name == "" || len(normalizeMatchText(supplemental.Name)) > len(normalizeMatchText(primary.Name)) {
		primary.Name = supplemental.Name
	}
	primary.Abbreviation = firstNonEmpty(primary.Abbreviation, supplemental.Abbreviation)
	if primary.LogoURL == "" || (strings.HasPrefix(primary.LogoURL, gameThumbsPublicBaseURL+"/") && supplemental.LogoURL != "" && !strings.HasPrefix(supplemental.LogoURL, gameThumbsPublicBaseURL+"/")) {
		primary.LogoURL = supplemental.LogoURL
	}
	primary.PrimaryColor = firstNonEmpty(primary.PrimaryColor, supplemental.PrimaryColor)
	primary.SecondaryColor = firstNonEmpty(primary.SecondaryColor, supplemental.SecondaryColor)
	primary.Favorite = primary.Favorite || supplemental.Favorite
	return primary
}

func (s *HTTPRoutesServer) handleSportsFavorite(request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	return userStateUnavailableResponse(), nil
}

func (s *HTTPRoutesServer) sportsPayload(ctx context.Context, refresh bool) SportsPayload {
	now := time.Now()
	snapshot := s.store.Current()
	guideEvents, refreshScores := sportsEventsFromGuideWithScoreHints(snapshot, now)
	providerCtx, cancelProvider := context.WithTimeout(ctx, sportsProviderFetchTimeout)
	events, updatedUnix, source, err := s.cachedSportsEvents(providerCtx, now, refresh || refreshScores)
	cancelProvider()
	if len(guideEvents) > 0 {
		if len(events) == 0 {
			events = guideEvents
			source = "EPG fallback"
			err = nil
		} else {
			events = mergeSportsGuideEvents(events, guideEvents)
			source = firstNonEmpty(source, "Sports provider") + " + EPG"
		}
		if updatedUnix <= 0 {
			updatedUnix = snapshot.Health.EPGLastSuccessUnix
			if updatedUnix <= 0 {
				updatedUnix = now.Unix()
			}
		}
	}
	channelIndex := newSportsChannelIndex(snapshot)
	for index := range events {
		events[index] = normalizeSportsEventFreshness(events[index], now)
		events[index].Home.Favorite = false
		events[index].Away.Favorite = false
		if len(events[index].Channels) > 0 {
			events[index].Channels = mergeSportsChannelMatches(events[index].Channels)
			events[index].MatchDiagnostics = make([]SportsMatchDiagnostic, 0, len(events[index].Channels))
			for _, match := range events[index].Channels {
				events[index].MatchDiagnostics = append(events[index].MatchDiagnostics, SportsMatchDiagnostic{
					ChannelID: match.ID, ChannelName: match.Name, Accepted: true,
					Evidence: "epg", Confidence: "high", Reason: firstNonEmpty(match.Reason, "EPG-derived event feed"), Score: match.Score,
				})
			}
			continue
		}
		if ctx.Err() != nil {
			continue
		}
		matches, diagnostics := channelIndex.MatchDetailedContext(ctx, events[index])
		events[index].Channels = mergeSportsChannelMatches(events[index].Channels, matches)
		events[index].MatchDiagnostics = diagnostics
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].Live != events[j].Live {
			return events[i].Live
		}
		leftStart := sportsSortStartUnix(events[i])
		rightStart := sportsSortStartUnix(events[j])
		if leftStart != rightStart {
			return leftStart < rightStart
		}
		return events[i].Name < events[j].Name
	})
	events = filterPlayableSportsEvents(events)
	if enricher, ok := s.sportsProvider.(sportsEventEnricher); ok {
		events = enricher.EnrichEvents(ctx, events, 8)
	}
	events = rankSportsEvents(events, now)
	events = s.proxySportsEventImages(events)
	payload := SportsPayload{
		UpdatedAtUnix: updatedUnix,
		Source:        source,
		Leagues:       sportsLeagues(events),
		Events:        events,
		FavoriteTeams: []string{},
	}
	if err != nil {
		payload.Error = err.Error()
	}
	return payload
}

func guideSportsTitleHasScoreLookupHint(title string) bool {
	return guideSportsTitleIsLive(title) || strings.Contains(title, "ᴺᵉʷ")
}

func guideSportsTitleIsLive(title string) bool {
	return strings.Contains(title, "ᴸᶦᵛᵉ") || containsMatchTerm(normalizeMatchText(title), "live")
}

func normalizeSportsEventFreshness(event SportsEvent, now time.Time) SportsEvent {
	if !event.Live {
		return event
	}
	stale := event.EndUnix > 0 && event.EndUnix < now.Add(-2*time.Hour).Unix()
	if !stale && event.StartUnix > 0 && event.StartUnix < now.Add(-18*time.Hour).Unix() {
		stale = true
	}
	if !stale {
		return event
	}
	event.Live = false
	event.Completed = true
	event.Status = "final"
	event.StatusText = "Final"
	return event
}

func sportsPlayableChannelMatch(match SportsChannelMatch) bool {
	switch strings.ToLower(strings.TrimSpace(match.Confidence)) {
	case "low":
		return false
	case "high", "medium":
		return true
	default:
		return match.Score >= sportsChannelMinimumScore
	}
}

func filterPlayableSportsEvents(events []SportsEvent) []SportsEvent {
	playable := make([]SportsEvent, 0, len(events))
	for _, event := range events {
		channels := make([]SportsChannelMatch, 0, len(event.Channels))
		for _, match := range event.Channels {
			if sportsPlayableChannelMatch(match) {
				channels = append(channels, match)
			}
		}
		if len(channels) == 0 {
			continue
		}
		event.Channels = channels
		playable = append(playable, event)
	}
	return playable
}

func mergeSportsGuideEvents(events, guideEvents []SportsEvent) []SportsEvent {
	merged := cloneSportsEvents(events)
	for _, guideEvent := range guideEvents {
		matched := false
		for index := range merged {
			if !sportsEventsSameMatchup(merged[index], guideEvent) {
				continue
			}
			merged[index].Channels = mergeSportsChannelMatches(guideEvent.Channels, merged[index].Channels)
			if guideEvent.SportName != "" && guideEvent.SportName != "Sports" {
				merged[index].LeagueID = guideEvent.LeagueID
				merged[index].LeagueName = guideEvent.LeagueName
				merged[index].SportName = guideEvent.SportName
				merged[index].LeagueLogoURL = firstNonEmpty(guideEvent.LeagueLogoURL, merged[index].LeagueLogoURL)
			}
			for _, pair := range [][2]*SportsTeam{{&merged[index].Home, &guideEvent.Home}, {&merged[index].Away, &guideEvent.Away}} {
				if normalizeMatchText(pair[0].Name) == normalizeMatchText(pair[1].Name) {
					*pair[0] = mergeSportsTeamIdentity(*pair[1], *pair[0])
				}
			}
			if guideEvent.Live && merged[index].Status == "scheduled" {
				merged[index].Live = true
				merged[index].Status = "airing"
				merged[index].StatusText = "On now"
			}
			matched = true
			break
		}
		if !matched {
			if guideEvent.Live && guideEvent.Status == "airing" {
				for _, providerEvent := range merged {
					if providerEvent.Completed && providerEvent.StartUnix < guideEvent.StartUnix && sportsEventsSameIdentity(providerEvent, guideEvent) {
						guideEvent.Status = "replay"
						guideEvent.StatusText = "Replay"
						break
					}
				}
			}
			merged = append(merged, guideEvent)
		}
	}
	return merged
}

func sportsEventsSameMatchup(left, right SportsEvent) bool {
	if left.StartUnix > 0 && right.StartUnix > 0 {
		difference := left.StartUnix - right.StartUnix
		if difference < 0 {
			difference = -difference
		}
		if difference > int64(6*time.Hour/time.Second) {
			return false
		}
	}
	return sportsEventsSameIdentity(left, right)
}

func sportsEventsSameIdentity(left, right SportsEvent) bool {
	leftText := normalizeMatchText(strings.Join([]string{left.Name, left.ShortName, left.Away.Name, left.Home.Name}, " "))
	rightText := normalizeMatchText(strings.Join([]string{right.Name, right.ShortName, right.Away.Name, right.Home.Name}, " "))
	return strongSportsGuideMatch(leftText, right) && strongSportsGuideMatch(rightText, left)
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
		leagueID, leagueName, sportName, sportsContext := guideSportsLeague(strings.Join([]string{displayTitle, strings.Join(program.Categories, " "), channel.Name, categoryName}, " "))
		sportsContext = sportsContext || metadataSports
		awayName, homeName, matchup := guideSportsMatchup(displayTitle)
		eventType := ""
		if series, location, race := guideSportsRace(displayTitle); race {
			awayName, homeName, matchup = series, location, true
			eventType = "race"
		}
		boutCard := leagueID == "boxing" && guideSportsMultipleBouts(displayTitle)
		if !sportsContext || (!matchup && !metadataSports && !boutCard) {
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
	if len(events) > 250 {
		events = events[:250]
	}
	return events, refreshScores
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
	competitionText := normalizeMatchText(value)
	if containsMatchTerm(competitionText, "lanka premier league") || (containsMatchTerm(competitionText, "lpl") && containsMatchTerm(competitionText, "cricket")) {
		return "lanka-premier-league", "Lanka Premier League", "Cricket", true
	}
	if containsMatchTerm(competitionText, "hoopqueens") || containsMatchTerm(competitionText, "hoop queens") {
		return "hoopqueens", "HoopQueens Basketball", "Basketball", true
	}
	if containsMatchTerm(competitionText, "canada cup") && containsMatchTerm(competitionText, "softball") {
		return "canada-cup-softball", "Canada Cup Softball", "Softball", true
	}
	if text := gameThumbsMatchText(value); containsMatchTerm(text, "volleyball nations league") {
		if containsMatchTerm(text, "womens") || containsMatchTerm(text, "women") {
			return "womens-volleyball-nations-league", "Women's Volleyball Nations League", "Volleyball", true
		}
		if containsMatchTerm(text, "mens") || containsMatchTerm(text, "men") {
			return "mens-volleyball-nations-league", "Men's Volleyball Nations League", "Volleyball", true
		}
		return "volleyball-nations-league", "Volleyball Nations League", "Volleyball", true
	}
	if id, name, sport := guideWorldCupCompetition(value); id != "" {
		return id, name, sport, true
	}
	if id, name, sport := guideCollegeCompetition(value); id != "" {
		return id, name, sport, true
	}
	text := normalizeMatchText(value)
	for _, candidate := range []struct {
		terms     []string
		id, name  string
		sportName string
	}{
		{[]string{"wnba"}, "wnba", "WNBA", "Basketball"},
		{[]string{"nba"}, "nba", "NBA", "Basketball"},
		{[]string{"nfl"}, "nfl", "NFL", "Football"},
		{[]string{"cfp", "college football"}, "college-football", "College Football", "Football"},
		{[]string{"mlb", "cubs classics"}, "mlb", "MLB", "Baseball"},
		{[]string{"cebl", "canadian elite basketball league"}, "cebl", "CEBL", "Basketball"},
		{[]string{"nhl"}, "nhl", "NHL", "Hockey"},
		{[]string{"mls"}, "mls", "MLS", "Soccer"},
		{[]string{"uefa champions league", "champions league"}, "uefa-champions-league", "UEFA Champions League", "Soccer"},
		{[]string{"premier league"}, "premier-league", "Premier League", "Soccer"},
		{[]string{"world cup", "fifa"}, "world-cup", "World Cup", "Soccer"},
		{[]string{"ufc", "mma"}, "mma", "MMA", "Combat Sports"},
		{[]string{"boxing", "boxeo"}, "boxing", "Boxing", "Combat Sports"},
		{[]string{"formula e", "formule e"}, "formula-e", "Formula E", "Motorsport"},
		{[]string{"formula 1", "f1"}, "formula-1", "Formula 1", "Motorsport"},
		{[]string{"nascar cup series", "ncs race"}, "nascar-cup-series", "NASCAR Cup Series", "Motorsport"},
		{[]string{"afl premiership", "australian football league"}, "afl", "AFL", "Australian Football"},
		{[]string{"tennis"}, "tennis", "Tennis", "Tennis"},
		{[]string{"golf", "pga"}, "golf", "Golf", "Golf"},
		{[]string{"cricket"}, "cricket", "Cricket", "Cricket"},
	} {
		for _, term := range candidate.terms {
			if containsMatchTerm(text, term) {
				return candidate.id, candidate.name, candidate.sportName, true
			}
		}
	}
	if containsMatchTerm(text, "sports") || containsMatchTerm(text, "soccer") || containsMatchTerm(text, "football") || containsMatchTerm(text, "basketball") || containsMatchTerm(text, "baseball") || containsMatchTerm(text, "hockey") {
		return "sports", "Sports", "Sports", true
	}
	return "", "", "", false
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

func sportsEventHasChannel(event *SportsEvent, channelID string) bool {
	for _, channel := range event.Channels {
		if channel.ID == channelID {
			return true
		}
	}
	return false
}

func mergeSportsChannelMatches(groups ...[]SportsChannelMatch) []SportsChannelMatch {
	seen := map[string]bool{}
	merged := make([]SportsChannelMatch, 0)
	for _, group := range groups {
		for _, channel := range group {
			if channel.ID == "" || seen[channel.ID] {
				continue
			}
			seen[channel.ID] = true
			merged = append(merged, channel)
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		return merged[i].Name < merged[j].Name
	})
	if len(merged) > 6 {
		merged = merged[:6]
	}
	return merged
}

func (s *HTTPRoutesServer) preparedSportsPayload(refresh bool) SportsPayload {
	now := time.Now()
	snapshot := s.store.Current()
	guideUpdatedUnix := snapshot.Health.EPGLastSuccessUnix
	s.sportsPreparedMu.Lock()
	guideChanged := guideUpdatedUnix > 0 && guideUpdatedUnix != s.sportsPrepared.GuideUpdatedUnix
	needsRefresh := !s.sportsPrepared.Ready || refresh || guideChanged || now.After(s.sportsPrepared.ExpiresAfter)
	if needsRefresh && !s.sportsPrepared.Refreshing {
		s.sportsPrepared.Refreshing = true
		go s.rebuildSportsPayload(refresh)
	}
	payload := cloneSportsPayload(s.sportsPrepared.Payload)
	payload.Refreshing = s.sportsPrepared.Refreshing
	if !s.sportsPrepared.Ready && payload.Source == "" {
		provider := s.sportsProvider
		if provider == nil {
			provider = noopSportsProvider{}
		}
		payload.Source = provider.Source()
	}
	if !s.sportsPrepared.Ready && len(payload.Events) == 0 {
		if guideEvents := sportsEventsFromGuide(snapshot, now); len(guideEvents) > 0 {
			payload.Events = guideEvents
			payload.Leagues = sportsLeagues(guideEvents)
			payload.Source = "EPG fallback"
			payload.UpdatedAtUnix = snapshot.Health.EPGLastSuccessUnix
			if payload.UpdatedAtUnix <= 0 {
				payload.UpdatedAtUnix = now.Unix()
			}
		}
	}
	s.sportsPreparedMu.Unlock()
	return payload
}

func (s *HTTPRoutesServer) rebuildSportsPayload(refresh bool) {
	guideUpdatedUnix := s.store.Current().Health.EPGLastSuccessUnix
	ctx, cancel := context.WithTimeout(context.Background(), sportsBuildTimeout)
	payload := s.sportsPayload(ctx, refresh)
	cancel()

	now := time.Now()
	s.sportsPreparedMu.Lock()
	if payload.Error != "" && s.sportsPrepared.Ready && len(s.sportsPrepared.Payload.Events) > 0 {
		stale := cloneSportsPayload(s.sportsPrepared.Payload)
		stale.Error = payload.Error
		payload = stale
	}
	payload.Refreshing = false
	s.sportsPrepared.Payload = cloneSportsPayload(payload)
	s.sportsPrepared.Ready = true
	s.sportsPrepared.Refreshing = false
	s.sportsPrepared.GuideUpdatedUnix = guideUpdatedUnix
	s.sportsPrepared.ExpiresAfter = now.Add(sportsPreparedTTL(payload))
	s.sportsPreparedMu.Unlock()
}

func sportsPreparedTTL(payload SportsPayload) time.Duration {
	if payload.Error != "" || len(payload.Events) == 0 {
		return 30 * time.Second
	}
	return sportsCacheTTL(payload.Events)
}

func cloneSportsPayload(payload SportsPayload) SportsPayload {
	clone := payload
	clone.Events = cloneSportsEvents(payload.Events)
	clone.Leagues = append([]SportsLeague(nil), payload.Leagues...)
	clone.FavoriteTeams = append([]string(nil), payload.FavoriteTeams...)
	return clone
}

func sportsSortStartUnix(event SportsEvent) int64 {
	if event.StartUnix > 0 {
		return event.StartUnix
	}
	return 1<<62 - 1
}

func (s *HTTPRoutesServer) cachedSportsEvents(ctx context.Context, now time.Time, refresh bool) ([]SportsEvent, int64, string, error) {
	s.sportsMu.Lock()
	defer s.sportsMu.Unlock()

	if !refresh && now.Before(s.sportsCache.ExpiresAfter) {
		return cloneSportsEvents(s.sportsCache.Events), s.sportsCache.UpdatedUnix, s.sportsCache.Source, nil
	}
	provider := s.sportsProvider
	if provider == nil {
		provider = noopSportsProvider{}
	}
	events, err := provider.Events(ctx, now)
	source := provider.Source()
	if err != nil {
		if len(s.sportsCache.Events) > 0 {
			return cloneSportsEvents(s.sportsCache.Events), s.sportsCache.UpdatedUnix, s.sportsCache.Source, err
		}
		return []SportsEvent{}, now.Unix(), source, err
	}
	for index := range events {
		events[index].ProviderSource = strings.TrimSpace(source)
	}
	events = normalizeSportsEvents(events)
	updatedUnix := now.Unix()
	s.sportsCache = sportsEventCache{
		Events:       cloneSportsEvents(events),
		UpdatedUnix:  updatedUnix,
		Source:       source,
		ExpiresAfter: now.Add(sportsCacheTTL(events)),
	}
	return cloneSportsEvents(events), updatedUnix, source, nil
}

func sportsCacheTTL(events []SportsEvent) time.Duration {
	for _, event := range events {
		if event.Live {
			return 30 * time.Second
		}
	}
	return 5 * time.Minute
}

func sportsLeagues(events []SportsEvent) []SportsLeague {
	byID := map[string]*SportsLeague{}
	for _, event := range events {
		id := strings.TrimSpace(event.LeagueID)
		if id == "" {
			id = "sports"
		}
		league := byID[id]
		if league == nil {
			league = &SportsLeague{
				ID:              id,
				ProviderID:      event.ProviderLeagueID,
				Name:            firstNonEmpty(event.LeagueName, id),
				SportName:       event.SportName,
				LogoURL:         event.LeagueLogoURL,
				LogoFallbackURL: event.LeagueLogoFallbackURL,
				Description:     event.LeagueDescription,
			}
			byID[id] = league
			if id == "sports" {
				league.Name = "Other sports"
				league.Description = "Guide broadcasts whose league has not been identified."
			}
		}
		if league.ProviderID == "" {
			league.ProviderID = event.ProviderLeagueID
		}
		if league.SportName == "" {
			league.SportName = event.SportName
		}
		if league.LogoURL == "" {
			league.LogoURL = event.LeagueLogoURL
			league.LogoFallbackURL = event.LeagueLogoFallbackURL
		}
		if league.Description == "" {
			league.Description = event.LeagueDescription
		}
		if event.Live {
			league.LiveCount++
		} else if !event.Completed {
			league.UpcomingCount++
		}
	}
	leagues := make([]SportsLeague, 0, len(byID))
	for _, league := range byID {
		leagues = append(leagues, *league)
	}
	sort.Slice(leagues, func(i, j int) bool {
		return leagues[i].Name < leagues[j].Name
	})
	return leagues
}

func normalizeSportsEvents(events []SportsEvent) []SportsEvent {
	normalized := make([]SportsEvent, 0, len(events))
	for _, event := range events {
		event.ID = strings.TrimSpace(event.ID)
		event.ProviderSource = strings.TrimSpace(event.ProviderSource)
		event.ProviderID = strings.TrimSpace(event.ProviderID)
		event.ProviderLeagueID = strings.TrimSpace(event.ProviderLeagueID)
		event.LeagueID = strings.TrimSpace(event.LeagueID)
		event.LeagueName = strings.TrimSpace(event.LeagueName)
		event.LeagueLogoURL = safeSportsImageURL(event.LeagueLogoURL)
		event.LeagueDescription = strings.TrimSpace(event.LeagueDescription)
		event.SportName = strings.TrimSpace(event.SportName)
		event.Name = strings.TrimSpace(event.Name)
		event.ShortName = strings.TrimSpace(event.ShortName)
		event.EventType = strings.TrimSpace(event.EventType)
		event.Season = strings.TrimSpace(event.Season)
		event.Round = strings.TrimSpace(event.Round)
		event.Venue = strings.TrimSpace(event.Venue)
		event.BroadcastTimezone = strings.TrimSpace(event.BroadcastTimezone)
		event.ImageURL = safeSportsImageURL(event.ImageURL)
		event.Description = strings.TrimSpace(event.Description)
		event.Status = strings.TrimSpace(event.Status)
		event.StatusText = strings.TrimSpace(event.StatusText)
		event.Period = strings.TrimSpace(event.Period)
		event.Clock = strings.TrimSpace(event.Clock)
		event = canonicalizeKnownSportsLeague(event)
		event = normalizeLPLTeams(event)
		event.Home = normalizeSportsTeam(event.Home)
		event.Away = normalizeSportsTeam(event.Away)
		event = applySportsIdentityFallbacks(event)
		if event.ID == "" {
			event.ID = stableSportsID(event)
		}
		if event.StableID == "" {
			event.StableID = stableSportsEventIdentity(event)
		}
		if event.Name == "" {
			event.Name = strings.TrimSpace(event.Away.Name + " at " + event.Home.Name)
		}
		if event.ShortName == "" {
			event.ShortName = event.Name
		}
		if event.Status == "" {
			event.Status = "scheduled"
		}
		normalized = append(normalized, event)
	}
	return normalized
}

func canonicalizeKnownSportsLeague(event SportsEvent) SportsEvent {
	leagueID, leagueName, sportName, matched := guideSportsLeague(strings.Join([]string{
		event.LeagueID,
		event.LeagueName,
		event.SportName,
		event.Name,
		event.ShortName,
	}, " "))
	if !matched || leagueID == "" || leagueID == "sports" {
		// Generic guide titles such as "Best of Devils" may omit the league.
		// Require both full club identities before upgrading their classification.
		if event.LeagueID == "" || event.LeagueID == "sports" {
			awayLeague := gameThumbsLeagueSlugForTeam(event.Away, "")
			if awayLeague == gameThumbsLeagueSlugForTeam(event.Home, "") {
				switch awayLeague {
				case "nhl":
					event.LeagueID, event.LeagueName, event.SportName = "nhl", "NHL", "Hockey"
				case "nba":
					event.LeagueID, event.LeagueName, event.SportName = "nba", "NBA", "Basketball"
				}
			}
		}
		return event
	}
	event.LeagueID = leagueID
	event.LeagueName = leagueName
	if sportName != "" {
		event.SportName = sportName
	}
	return event
}

func normalizeSportsTeam(team SportsTeam) SportsTeam {
	team.ID = strings.TrimSpace(team.ID)
	team.Name = strings.TrimSpace(team.Name)
	team.Abbreviation = strings.TrimSpace(team.Abbreviation)
	team.LogoURL = safeSportsImageURL(team.LogoURL)
	team.PrimaryColor = strings.TrimSpace(team.PrimaryColor)
	team.SecondaryColor = strings.TrimSpace(team.SecondaryColor)
	if team.ID == "" {
		team.ID = stableSportsTeamID(team)
	}
	return team
}

func stableSportsID(event SportsEvent) string {
	parts := []string{event.LeagueID, event.Name, event.Home.Name, event.Away.Name, fmt.Sprintf("%d", event.StartUnix)}
	return "sports:" + sportsHash(strings.Join(parts, "|"))
}

func stableSportsEventIdentity(event SportsEvent) string {
	if providerID := strings.TrimSpace(event.ProviderID); providerID != "" {
		return "sports-event:" + sportsHash(strings.Join([]string{
			"provider",
			firstNonEmpty(strings.TrimSpace(event.ProviderSource), "unknown"),
			strings.TrimSpace(event.ProviderLeagueID),
			providerID,
		}, "|"))
	}
	parts := []string{
		strings.ToLower(strings.TrimSpace(event.LeagueID)),
		strings.ToLower(strings.TrimSpace(event.Season)),
		strings.ToLower(strings.TrimSpace(event.Round)),
		strings.ToLower(strings.TrimSpace(event.EventType)),
		strings.ToLower(strings.TrimSpace(event.Away.ID)),
		strings.ToLower(strings.TrimSpace(event.Home.ID)),
		strings.ToLower(strings.TrimSpace(event.Name)),
	}
	return "sports-event:" + sportsHash(strings.Join(parts, "|"))
}

func stableSportsTeamID(team SportsTeam) string {
	return "sports-team:" + sportsHash(strings.ToLower(strings.TrimSpace(team.Name+"|"+team.Abbreviation)))
}

func sportsHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func cloneSportsEvents(events []SportsEvent) []SportsEvent {
	clone := make([]SportsEvent, len(events))
	for index, event := range events {
		if event.Spread != nil {
			spread := *event.Spread
			event.Spread = &spread
		}
		event.Channels = append([]SportsChannelMatch(nil), event.Channels...)
		event.Ranking.Signals = append([]SportsRankingSignal(nil), event.Ranking.Signals...)
		event.MatchDiagnostics = append([]SportsMatchDiagnostic(nil), event.MatchDiagnostics...)
		event.Artwork = cloneSportsArtwork(event.Artwork)
		clone[index] = event
	}
	return clone
}

func cloneSportsArtwork(artwork *SportsArtwork) *SportsArtwork {
	if artwork == nil {
		return nil
	}
	clone := *artwork
	cloneImage := func(image *SportsImage) *SportsImage {
		if image == nil {
			return nil
		}
		copy := *image
		return &copy
	}
	clone.Poster = cloneImage(artwork.Poster)
	clone.Backdrop = cloneImage(artwork.Backdrop)
	clone.Logo = cloneImage(artwork.Logo)
	clone.Banner = cloneImage(artwork.Banner)
	clone.Thumbnail = cloneImage(artwork.Thumbnail)
	return &clone
}

func sortedBoolKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if value {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

type sportsTerm struct {
	Text         string
	Reason       string
	Weight       int
	TeamName     bool
	Abbreviation bool
	Weak         bool
}

type sportsIndexedChannel struct {
	Channel      model.Channel
	CategoryName string
	ChannelText  string
	CategoryText string
	RawText      string
	Segments     []string
	Programs     []sportsIndexedProgram
}

type sportsIndexedProgram struct {
	Program  model.Program
	Text     string
	Segments []string
}

type sportsChannelIndex struct {
	Channels []sportsIndexedChannel
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
		channels = append(channels, sportsIndexedChannel{
			Channel:      channel,
			CategoryName: categoryName,
			ChannelText:  normalizeMatchText(strings.Join([]string{channel.Name, channel.Number}, " ")),
			CategoryText: normalizeMatchText(strings.Join([]string{categoryName, channel.CategoryName}, " ")),
			RawText:      strings.Join([]string{channel.Name, channel.Number, categoryName, channel.CategoryName}, " "),
			Segments:     sportsMatchSegments(channel.Name, categoryName, channel.CategoryName),
			Programs:     programsByChannel[channel.ID],
		})
	}
	return sportsChannelIndex{Channels: channels}
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
	terms := sportsMatchTerms(event)
	if len(terms) == 0 {
		return []SportsChannelMatch{}, []SportsMatchDiagnostic{}
	}
	matches := make([]SportsChannelMatch, 0)
	diagnostics := make([]SportsMatchDiagnostic, 0)
	for _, indexed := range index.Channels {
		if ctx.Err() != nil {
			break
		}
		result := scoreIndexedSportsChannelResult(indexed, event, terms)
		score, reason := result.Score, result.Reason
		if score < sportsChannelMinimumScore {
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
			Reason:       reason,
			Evidence:     result.Evidence,
			Confidence:   result.Confidence,
			Score:        score,
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
			if normalizeMatchText(existing.Text) == normalized {
				return
			}
		}
		terms = append(terms, sportsTerm{Text: text, Reason: reason, Weight: weight, TeamName: teamName, Abbreviation: abbreviation, Weak: weak})
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

func scoreSportsChannel(channel model.Channel, categoryName string, programs []model.Program, event SportsEvent, terms []sportsTerm) (int, string) {
	indexedPrograms := make([]sportsIndexedProgram, 0, len(programs))
	for _, program := range programs {
		indexedPrograms = append(indexedPrograms, sportsIndexedProgram{
			Program:  program,
			Text:     normalizeMatchText(strings.Join([]string{program.Title, program.Summary}, " ")),
			Segments: sportsMatchSegments(program.Title, program.Summary),
		})
	}
	return scoreIndexedSportsChannel(sportsIndexedChannel{
		Channel:      channel,
		CategoryName: firstNonEmpty(categoryName, channel.CategoryName),
		ChannelText:  normalizeMatchText(strings.Join([]string{channel.Name, channel.Number}, " ")),
		CategoryText: normalizeMatchText(strings.Join([]string{categoryName, channel.CategoryName}, " ")),
		RawText:      strings.Join([]string{channel.Name, channel.Number, categoryName, channel.CategoryName}, " "),
		Segments:     sportsMatchSegments(channel.Name, categoryName, channel.CategoryName),
		Programs:     indexedPrograms,
	}, event, terms)
}

func scoreIndexedSportsChannel(channel sportsIndexedChannel, event SportsEvent, terms []sportsTerm) (int, string) {
	result := scoreIndexedSportsChannelResult(channel, event, terms)
	return result.Score, result.Reason
}

type sportsChannelScore struct {
	Score          int
	Reason         string
	Evidence       string
	Confidence     string
	RejectedReason string
}

func scoreIndexedSportsChannelResult(channel sportsIndexedChannel, event SportsEvent, terms []sportsTerm) sportsChannelScore {
	if sportsTextDatedBeforeEvent(channel.RawText, event) {
		return sportsChannelScore{RejectedReason: "Feed is dated before this fixture"}
	}
	score := 0
	structuralMatch := false
	strongGuideMatch := false
	reasons := map[string]bool{}
	channelText := channel.ChannelText
	categoryText := channel.CategoryText
	hasAbbreviationContext := sportsChannelAbbreviationContext(channelText, categoryText, event)
	channelBothSides := sportsSegmentsContainBothSides(channel.Segments, event)
	if !channelBothSides && sportsSegmentsContainSide(channel.Segments, event.Home) && sportsSegmentsContainSide(channel.Segments, event.Away) {
		return sportsChannelScore{RejectedReason: "Teams appear in separate provider-label segments"}
	}
	for _, term := range terms {
		if (term.Abbreviation || term.Weak) && !hasAbbreviationContext {
			continue
		}
		if containsSportsStructuralTerm(channelText, term) {
			score += term.Weight
			structuralMatch = true
			reasons["channel: "+term.Reason] = true
		}
		if containsSportsStructuralTerm(categoryText, term) {
			score += term.Weight / 2
			structuralMatch = true
			reasons["group: "+term.Reason] = true
		}
	}
	for _, program := range channel.Programs {
		if !programNearSportsEvent(program.Program, event) {
			continue
		}
		programText := program.Text
		programBothSides := sportsSegmentsContainBothSides(program.Segments, event)
		if strongSportsGuideMatch(programText, event) && programBothSides {
			strongGuideMatch = true
		}
		for _, term := range terms {
			if containsMatchTerm(programText, term.Text) {
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
	ancillary := sportsAncillaryBroadcast(channel.RawText)
	if ancillary {
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
	} else if ancillary {
		confidence = "low"
	}
	return sportsChannelScore{Score: score, Reason: joinMatchReasons(reasons), Evidence: evidence, Confidence: confidence}
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

func sportsSegmentsContainBothSides(segments []string, event SportsEvent) bool {
	home := []string{event.Home.Name, event.Home.Abbreviation}
	away := []string{event.Away.Name, event.Away.Abbreviation}
	for _, segment := range segments {
		homeMatch := false
		awayMatch := false
		for _, term := range home {
			homeMatch = homeMatch || containsMatchTerm(segment, term)
		}
		for _, term := range away {
			awayMatch = awayMatch || containsMatchTerm(segment, term)
		}
		if homeMatch && awayMatch {
			return true
		}
	}
	return false
}

func sportsSegmentsContainSide(segments []string, team SportsTeam) bool {
	for _, segment := range segments {
		if containsMatchTerm(segment, team.Name) || containsMatchTerm(segment, team.Abbreviation) {
			return true
		}
	}
	return false
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

func sportsTextDatedBeforeEvent(value string, event SportsEvent) bool {
	if event.StartUnix <= 0 {
		return false
	}
	eventDay := time.Unix(event.StartUnix, 0).UTC().Truncate(24 * time.Hour)
	for _, match := range sportsISODatePattern.FindAllStringSubmatch(value, -1) {
		if len(match) == 4 && sportsParsedDateBefore(match[1], match[2], match[3], eventDay) {
			return true
		}
	}
	for _, match := range sportsUSDatePattern.FindAllStringSubmatch(value, -1) {
		if len(match) == 4 && sportsParsedDateBefore(match[3], match[1], match[2], eventDay) {
			return true
		}
	}
	return false
}

func sportsParsedDateBefore(yearText, monthText, dayText string, eventDay time.Time) bool {
	year, yearErr := strconv.Atoi(yearText)
	month, monthErr := strconv.Atoi(monthText)
	day, dayErr := strconv.Atoi(dayText)
	if yearErr != nil || monthErr != nil || dayErr != nil || month < 1 || month > 12 || day < 1 || day > 31 {
		return false
	}
	parsed := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return parsed.Before(eventDay)
}

// Abbreviations such as TEN and EDM are too ambiguous on their own. A channel
// needs to identify the event's league unless its guide explicitly confirms it.
func sportsChannelAbbreviationContext(channelText, categoryText string, event SportsEvent) bool {
	text := channelText + " " + categoryText
	return containsMatchTerm(text, event.LeagueName) || containsMatchTerm(text, event.LeagueID)
}

// Single-word national teams should not match a longer club name merely because
// the country is one word inside it (for example, England vs New England Revolution).
func containsSportsStructuralTerm(text string, term sportsTerm) bool {
	if !containsMatchTerm(text, term.Text) {
		return false
	}
	termText := normalizeMatchText(term.Text)
	if !term.TeamName || strings.Contains(termText, " ") {
		return true
	}
	return text == termText || strings.HasPrefix(text, termText+" ") || strings.HasSuffix(text, " "+termText)
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
	start := event.StartUnix - 6*3600
	end := event.StartUnix + 8*3600
	programStart := program.StartUnix
	programEnd := program.EndUnix
	if programEnd == 0 {
		programEnd = programStart + 2*3600
	}
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

func normalizeMatchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	space := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			space = false
			continue
		}
		if !space {
			builder.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(builder.String())
}

func containsMatchTerm(text, term string) bool {
	term = normalizeMatchText(term)
	if term == "" {
		return false
	}
	return strings.Contains(" "+text+" ", " "+term+" ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func safeSportsImageURL(value string) string {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return ""
	}
	return value
}

type noopSportsProvider struct{}

func (noopSportsProvider) Events(context.Context, time.Time) ([]SportsEvent, error) {
	return []SportsEvent{}, nil
}

func (noopSportsProvider) Source() string {
	return "none"
}
