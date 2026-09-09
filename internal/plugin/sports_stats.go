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
	baseURL string
	client  *http.Client
}

type SportsGameStats struct {
	Available     bool               `json:"available"`
	Message       string             `json:"message,omitempty"`
	UpdatedAtUnix int64              `json:"updatedAtUnix"`
	SourceURL     string             `json:"sourceUrl,omitempty"`
	StatusText    string             `json:"statusText,omitempty"`
	Live          bool               `json:"live"`
	Completed     bool               `json:"completed"`
	HomeScore     string             `json:"homeScore,omitempty"`
	AwayScore     string             `json:"awayScore,omitempty"`
	LastPlay      string             `json:"lastPlay,omitempty"`
	Possession    string             `json:"possession,omitempty"`
	FieldPosition string             `json:"fieldPosition,omitempty"`
	Rows          []SportsGameStat   `json:"rows"`
	Innings       []SportsGameInning `json:"innings,omitempty"`
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
		Possession       string `json:"possession"`
		DownDistanceText string `json:"downDistanceText"`
	} `json:"situation"`
	Date        string `json:"date"`
	Competitors []struct {
		HomeAway   string        `json:"homeAway"`
		Score      string        `json:"score"`
		Team       espnStatsTeam `json:"team"`
		Linescores []struct {
			DisplayValue string `json:"displayValue"`
		} `json:"linescores"`
		Hits   *int `json:"hits"`
		Errors *int `json:"errors"`
	} `json:"competitors"`
	Status struct {
		Type struct {
			State     string `json:"state"`
			Completed bool   `json:"completed"`
			Detail    string `json:"detail"`
		} `json:"type"`
	} `json:"status"`
}

type espnStatsEvent struct {
	ID           string                 `json:"id"`
	Competitions []espnStatsCompetition `json:"competitions"`
}

type espnStatsSummary struct {
	Header   espnStatsEvent `json:"header"`
	Boxscore struct {
		Teams []struct {
			Team       espnStatsTeam `json:"team"`
			Statistics []struct {
				Name         string `json:"name"`
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
		Text string `json:"text"`
	} `json:"plays"`
}

func sportsStatsLeaguePath(event SportsEvent) string {
	id, _, _, _ := guideSportsLeague(event.LeagueName)
	if event.LeagueID == "college-football" || id == "college-football" {
		return "football/college-football"
	}
	if event.LeagueID == "mlb" || id == "mlb" {
		return "baseball/mlb"
	}
	return ""
}

func (s *HTTPRoutesServer) handleSportsGameStats(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if request.GetMethod() != "" && request.GetMethod() != http.MethodGet {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed"), nil
	}
	id := queryValue(request, "game_stats")
	for _, event := range s.preparedSportsPayload(false).Events {
		if id != "" && (event.ID == id || event.StableID == id) {
			if sportsStatsLeaguePath(event) == "" {
				return s.respondJSON(http.StatusOK, SportsGameStats{Message: "Live stats are not available for this competition yet."})
			}
			return s.respondJSON(http.StatusOK, s.sportsStats.load(ctx, event))
		}
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
	cache.mu.Lock()
	defer cache.mu.Unlock()
	now := time.Now()
	key := event.ID
	if cache.events == nil || len(cache.events) >= 256 {
		cache.events = make(map[string]SportsEvent)
	}
	cache.events[event.ID] = event
	if event.StableID != "" {
		cache.events[event.StableID] = event
	}
	if value, ok := cache.entries[key]; ok && now.Unix()-value.UpdatedAtUnix < 30 {
		return value
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	value, err := cache.fetch(ctx, event)
	if err != nil {
		value = SportsGameStats{Message: "Live stats are temporarily unavailable. Retrying shortly."}
	}
	value.UpdatedAtUnix = now.Unix()
	if cache.entries == nil || len(cache.entries) >= 128 {
		cache.entries = make(map[string]SportsGameStats)
	}
	cache.entries[key] = value
	return value
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
	dates := start.Add(-12*time.Hour).Format("20060102") + "-" + start.Add(12*time.Hour).Format("20060102")
	var board struct {
		Events []espnStatsEvent `json:"events"`
	}
	boardPath := "/scoreboard?limit=1000&dates=" + dates
	if leaguePath == "football/college-football" {
		boardPath += "&groups=80"
	}
	if err := cache.get(ctx, leaguePath, boardPath, &board); err != nil {
		return missing, err
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
	if leaguePath == "baseball/mlb" {
		return result, nil
	}
	if result.Live && !result.Completed && matchedCompetition.Status.Type.State == "in" {
		for _, side := range matchedCompetition.Competitors {
			if side.Team.ID != "" && side.Team.ID == matchedCompetition.Situation.Possession {
				result.Possession = side.HomeAway
				result.FieldPosition = matchedCompetition.Situation.DownDistanceText
			}
		}
	}
	return result, nil
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
	result := SportsGameStats{SourceURL: "https://www.espn.com/college-football/boxscore/_/gameId/" + url.PathEscape(summary.Header.ID), StatusText: competition.Status.Type.Detail, Live: competition.Status.Type.State == "in", Completed: competition.Status.Type.Completed}
	baseball := sportsStatsLeaguePath(event) == "baseball/mlb"
	if baseball {
		result.SourceURL = "https://www.espn.com/mlb/boxscore/_/gameId/" + url.PathEscape(summary.Header.ID)
	}
	for _, side := range competition.Competitors {
		if side.HomeAway == "home" {
			result.HomeScore = side.Score
		}
		if side.HomeAway == "away" {
			result.AwayScore = side.Score
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
			}
		}
	}
	rowNames := [][2]string{{"totalYards", "Total yards"}, {"netPassingYards", "Passing yards"}, {"rushingYards", "Rushing yards"}, {"firstDowns", "First downs"}, {"thirdDownEff", "Third down"}, {"fourthDownEff", "Fourth down"}, {"turnovers", "Turnovers"}, {"totalPenaltiesYards", "Penalties–yards"}, {"possessionTime", "Possession"}}
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
	result.Available = len(result.Rows) > 0 && (!baseball || result.Live || result.Completed)
	if !result.Available {
		result.Message = "No live box score is available for this game yet."
	}
	return result
}
