package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestGuideHTTPReadsSQLiteWindowsAfterMemoryIsReleased(t *testing.T) {
	ctx := context.Background()
	storage := cache.NewFileSnapshotStorage(filepath.Join(t.TempDir(), "catalog-snapshot.json"))
	defer storage.Close()
	store := cache.NewStore()
	if err := store.Replace(cache.Snapshot{ConfigKey: "source-one", Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeXtream),
		Channels: []model.Channel{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}},
		Programs: []model.Program{
			{ID: "a1", ChannelID: "a", Title: "Morning News", StartUnix: 1000, EndUnix: 2000},
			{ID: "a2", ChannelID: "a", Title: "Next News", StartUnix: 2000, EndUnix: 3000},
			{ID: "b", ChannelID: "b", Title: "Long Show", StartUnix: 100, EndUnix: 3000},
			{ID: "future", ChannelID: "a", Title: "Later", StartUnix: 4000, EndUnix: 5000},
		},
	}}); err != nil {
		t.Fatal(err)
	}
	server := NewHTTPRoutesServer(store)
	params, _ := structpb.NewStruct(map[string]any{"channel_ids": "b,a", "start": "1500", "end": "2500"})
	request := &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/xtream/api/guide", Query: params}
	// Build the old memory index before migrating. A stale cached index must
	// not hide SQL read failures or keep the full guide alive indefinitely.
	if _, err := server.Handle(ctx, request); err != nil {
		t.Fatal(err)
	}
	store.SetStorage(storage)
	if err := store.Persist(); err != nil {
		t.Fatal(err)
	}
	if current := store.Current(); current.ProgramCount() != 4 || len(current.Catalog.Programs) != 0 {
		t.Fatal("full guide was retained in the published memory snapshot")
	}
	for _, path := range []string{"/xtream/api/guide", "/silo-compat/livetv/v1/programs"} {
		request.Path = path
		response, err := server.Handle(ctx, request)
		if err != nil || response.GetStatusCode() != 200 {
			t.Fatalf("%s: %v %v", path, response, err)
		}
		var body struct {
			Programs []struct {
				ID string `json:"id"`
			} `json:"programs"`
		}
		if err := json.Unmarshal(response.GetBody(), &body); err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, p := range body.Programs {
			ids = append(ids, p.ID)
		}
		if !reflect.DeepEqual(ids, []string{"b", "a1", "a2"}) {
			t.Fatalf("%s guide order/window: %v", path, ids)
		}
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	response, err := server.Handle(ctx, request)
	if err != nil || response.GetStatusCode() != http.StatusServiceUnavailable {
		t.Fatalf("SQL failure was hidden as an empty guide: %v %v", response, err)
	}
}
