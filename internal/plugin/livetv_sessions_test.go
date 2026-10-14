package plugin

import (
	"context"
	"net/http"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"google.golang.org/protobuf/types/known/structpb"
)

// liveTVSessionServer serves one Xtream source with two one-stream playback
// accounts, and one M3U channel, on a clock the test moves.
func liveTVSessionServer(t *testing.T) (*HTTPRoutesServer, *time.Time) {
	t.Helper()
	settings := config.Settings{SourceMode: config.SourceModeXtream, XtreamSources: []config.XtreamSource{{
		ID: "frost", Name: "Frost", BaseURL: "https://frost.example", LiveFormat: "ts", Enabled: true, CatalogAccountID: "a",
		Accounts: []config.XtreamAccount{
			{ID: "a", Username: "first-user", Password: "first-secret", Enabled: true, Catalog: true, Compatible: true, ConnectionLimit: 1},
			{ID: "b", Username: "second-user", Password: "second-secret", Enabled: true, Compatible: true, ConnectionLimit: 1},
		},
	}}}
	server := liveTVContractServer(model.SourceModeXtream, []model.Channel{
		{ID: "xtream:frost:101", Name: "News"},
		{ID: "m3u:local", Name: "Local", StreamURL: "https://local.example/1.ts"},
	}, settings)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	server.accountPool = newAccountPool(func() time.Time { return now })
	return server, &now
}

func liveTVSessionStream(t *testing.T, server *HTTPRoutesServer, channelID, sessionID string) *pluginv1.HandleHTTPResponse {
	t.Helper()
	return liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", map[string]any{"channel_id": channelID, "session_id": sessionID})
}

func liveTVSessionCall(t *testing.T, server *HTTPRoutesServer, route, sessionID string) *pluginv1.HandleHTTPResponse {
	t.Helper()
	query, err := structpb.NewStruct(map[string]any{"session_id": sessionID})
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodPost, Path: "/silo-compat/livetv/v1/session/" + route, Query: query})
	if err != nil {
		t.Fatal(err)
	}
	return response
}

// Each session leases its own account, the same session keeps its account,
// and a third session is refused until one stops.
func TestLiveTVSessionsLeasePlaybackAccounts(t *testing.T) {
	t.Parallel()
	server, _ := liveTVSessionServer(t)

	first := decodeLiveTVStream(t, liveTVSessionStream(t, server, "xtream:frost:101", "play-1"))
	second := decodeLiveTVStream(t, liveTVSessionStream(t, server, "xtream:frost:101", "play-2"))
	if !first.Leased || !second.Leased || first.URL == second.URL {
		t.Fatalf("streams = %+v and %+v, want two leased streams on different accounts", first, second)
	}
	if again := decodeLiveTVStream(t, liveTVSessionStream(t, server, "xtream:frost:101", "play-1")); again.URL != first.URL {
		t.Fatalf("the same session moved accounts: %s then %s", first.URL, again.URL)
	}

	full := liveTVSessionStream(t, server, "xtream:frost:101", "play-3")
	if full.GetStatusCode() != http.StatusConflict || decodeLiveTVCode(t, full) != "connection_limit" {
		t.Fatalf("third session = %d %s, want 409 connection_limit", full.GetStatusCode(), full.GetBody())
	}
	if usage := server.accountPool.Usage("frost"); usage["a"] != 1 || usage["b"] != 1 {
		t.Fatalf("usage = %v", usage)
	}

	if response := liveTVSessionCall(t, server, "stop", "play-1"); response.GetStatusCode() != http.StatusOK {
		t.Fatalf("stop = %d", response.GetStatusCode())
	}
	if third := decodeLiveTVStream(t, liveTVSessionStream(t, server, "xtream:frost:101", "play-3")); third.URL != first.URL {
		t.Fatalf("third session got %s, want the freed account's %s", third.URL, first.URL)
	}
}

// A heartbeat keeps a lease past the window; without one the lease lapses and
// the heartbeat says so.
func TestLiveTVSessionHeartbeat(t *testing.T) {
	t.Parallel()
	server, now := liveTVSessionServer(t)
	liveTVSessionStream(t, server, "xtream:frost:101", "play-1")

	*now = now.Add(60 * time.Second)
	if response := liveTVSessionCall(t, server, "heartbeat", "play-1"); response.GetStatusCode() != http.StatusOK {
		t.Fatalf("heartbeat = %d %s", response.GetStatusCode(), response.GetBody())
	}
	*now = now.Add(60 * time.Second)
	if usage := server.accountPool.Usage("frost"); usage["a"]+usage["b"] != 1 {
		t.Fatalf("lease lapsed despite a heartbeat: %v", usage)
	}

	*now = now.Add(accountLeaseTTL + time.Second)
	response := liveTVSessionCall(t, server, "heartbeat", "play-1")
	if response.GetStatusCode() != http.StatusNotFound || decodeLiveTVCode(t, response) != "session_not_found" {
		t.Fatalf("late heartbeat = %d %s, want 404 session_not_found", response.GetStatusCode(), response.GetBody())
	}
	if response := liveTVSessionCall(t, server, "stop", "never-started"); response.GetStatusCode() != http.StatusOK {
		t.Fatalf("stop of an unknown session = %d", response.GetStatusCode())
	}
}

// Channels outside the account pool, requests without a session, and refused
// formats hold nothing.
func TestLiveTVSessionsHoldNothingElse(t *testing.T) {
	t.Parallel()
	server, _ := liveTVSessionServer(t)

	if local := decodeLiveTVStream(t, liveTVSessionStream(t, server, "m3u:local", "play-1")); local.Leased {
		t.Fatalf("m3u stream = %+v, want unleased", local)
	}
	if plain := decodeLiveTVStream(t, liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", map[string]any{"channel_id": "xtream:frost:101"})); plain.Leased {
		t.Fatalf("stream without a session = %+v, want unleased", plain)
	}
	refused := liveTVContractGet(t, server, "/silo-compat/livetv/v1/stream", map[string]any{"channel_id": "m3u:local", "format": "hls", "session_id": "play-2"})
	if refused.GetStatusCode() != http.StatusUnprocessableEntity {
		t.Fatalf("hls of a ts channel = %d", refused.GetStatusCode())
	}
	if usage := server.accountPool.Usage("frost"); len(usage) != 0 {
		t.Fatalf("usage = %v, want none", usage)
	}

	// The web app's watch sessions share the pool with Silo's plays.
	server.accountPool.Lease("web-watch", server.settingsProvider().EffectiveXtreamSources()[0])
	server.accountPool.Lease("web-watch-2", server.settingsProvider().EffectiveXtreamSources()[0])
	if full := liveTVSessionStream(t, server, "xtream:frost:101", "play-3"); full.GetStatusCode() != http.StatusConflict {
		t.Fatalf("stream with every account watched in the web app = %d, want 409", full.GetStatusCode())
	}
}
