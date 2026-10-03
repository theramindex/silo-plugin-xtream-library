package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
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
	// Matching and enrichment get their own deadlines, so a slow Sportarr
	// fetch cannot use up the time channel matching needs.
	sportsMatchTimeout  = 20 * time.Second
	sportsEnrichTimeout = 10 * time.Second
	// An incomplete rebuild is retried this soon instead of after the full TTL.
	sportsIncompleteRetry = 20 * time.Second
)

// sportsBuildBudget holds the per-phase deadlines of one payload rebuild.
type sportsBuildBudget struct {
	Provider, Match, Enrich time.Duration
}

func (budget sportsBuildBudget) withDefaults() sportsBuildBudget {
	if budget.Provider <= 0 {
		budget.Provider = sportsProviderFetchTimeout
	}
	if budget.Match <= 0 {
		budget.Match = sportsMatchTimeout
	}
	if budget.Enrich <= 0 {
		budget.Enrich = sportsEnrichTimeout
	}
	return budget
}

// total bounds a whole rebuild: every phase plus slack for sorting and ranking.
func (budget sportsBuildBudget) total() time.Duration {
	return budget.Provider + budget.Match + budget.Enrich + 6*time.Second
}

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
	// Incomplete marks a cached payload whose channel matching was cut short.
	Incomplete bool
	// budget overrides the rebuild phase deadlines (tests).
	budget sportsBuildBudget
}

type SportsPayload struct {
	UpdatedAtUnix int64          `json:"updatedAtUnix"`
	Source        string         `json:"source"`
	Leagues       []SportsLeague `json:"leagues"`
	Events        []SportsEvent  `json:"events"`
	FavoriteTeams []string       `json:"favoriteTeams"`
	Refreshing    bool           `json:"refreshing,omitempty"`
	Error         string         `json:"error,omitempty"`
	// Incomplete is set when channel matching ran out of time; some events may
	// be missing channels until the next rebuild.
	Incomplete bool `json:"incomplete,omitempty"`
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
	// FollowIDs lists identities saved follows may use, since guide-only and
	// provider copies of the same team hash to different IDs.
	FollowIDs []string `json:"followIds,omitempty"`
	// LegacyIDs lists IDs earlier builds emitted for other spellings of this
	// team (merged Sportarr and guide copies, short guide names). It never
	// repeats id or followIds.
	LegacyIDs []string `json:"legacyIds,omitempty"`
	// Names and IDs of copies merged into this identity.
	aliasNames []string
	aliasIDs   []string
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
		strings.ToLower(strings.TrimSpace(sportsTeamIdentityKey(event.Away))),
		strings.ToLower(strings.TrimSpace(sportsTeamIdentityKey(event.Home))),
		strings.ToLower(strings.TrimSpace(event.Name)),
	}
	return "sports-event:" + sportsHash(strings.Join(parts, "|"))
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
