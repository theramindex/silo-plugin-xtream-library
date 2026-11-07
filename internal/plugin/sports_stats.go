package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// Stats are fetched only for an opened game, independently of guide matching.
type footballStatsCache struct {
	mu      sync.Mutex
	entries map[string]SportsGameStats
	events  map[string]SportsEvent
	// flights dedupes concurrent ESPN loads for the same game; mu is never
	// held across the network call.
	flights sportsFlightGroup
	baseURL string
	client  *http.Client
}

const (
	sportsStatsFreshSeconds = 30
	sportsStatsFetchTimeout = 10 * time.Second
)

type SportsPlayerBoxScore struct {
	Side     string                 `json:"side"`
	Category string                 `json:"category"`
	Columns  []string               `json:"columns"`
	Players  []SportsBoxScorePlayer `json:"players"`
}
type SportsBoxScorePlayer struct {
	Name       string   `json:"name"`
	Headshot   string   `json:"headshot,omitempty"`
	Jersey     string   `json:"jersey,omitempty"`
	Position   string   `json:"position,omitempty"`
	Starter    bool     `json:"starter"`
	DidNotPlay bool     `json:"didNotPlay"`
	Stats      []string `json:"stats"`
}
type espnPlayerBoxScore struct {
	Team       espnStatsTeam `json:"team"`
	Statistics []struct {
		Name     string   `json:"name"`
		Text     string   `json:"text"`
		Labels   []string `json:"labels"`
		Athletes []struct {
			Athlete struct {
				DisplayName string `json:"displayName"`
				Headshot    any    `json:"headshot"`
				Jersey      string `json:"jersey"`
				Position    struct {
					Abbreviation string `json:"abbreviation"`
				} `json:"position"`
			} `json:"athlete"`
			Starter    bool     `json:"starter"`
			DidNotPlay bool     `json:"didNotPlay"`
			Stats      []string `json:"stats"`
		} `json:"athletes"`
	} `json:"statistics"`
}

type SportsGameStats struct {
	BoxScores     []SportsPlayerBoxScore `json:"boxScores,omitempty"`
	Available     bool                   `json:"available"`
	Message       string                 `json:"message,omitempty"`
	UpdatedAtUnix int64                  `json:"updatedAtUnix"`
	SourceURL     string                 `json:"sourceUrl,omitempty"`
	StatusText    string                 `json:"statusText,omitempty"`
	Live          bool                   `json:"live"`
	Completed     bool                   `json:"completed"`
	HomeScore     string                 `json:"homeScore,omitempty"`
	AwayScore     string                 `json:"awayScore,omitempty"`
	LastPlay      string                 `json:"lastPlay,omitempty"`
	Possession    string                 `json:"possession,omitempty"`
	FieldPosition string                 `json:"fieldPosition,omitempty"`
	Rows          []SportsGameStat       `json:"rows"`
	Innings       []SportsGameInning     `json:"innings,omitempty"`
	Situation     *SportsGameSituation   `json:"situation,omitempty"`
	Probables     []SportsGamePlayer     `json:"probables,omitempty"`
	Leaders       []SportsGameLeader     `json:"leaders,omitempty"`
	Plays         []SportsGamePlay       `json:"plays,omitempty"`
	Videos        []SportsGameVideo      `json:"videos,omitempty"`
}

type SportsGameSituation struct {
	Balls    int               `json:"balls"`
	Strikes  int               `json:"strikes"`
	Outs     int               `json:"outs"`
	OnFirst  bool              `json:"onFirst"`
	OnSecond bool              `json:"onSecond"`
	OnThird  bool              `json:"onThird"`
	Batter   *SportsGamePlayer `json:"batter,omitempty"`
	Pitcher  *SportsGamePlayer `json:"pitcher,omitempty"`
}

