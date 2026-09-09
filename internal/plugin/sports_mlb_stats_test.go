package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

const mlbCompetition = `{"date":"2026-09-08T01:10Z","competitors":[{"homeAway":"home","score":"6","hits":10,"errors":0,"team":{"id":"19","displayName":"Los Angeles Dodgers"},"linescores":[{"displayValue":"0"},{"displayValue":"6"}]},{"homeAway":"away","score":"3","hits":7,"errors":1,"team":{"id":"17","displayName":"Cincinnati Reds"},"linescores":[{"displayValue":"0"},{"displayValue":"0"},{"displayValue":"3"}]}],"status":{"type":{"state":"post","detail":"Final","completed":true}}}`

func mlbFixtureEvent() SportsEvent {
	return SportsEvent{ID: "mlb-game", LeagueID: "mlb", StartUnix: time.Date(2026, 9, 8, 1, 10, 0, 0, time.UTC).Unix(), Home: SportsTeam{Name: "Los Angeles Dodgers"}, Away: SportsTeam{Name: "Cincinnati Reds"}}
}

func TestMLBStatsFetchAndCache(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/scoreboard":
			if r.URL.Query().Get("groups") != "" || r.URL.Query().Get("dates") != "20260907-20260908" {
				t.Error("MLB must use the game date window without football groups")
			}
			_, _ = w.Write([]byte(`{"events":[{"id":"401816849","competitions":[` + mlbCompetition + `]}]}`))
		case "/summary":
			if r.URL.Query().Get("event") != "401816849" {
				t.Error("wrong MLB game")
			}
			_, _ = w.Write([]byte(`{"header":{"id":"401816849","competitions":[` + mlbCompetition + `]},"boxscore":{"teams":[{"team":{"displayName":"Los Angeles Dodgers"},"statistics":[{"name":"batting","stats":[{"name":"walks","displayValue":"5"}]},{"name":"pitching","stats":[{"name":"walks","displayValue":"99"}]}]},{"team":{"displayName":"Cincinnati Reds"},"statistics":[{"name":"batting","stats":[{"name":"walks","displayValue":"0"}]}]}]},"plays":[{"text":"Last out"},{"text":" "}]}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	cache := footballStatsCache{baseURL: server.URL, client: server.Client()}
	value := cache.load(context.Background(), mlbFixtureEvent())
	if !value.Available || value.Live || !value.Completed || value.HomeScore != "6" || value.AwayScore != "3" || value.LastPlay != "Last out" || !strings.Contains(value.SourceURL, "/mlb/boxscore/") {
		t.Fatalf("incorrect final MLB stats: %+v", value)
	}
	if len(value.Rows) != 4 || value.Rows[2].Home != "0" || value.Rows[3].Home != "5" || value.Rows[3].Away != "0" {
		t.Fatalf("retain zero errors/walks and batting orientation: %+v", value.Rows)
	}
	if len(value.Innings) != 3 || value.Innings[0].Home != "0" || value.Innings[2].Home != "" || value.Innings[2].Away != "3" {
		t.Fatalf("unplayed innings must remain blank: %+v", value.Innings)
	}
	_ = cache.load(context.Background(), mlbFixtureEvent())
	if requests != 2 {
		t.Fatalf("expected two cached source requests, got %d", requests)
	}
}

func TestMLBStatsIdentityAndGamePhase(t *testing.T) {
	var summary espnStatsSummary
	if err := json.Unmarshal([]byte(`{"header":{"competitions":[`+mlbCompetition+`]}}`), &summary); err != nil {
		t.Fatal(err)
	}
	event := mlbFixtureEvent()
	competition := summary.Header.Competitions[0]
	if !espnStatsMatches(event, competition) || sportsStatsLeaguePath(event) != "baseball/mlb" {
		t.Fatal("MLB matchup must resolve")
	}
	wrong := event
	wrong.Home, wrong.Away = event.Away, event.Home
	if espnStatsMatches(wrong, competition) {
		t.Fatal("reversed matchup must not resolve")
	}
	wrong = event
	wrong.StartUnix -= 24 * 3600
	if espnStatsMatches(wrong, competition) {
		t.Fatal("another day's game must not resolve")
	}
	summary.Header.Competitions[0].Status.Type.Completed = false
	summary.Header.Competitions[0].Status.Type.State = "pre"
	if espnGameStats(event, summary).Available {
		t.Fatal("pregame zero totals are not live stats")
	}
	summary.Header.Competitions[0].Status.Type.State = "in"
	if value := espnGameStats(event, summary); !value.Available || !value.Live || value.Completed {
		t.Fatal("in-progress MLB totals must be available")
	}
}

func TestMLBStatsRejectAmbiguousDoubleheader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/scoreboard" {
			t.Error("must not fetch an ambiguous doubleheader")
		}
		_, _ = w.Write([]byte(`{"events":[{"id":"1","competitions":[` + mlbCompetition + `]},{"id":"2","competitions":[` + mlbCompetition + `]}]}`))
	}))
	defer server.Close()
	cache := footballStatsCache{baseURL: server.URL, client: server.Client()}
	if cache.load(context.Background(), mlbFixtureEvent()).Available {
		t.Fatal("ambiguous doubleheaders must remain unavailable")
	}
}

func TestSportsGuideLiveTitleStatus(t *testing.T) {
	now := time.Date(2026, 9, 8, 1, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		title  string
		start  int64
		end    int64
		status string
	}{
		{"MLB: Dodgers vs Reds LIVE", -60, 60, "live"},
		{"MLB: Dodgers vs Reds ᴸᶦᵛᵉ", -60, 60, "live"},
		{"MLB: Dodgers vs Reds ᴺᵉʷ", -60, 60, "airing"},
		{"MLB: Dodgers vs Reds LIVE Replay", -60, 60, "replay"},
		{"MLB: Dodgers vs Reds LIVE", -120, -60, "ended"},
		{"MLB: Dodgers vs Reds LIVE", 60, 120, "scheduled"},
	} {
		t.Run(tc.title+tc.status, func(t *testing.T) {
			_, _, status, _ := guideSportsBroadcastStatus(model.Program{Title: tc.title, StartUnix: now.Unix() + tc.start}, now.Unix()+tc.end, now)
			if status != tc.status {
				t.Fatalf("got %s, want %s", status, tc.status)
			}
		})
	}
}
