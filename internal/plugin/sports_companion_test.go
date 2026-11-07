package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestSportsRouteReturnsOrientedPlayerBoxScore(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/scoreboard" {
			_, _ = w.Write([]byte(`{"events":[{"id":"401860879","competitions":[` + statsFixtureCompetition + `]}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"header":{"id":"401860879","competitions":[` + statsFixtureCompetition + `]},"boxscore":{"players":[{"team":{"id":"21"},"statistics":[{"name":"passing","labels":["C/ATT","YDS","TD"],"athletes":[{"athlete":{"displayName":"Home Quarterback","jersey":"12","position":{"abbreviation":"QB"}},"stats":["10/20","123","0"]},{"athlete":{"displayName":"Reserve"},"didNotPlay":true,"stats":[]}]}]},{"team":{"id":"foreign"},"statistics":[{"labels":["PTS"],"athletes":[{"athlete":{"displayName":"Wrong Game"},"stats":["99"]}]}]}]}}`))
	}))
	defer upstream.Close()
	server := newEnabledSportsTestServer(cache.NewStore())
	server.sportsPrepared = sportsPreparedCache{Ready: true, ExpiresAfter: time.Now().Add(time.Minute), Payload: SportsPayload{Events: []SportsEvent{statsFixtureEvent()}}}
	server.sportsStats.baseURL = upstream.URL
	server.sportsStats.client = upstream.Client()
	query, _ := structpb.NewStruct(map[string]any{"game_stats": "game"})
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/xtream/api/sports", Query: query})
	if err != nil || response.GetStatusCode() != 200 {
		t.Fatalf("stats route: %v %+v", err, response)
	}
	var body struct {
		Available bool
		BoxScores []struct {
			Side    string
			Columns []string
			Players []struct {
				Name       string
				Stats      []string
				DidNotPlay bool
			}
		}
	}
	if err := json.Unmarshal(response.GetBody(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Available || len(body.BoxScores) != 1 {
		t.Fatalf("expected one team's player box score: %s", response.GetBody())
	}
	g := body.BoxScores[0]
	if g.Side != "home" || len(g.Columns) != 3 || len(g.Players) != 2 || g.Players[0].Name != "Home Quarterback" || g.Players[0].Stats[2] != "0" || !g.Players[1].DidNotPlay {
		t.Fatalf("orientation, zero stats and DNP: %s", response.GetBody())
	}
}
