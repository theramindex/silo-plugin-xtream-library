package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestGeneratedSportsGuidePersistsWithoutReplacingOrFeedingBackIntoProviderEPG(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalog.json")
	now := time.Now().UTC().Truncate(time.Minute)
	settings := json.RawMessage(`{"sportsEnabled":true,"sportsGuide":{"enabled":true}}`)
	storage := cache.NewFileSnapshotStorage(path)
	store := cache.NewStore()
	store.SetStorage(storage)
	store.SetAdminSettings(settings)
	snapshot := cache.Snapshot{ConfigKey: "source-a", Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeXtream),
		Channels: []model.Channel{{ID: "game", Name: "NBA | Boston Celtics @ New York Knicks", CategoryID: "nba", CategoryName: "NBA"}},
		Programs: []model.Program{{ID: "provider", ChannelID: "game", Title: "Provider coverage", StartUnix: now.Add(90 * time.Minute).Unix(), EndUnix: now.Add(2 * time.Hour).Unix()}},
	}}
	if err := store.Replace(snapshot); err != nil {
		t.Fatal(err)
	}
	server := NewHTTPRoutesServer(store)
	server.sportsProvider = staticSportsProvider{events: []SportsEvent{{ID: "espn:game", ProviderSource: "ESPN", LeagueID: "nba", LeagueName: "NBA", Name: "Boston Celtics at New York Knicks", Home: SportsTeam{Name: "New York Knicks"}, Away: SportsTeam{Name: "Boston Celtics"}, StartUnix: now.Add(time.Hour).Unix()}}}
	server.rebuildSportsPayload(false)
	read := func(server *HTTPRoutesServer, path string) []model.Program {
		t.Helper()
		params, _ := structpb.NewStruct(map[string]any{"channel_ids": "game", "start": fmt.Sprint(now.Unix()), "end": fmt.Sprint(now.Add(5 * time.Hour).Unix())})
		response, err := server.Handle(ctx, &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: path, Query: params})
		if err != nil || response.GetStatusCode() != 200 {
			t.Fatalf("guide response: %v %v", response, err)
		}
		var body struct {
			Programs []model.Program `json:"programs"`
		}
		if err := json.Unmarshal(response.GetBody(), &body); err != nil {
			t.Fatal(err)
		}
		generated := 0
		provider := 0
		for _, p := range body.Programs {
			if p.Generated != nil {
				generated++
				if p.Generated.Provider != "ESPN" || !p.Generated.Estimated {
					t.Fatalf("missing provenance: %+v", p)
				}
				if p.StartUnix < snapshot.Catalog.Programs[0].EndUnix && p.EndUnix > snapshot.Catalog.Programs[0].StartUnix {
					t.Fatal("generated entry overlaps provider coverage")
				}
			} else if p.ID == "provider" {
				provider++
			}
		}
		if generated != 4 || provider != 1 {
			t.Fatalf("expected three phases with game split around provider listing: %+v", body.Programs)
		}
		return body.Programs
	}
	read(server, "/xtream/api/guide")
	if raw, err := store.Current().WithPrograms(ctx); err != nil || len(raw.Catalog.Programs) != 1 || raw.Catalog.Programs[0].Generated != nil {
		t.Fatalf("generated guide contaminated provider input: %+v %v", raw, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	storage = cache.NewFileSnapshotStorage(path)
	defer storage.Close()
	restored, found, err := storage.Load()
	if err != nil || !found {
		t.Fatalf("restart: %v %v", found, err)
	}
	store = cache.NewStore()
	defer store.Close()
	if err := store.Replace(restored); err != nil {
		t.Fatal(err)
	}
	store.SetStorage(storage)
	store.SetAdminSettings(settings)
	server = NewHTTPRoutesServer(store)
	server.sportsProvider = staticSportsProvider{err: errors.New("schedule provider offline")}
	server.rebuildSportsPayload(false)
	read(server, "/silo-compat/livetv/v1/programs")
	// Policy changes hide the old overlay immediately, even before a rebuild.
	store.SetAdminSettings(json.RawMessage(`{"sportsEnabled":true,"sportsGuide":{"enabled":false}}`))
	params, _ := structpb.NewStruct(map[string]any{"channel_ids": "game", "start": fmt.Sprint(now.Unix()), "end": fmt.Sprint(now.Add(5 * time.Hour).Unix())})
	response, _ := server.Handle(ctx, &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/xtream/api/guide", Query: params})
	var body struct{ Programs []model.Program }
	_ = json.Unmarshal(response.GetBody(), &body)
	if len(body.Programs) != 1 || body.Programs[0].ID != "provider" {
		t.Fatalf("disabled policy still served generated entries: %s", response.GetBody())
	}
}