type SportsGamePlayer struct {
	Name     string `json:"name"`
	Headshot string `json:"headshot,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Side     string `json:"side,omitempty"`
}

type SportsGameLeader struct {
	Label string `json:"label"`
	Name  string `json:"name"`
	Value string `json:"value"`
	Side  string `json:"side,omitempty"`
	Photo string `json:"photo,omitempty"`
}

type SportsGamePlay struct {
	Text    string `json:"text"`
	Period  string `json:"period,omitempty"`
	Clock   string `json:"clock,omitempty"`
	Scoring bool   `json:"scoring,omitempty"`
}

type SportsGameVideo struct {
	Title     string `json:"title"`
	Thumbnail string `json:"thumbnail,omitempty"`
	URL       string `json:"url"`
	Stream    string `json:"stream,omitempty"`
}

type espnAthleteRef struct {
	Athlete struct {
		DisplayName string `json:"displayName"`
		Headshot    any    `json:"headshot"`
	} `json:"athlete"`
	Summary string `json:"summary"`
}

func espnHeadshot(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		if href, ok := typed["href"].(string); ok {
			return href
		}
	}
	return ""
}

func espnPlayer(ref *espnAthleteRef, side string) *SportsGamePlayer {
	if ref == nil || strings.TrimSpace(ref.Athlete.DisplayName) == "" {
		return nil
	}
	headshot := espnHeadshot(ref.Athlete.Headshot)
	if !strings.HasPrefix(headshot, "https://") {
		headshot = ""
	}
	return &SportsGamePlayer{Name: strings.TrimSpace(ref.Athlete.DisplayName), Headshot: headshot, Summary: strings.TrimSpace(ref.Summary), Side: side}
}

type SportsGameInning struct {
	Number int    `json:"number"`
	Home   string `json:"home"`
	Away   string `json:"away"`
}

type SportsGameStat struct {
	Label string `json:"label"`
	Home  string `json:"home"`
	Away  string `json:"away"`
}

type espnStatsTeam struct {
	ID          string `json:"id"`
	Location    string `json:"location"`
	DisplayName string `json:"displayName"`
}

type espnStatsCompetition struct {
	Situation struct {
		Possession       string          `json:"possession"`
		DownDistanceText string          `json:"downDistanceText"`
		Balls            int             `json:"balls"`
		Strikes          int             `json:"strikes"`
		Outs             int             `json:"outs"`
		OnFirst          bool            `json:"onFirst"`
		OnSecond         bool            `json:"onSecond"`
		OnThird          bool            `json:"onThird"`
		Batter           *espnAthleteRef `json:"batter"`
		Pitcher          *espnAthleteRef `json:"pitcher"`
	} `json:"situation"`
	Date        string `json:"date"`
	Competitors []struct {
		HomeAway   string        `json:"homeAway"`
		Score      string        `json:"score"`
		Team       espnStatsTeam `json:"team"`
		Linescores []struct {
			DisplayValue string `json:"displayValue"`
		} `json:"linescores"`
		Hits      *int `json:"hits"`
		Errors    *int `json:"errors"`
		Probables []struct {
			espnAthleteRef
			Record string `json:"record"`
			// ESPN sends a list on scoreboards and an object in game summaries.
			Statistics json.RawMessage `json:"statistics"`
		} `json:"probables"`
		Leaders []espnLeaderCategory `json:"leaders"`
	} `json:"competitors"`
	Leaders []espnLeaderCategory `json:"leaders"`
	Status  struct {
		Type struct {
			State     string `json:"state"`
			Completed bool   `json:"completed"`
			Detail    string `json:"detail"`
		} `json:"type"`
	} `json:"status"`
}

type espnLeaderCategory struct {
	DisplayName string `json:"displayName"`
	Leaders     []struct {
		DisplayValue string `json:"displayValue"`
		Athlete      struct {
			DisplayName string `json:"displayName"`
			Headshot    any    `json:"headshot"`
		} `json:"athlete"`
		Team struct {
			ID string `json:"id"`
		} `json:"team"`
	} `json:"leaders"`
}

type espnStatsEvent struct {
	ID           string                 `json:"id"`
	Competitions []espnStatsCompetition `json:"competitions"`
}

type espnStatsSummary struct {
	Header   espnStatsEvent `json:"header"`
	Boxscore struct {
		Players []espnPlayerBoxScore `json:"players"`
		Teams   []struct {
			Team       espnStatsTeam `json:"team"`
			Statistics []struct {
				Name         string `json:"name"`
				Label        string `json:"label"`
				DisplayValue string `json:"displayValue"`
				Stats        []struct {
					Name         string `json:"name"`
					DisplayValue string `json:"displayValue"`
				} `json:"stats"`
			} `json:"statistics"`
		} `json:"teams"`
	} `json:"boxscore"`
	Drives struct {
		Current struct {
			Plays []struct {
				Text string `json:"text"`
			} `json:"plays"`
		} `json:"current"`
	} `json:"drives"`
	Plays []struct {
		Text   string `json:"text"`
		Period struct {
			DisplayValue string `json:"displayValue"`
		} `json:"period"`
		Clock struct {
			DisplayValue string `json:"displayValue"`
		} `json:"clock"`
		ScoringPlay bool `json:"scoringPlay"`
		Type        struct {
			Text string `json:"text"`
		} `json:"type"`
	} `json:"plays"`
	Videos []struct {
		Headline  string `json:"headline"`
		Thumbnail string `json:"thumbnail"`
		Links     struct {
			Source struct {
				Href string `json:"href"`
			} `json:"source"`
			Web struct {
				Href string `json:"href"`
			} `json:"web"`
		} `json:"links"`
	} `json:"videos"`
}

func sportsStatsLeaguePath(event SportsEvent) string {
	id, _, _, _ := guideSportsLeague(event.LeagueName)
	paths := map[string]string{"nba": "basketball/nba", "wnba": "basketball/wnba", "nfl": "football/nfl", "college-football": "football/college-football", "mlb": "baseball/mlb", "nhl": "hockey/nhl", "mls": "soccer/usa.1", "premier-league": "soccer/eng.1", "uefa-champions-league": "soccer/uefa.champions"}
	for _, candidate := range []string{event.LeagueID, id} {
		if path := paths[candidate]; path != "" {
			return path
		}
	}
	return ""
}

func sportsStatsSourceURL(leaguePath, eventID string) string {
	id := url.PathEscape(eventID)
	switch {
	case leaguePath == "baseball/mlb":
		return "https://www.espn.com/mlb/boxscore/_/gameId/" + id
	case leaguePath == "football/college-football":
		return "https://www.espn.com/college-football/boxscore/_/gameId/" + id
	case strings.HasPrefix(leaguePath, "soccer/"):
		return "https://www.espn.com/soccer/match/_/gameId/" + id
	default:
		return "https://www.espn.com/" + leaguePath[strings.LastIndex(leaguePath, "/")+1:] + "/game/_/gameId/" + id
	}
}

func (s *HTTPRoutesServer) handleSportsGameStats(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if request.GetMethod() != "" && request.GetMethod() != http.MethodGet {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed"), nil
	}
	id := queryValue(request, "game_stats")
	if event, ok := s.preparedSportsEvent(id); ok {
		if sportsStatsLeaguePath(event) == "" {
			return s.respondJSON(http.StatusOK, SportsGameStats{Message: "Live stats are not available for this competition yet."})
		}
		return s.respondJSON(http.StatusOK, s.sportsStats.load(ctx, event))
	}
	// A broadcast may end before the game does. Keep polling a previously
	// validated fixture, without trusting client-supplied team identities.
	s.sportsStats.mu.Lock()
	event, known := s.sportsStats.events[id]
	s.sportsStats.mu.Unlock()
	if known && time.Now().Unix()-event.StartUnix < 12*3600 {
		return s.respondJSON(http.StatusOK, s.sportsStats.load(ctx, event))
	}
	return textResponse(http.StatusNotFound, "game not found"), nil
}

func (cache *footballStatsCache) load(ctx context.Context, event SportsEvent) SportsGameStats {
	key := event.ID
	cache.mu.Lock()
	if cache.events == nil || len(cache.events) >= 256 {
		cache.events = make(map[string]SportsEvent)
	}
	cache.events[event.ID] = event
	if event.StableID != "" {
		cache.events[event.StableID] = event
	}
	if value, ok := cache.entries[key]; ok && time.Now().Unix()-value.UpdatedAtUnix < sportsStatsFreshSeconds {
		cache.mu.Unlock()
		return value
	}
	cache.mu.Unlock()

	// Pollers of the same game share one ESPN request. It is detached from
	// any one caller so a closed poll does not fail the others.
	value, err := cache.flights.do(ctx, key, func() (any, error) {
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sportsStatsFetchTimeout)
		defer cancel()
		value, err := cache.fetch(fetchCtx, event)
		if err != nil {
			value = SportsGameStats{Message: "Live stats are temporarily unavailable. Retrying shortly."}
		}
		value.UpdatedAtUnix = time.Now().Unix()
		cache.mu.Lock()
		if cache.entries == nil || len(cache.entries) >= 128 {
			cache.entries = make(map[string]SportsGameStats)
		}
		cache.entries[key] = value
		cache.mu.Unlock()
		return value, nil
	})
	if err != nil {
		return SportsGameStats{Message: "Live stats are temporarily unavailable. Retrying shortly."}
	}
	stats, _ := value.(SportsGameStats)
	return stats
}

func (cache *footballStatsCache) get(ctx context.Context, leaguePath, path string, result any) error {
	base := cache.baseURL
	if base == "" {
		base = "https://site.api.espn.com/apis/site/v2/sports/" + leaguePath
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	client := cache.client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("stats source returned %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(result)
}

func (cache *footballStatsCache) fetch(ctx context.Context, event SportsEvent) (SportsGameStats, error) {
	missing := SportsGameStats{Message: "No live box score is available for this game yet."}
	leaguePath := sportsStatsLeaguePath(event)
	if leaguePath == "" {
		return missing, nil
	}
	if event.StartUnix == 0 {
		return missing, nil
	}
	start := time.Unix(event.StartUnix, 0).UTC()
	var board struct {
		Events []espnStatsEvent `json:"events"`
	}
	// ESPN rejects date ranges on scoreboards, so query each day on either side
	// of the start time.
	seenDays := map[string]bool{}
	seenEvents := map[string]bool{}
	for _, day := range []string{start.Add(-12 * time.Hour).Format("20060102"), start.Add(12 * time.Hour).Format("20060102")} {
		if seenDays[day] {
			continue
		}
		seenDays[day] = true
		boardPath := "/scoreboard?limit=1000&dates=" + day
		if leaguePath == "football/college-football" {
			boardPath += "&groups=80"
		}
		if strings.HasSuffix(leaguePath, "college-basketball") {
			boardPath += "&groups=50"
		}
		var dayBoard struct {
			Events []espnStatsEvent `json:"events"`
		}
		if err := cache.get(ctx, leaguePath, boardPath, &dayBoard); err != nil {
			return missing, err
		}
		for _, candidate := range dayBoard.Events {
			if !seenEvents[candidate.ID] {
				seenEvents[candidate.ID] = true
				board.Events = append(board.Events, candidate)
			}
		}
	}
	match := ""
	var matchedCompetition espnStatsCompetition
	for _, candidate := range board.Events {
		if len(candidate.Competitions) != 1 || !espnStatsMatches(event, candidate.Competitions[0]) {
			continue
		}
		if match != "" {
			return missing, nil
		} // Ambiguous fixtures must not share stats.
		match = candidate.ID
		matchedCompetition = candidate.Competitions[0]
	}
	if match == "" {
		return missing, nil
	}
	var summary espnStatsSummary
	if err := cache.get(ctx, leaguePath, "/summary?event="+url.QueryEscape(match), &summary); err != nil {
		return missing, err
	}
	if summary.Header.ID != match || len(summary.Header.Competitions) != 1 || !espnStatsMatches(event, summary.Header.Competitions[0]) {
		return missing, nil
	}
	result := espnGameStats(event, summary)
	result.SourceURL = sportsStatsSourceURL(leaguePath, summary.Header.ID)
	applyESPNCompetitionDetail(&result, matchedCompetition, leaguePath)
	return result, nil
}

func applyESPNCompetitionDetail(result *SportsGameStats, competition espnStatsCompetition, leaguePath string) {
	live := result.Live && !result.Completed && competition.Status.Type.State == "in"
	sideByTeam := map[string]string{}
	for _, side := range competition.Competitors {
		sideByTeam[side.Team.ID] = side.HomeAway
	}
	if live && strings.HasPrefix(leaguePath, "football/") {
		if side := sideByTeam[competition.Situation.Possession]; side != "" && competition.Situation.Possession != "" {
			result.Possession = side
			result.FieldPosition = competition.Situation.DownDistanceText
		}
	}
	if live && leaguePath == "baseball/mlb" {
		situation := competition.Situation
		result.Situation = &SportsGameSituation{
			Balls: situation.Balls, Strikes: situation.Strikes, Outs: situation.Outs,
			OnFirst: situation.OnFirst, OnSecond: situation.OnSecond, OnThird: situation.OnThird,
			Batter: espnPlayer(situation.Batter, ""), Pitcher: espnPlayer(situation.Pitcher, ""),
		}
	}
	if competition.Status.Type.State == "pre" {
		for _, side := range competition.Competitors {
			for _, probable := range side.Probables {
				player := espnPlayer(&probable.espnAthleteRef, side.HomeAway)
				if player == nil {
					continue
				}
				stats := []string{}
				if probable.Record != "" {
					stats = append(stats, probable.Record)
				}
				var statistics []struct {
					Abbreviation string `json:"abbreviation"`
					DisplayValue string `json:"displayValue"`
				}
				_ = json.Unmarshal(probable.Statistics, &statistics)
				for _, stat := range statistics {
					if stat.Abbreviation == "ERA" && stat.DisplayValue != "" {
						stats = append(stats, stat.DisplayValue+" ERA")
					}
				}
				player.Summary = strings.Join(stats, ", ")
				result.Probables = append(result.Probables, *player)
				break
			}
		}
	}
	categories := competition.Leaders
	for _, side := range competition.Competitors {
		categories = append(categories, side.Leaders...)
	}
	seen := map[string]bool{}
	for _, category := range categories {
		for _, leader := range category.Leaders {
			name := strings.TrimSpace(leader.Athlete.DisplayName)
			key := category.DisplayName + "|" + name
			if name == "" || category.DisplayName == "" || seen[key] || len(result.Leaders) >= 8 {
				continue
			}
			seen[key] = true
			photo := espnHeadshot(leader.Athlete.Headshot)
			if !strings.HasPrefix(photo, "https://") {
				photo = ""
			}
			result.Leaders = append(result.Leaders, SportsGameLeader{Label: category.DisplayName, Name: name, Value: strings.TrimSpace(leader.DisplayValue), Side: sideByTeam[leader.Team.ID], Photo: photo})
			break
		}
	}
	if len(result.Leaders) > 0 || result.Situation != nil || len(result.Probables) > 0 {
		result.Available = true
		result.Message = ""
	}
}

func espnStatsTeamMatches(team SportsTeam, candidate espnStatsTeam) bool {
	canonical := func(value string) string {
		return normalizeSportsIdentityText(strings.NewReplacer("'", "", "’", "", "ʻ", "").Replace(value))
	}
	name := canonical(team.Name)
	if name == "" {
		return false
	}
	if id := ncaaTeamLogoIDs[name]; id != "" && candidate.ID == id {
		return true
	}
	return name == canonical(candidate.Location) || name == canonical(candidate.DisplayName)
}

func espnStatsMatches(event SportsEvent, competition espnStatsCompetition) bool {
	start := parseSportarrTime(competition.Date)
	if start == 0 {
		if parsed, err := time.Parse("2006-01-02T15:04Z", competition.Date); err == nil {
			start = parsed.Unix()
		}
	}
	if start == 0 || event.StartUnix == 0 || start-event.StartUnix > 6*3600 || event.StartUnix-start > 6*3600 {
		return false
	}
	home, away := false, false
	for _, side := range competition.Competitors {
		if side.HomeAway == "home" {
			home = espnStatsTeamMatches(event.Home, side.Team)
		}
		if side.HomeAway == "away" {
			away = espnStatsTeamMatches(event.Away, side.Team)
		}
	}
	return home && away
}

func espnGameStats(event SportsEvent, summary espnStatsSummary) SportsGameStats {
	competition := summary.Header.Competitions[0]
	leaguePath := sportsStatsLeaguePath(event)
	result := SportsGameStats{SourceURL: sportsStatsSourceURL(leaguePath, summary.Header.ID), StatusText: competition.Status.Type.Detail, Live: competition.Status.Type.State == "in", Completed: competition.Status.Type.Completed}
	baseball := leaguePath == "baseball/mlb"
	football := strings.HasPrefix(leaguePath, "football/")
	labels := map[string]string{}
	order := []string{}
	for _, side := range competition.Competitors {
		if side.HomeAway == "home" {
			result.HomeScore = side.Score
		}
		if side.HomeAway == "away" {
			result.AwayScore = side.Score
		}
	}
	for _, team := range summary.Boxscore.Players {
		side := ""
		for _, competitor := range competition.Competitors {
			if team.Team.ID != "" && team.Team.ID == competitor.Team.ID && (competitor.HomeAway == "home" || competitor.HomeAway == "away") {
				side = competitor.HomeAway
			}
		}
		if side == "" {
			continue
		}
		for _, category := range team.Statistics {
			if len(result.BoxScores) >= 12 || len(category.Labels) == 0 || len(category.Labels) > 24 {
				continue
			}
			group := SportsPlayerBoxScore{Side: side, Category: category.Text, Columns: category.Labels}
			if group.Category == "" {
				group.Category = category.Name
			}
			for _, player := range category.Athletes {
				if len(group.Players) >= 100 {
					break
				}
				if strings.TrimSpace(player.Athlete.DisplayName) == "" || (!player.DidNotPlay && len(player.Stats) != len(group.Columns)) {
					continue
				}
				photo := espnHeadshot(player.Athlete.Headshot)
				if !strings.HasPrefix(photo, "https://") {
					photo = ""
				}
				group.Players = append(group.Players, SportsBoxScorePlayer{Name: player.Athlete.DisplayName, Headshot: photo, Jersey: player.Athlete.Jersey, Position: player.Athlete.Position.Abbreviation, Starter: player.Starter, DidNotPlay: player.DidNotPlay, Stats: player.Stats})
			}
			if len(group.Players) > 0 {
				result.BoxScores = append(result.BoxScores, group)
			}
		}
	}
	home, away := map[string]string{}, map[string]string{}
	for _, team := range summary.Boxscore.Teams {
		var target map[string]string
		if espnStatsTeamMatches(event.Home, team.Team) {
			target = home
		} else if espnStatsTeamMatches(event.Away, team.Team) {
			target = away
		}
		if target == nil {
			continue
		}
		for _, stat := range team.Statistics {
			if baseball {
				for _, nested := range stat.Stats {
					if stat.Name == "batting" || (stat.Name == "fielding" && nested.Name == "errors") {
						target[nested.Name] = strings.TrimSpace(nested.DisplayValue)
					}
				}
			} else {
				target[stat.Name] = strings.TrimSpace(stat.DisplayValue)
				if _, known := labels[stat.Name]; !known && strings.TrimSpace(stat.Label) != "" {
					labels[stat.Name] = strings.TrimSpace(stat.Label)
					order = append(order, stat.Name)
				}
			}
		}
	}
	rowNames := [][2]string{{"totalYards", "Total yards"}, {"netPassingYards", "Passing yards"}, {"rushingYards", "Rushing yards"}, {"firstDowns", "First downs"}, {"thirdDownEff", "Third down"}, {"fourthDownEff", "Fourth down"}, {"turnovers", "Turnovers"}, {"totalPenaltiesYards", "Penalties–yards"}, {"possessionTime", "Possession"}}
	if !football && !baseball {
		rowNames = [][2]string{}
		for _, name := range order {
			if len(rowNames) >= 10 {
				break
			}
			rowNames = append(rowNames, [2]string{name, labels[name]})
		}
	}
	if baseball {
		rowNames = [][2]string{{"runs", "Runs"}, {"hits", "Hits"}, {"errors", "Errors"}, {"homeRuns", "Home runs"}, {"walks", "Walks"}, {"strikeouts", "Strikeouts"}, {"stolenBases", "Stolen bases"}, {"avg", "Batting average"}, {"onBasePct", "On-base percentage"}}
		for _, side := range competition.Competitors {
			target := away
			if side.HomeAway == "home" {
				target = home
			}
			target["runs"] = side.Score
			if side.Hits != nil {
				target["hits"] = strconv.Itoa(*side.Hits)
			}
			if side.Errors != nil {
				target["errors"] = strconv.Itoa(*side.Errors)
			}
			for i, inning := range side.Linescores {
				for len(result.Innings) <= i {
					result.Innings = append(result.Innings, SportsGameInning{Number: len(result.Innings) + 1})
				}
				if side.HomeAway == "home" {
					result.Innings[i].Home = inning.DisplayValue
				} else {
					result.Innings[i].Away = inning.DisplayValue
				}
			}
		}
		for i := len(summary.Plays) - 1; i >= 0; i-- {
			if text := strings.TrimSpace(summary.Plays[i].Text); text != "" {
				result.LastPlay = text
				break
			}
		}
	}
	for _, stat := range rowNames {
		if home[stat[0]] != "" && away[stat[0]] != "" {
			result.Rows = append(result.Rows, SportsGameStat{Label: stat[1], Home: home[stat[0]], Away: away[stat[0]]})
		}
	}
	plays := summary.Drives.Current.Plays
	if !baseball && len(plays) > 0 {
		result.LastPlay = strings.TrimSpace(plays[len(plays)-1].Text)
	}
	if !baseball && !football && result.LastPlay == "" {
		for i := len(summary.Plays) - 1; i >= 0; i-- {
			if text := strings.TrimSpace(summary.Plays[i].Text); text != "" {
				result.LastPlay = text
				break
			}
		}
	}
	for i := len(summary.Plays) - 1; i >= 0 && len(result.Plays) < 12; i-- {
		play := summary.Plays[i]
		text := strings.TrimSpace(play.Text)
		if text == "" || (baseball && !play.ScoringPlay && play.Type.Text != "Play Result") {
			continue
		}
		result.Plays = append(result.Plays, SportsGamePlay{Text: text, Period: strings.TrimSpace(play.Period.DisplayValue), Clock: strings.TrimSpace(play.Clock.DisplayValue), Scoring: play.ScoringPlay})
	}
	for _, video := range summary.Videos {
		stream := strings.TrimSpace(video.Links.Source.Href)
		if !strings.HasPrefix(stream, "https://") {
			stream = ""
		}
		link := strings.TrimSpace(video.Links.Web.Href)
		if !strings.HasPrefix(link, "https://") {
			link = stream
		}
		title := strings.TrimSpace(video.Headline)
		if title == "" || link == "" || len(result.Videos) >= 8 {
			continue
		}
		thumbnail := video.Thumbnail
		if !strings.HasPrefix(thumbnail, "https://") {
			thumbnail = ""
		}
		result.Videos = append(result.Videos, SportsGameVideo{Title: title, Thumbnail: thumbnail, URL: link, Stream: stream})
	}
	result.Available = len(result.BoxScores) > 0 || (len(result.Rows) > 0 && (!baseball || result.Live || result.Completed)) || len(result.Plays) > 0 || len(result.Videos) > 0
	if !result.Available {
		result.Message = "No live box score is available for this game yet."
	}
	return result
}
