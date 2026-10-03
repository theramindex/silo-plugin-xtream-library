package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"google.golang.org/protobuf/types/known/structpb"
)

func guideTestServer() (*cache.Store, *HTTPRoutesServer) {
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeXtream),
		Channels: []model.Channel{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}, {ID: "c", Name: "C"}},
		Programs: []model.Program{
			{ID: "a3", ChannelID: "a", Title: "A3", StartUnix: 3000, EndUnix: 4000},
			{ID: "a1", ChannelID: "a", Title: "A1", StartUnix: 1000, EndUnix: 2000},
			{ID: "a2", ChannelID: "a", Title: "A2", StartUnix: 2000, EndUnix: 3000},
			{ID: "b-long", ChannelID: "b", Title: "B long", StartUnix: 0, EndUnix: 10000},
			{ID: "b2", ChannelID: "b", Title: "B2", StartUnix: 10000, EndUnix: 11000},
			{ID: "c1", ChannelID: "c", Title: "C1", StartUnix: 2500, EndUnix: 2600},
			{ID: "c-point", ChannelID: "c", Title: "C point", StartUnix: 5000},
		},
	}})
	return store, NewHTTPRoutesServer(store)
}

func fetchGuideIDs(t *testing.T, server *HTTPRoutesServer, params map[string]any) []string {
	t.Helper()
	query, err := structpb.NewStruct(params)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/dispatcharr/api/guide", Query: query})
	if err != nil {
		t.Fatalf("guide route: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected 200 for %v, got %d: %s", params, response.GetStatusCode(), response.GetBody())
	}
	var payload GuidePayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal guide: %v", err)
	}
	if payload.Programs == nil {
		t.Fatalf("expected programs array, got null for %v", params)
	}
	ids := make([]string, 0, len(payload.Programs))
	for _, program := range payload.Programs {
		ids = append(ids, program.ID)
	}
	return ids
}

func TestGuideRouteWindowAndChannelFilters(t *testing.T) {
	t.Parallel()

	_, server := guideTestServer()
	cases := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{"full guide sorted by start then id", map[string]any{}, "b-long,a1,a2,c1,a3,c-point,b2"},
		{"legacy single channel", map[string]any{"channel_id": "a"}, "a1,a2,a3"},
		{"channel list", map[string]any{"channel_ids": "c, a"}, "a1,a2,c1,a3,c-point"},
		{"channel_id merged with list", map[string]any{"channel_id": "b", "channel_ids": "c"}, "b-long,c1,c-point,b2"},
		{"window overlaps, half open", map[string]any{"start": "2000", "end": "3000"}, "b-long,a2,c1"},
		{"window catches long earlier program", map[string]any{"channel_ids": "b", "start": "9000", "end": "9500"}, "b-long"},
		{"open-ended start", map[string]any{"start": "4500"}, "b-long,c-point,b2"},
		{"open-ended end", map[string]any{"channel_id": "a", "end": "2001"}, "a1,a2"},
		{"point program inside window", map[string]any{"channel_id": "c", "start": "5000", "end": "5001"}, "c-point"},
		{"unknown channel", map[string]any{"channel_ids": "zzz"}, ""},
	}
	for _, tc := range cases {
		if got := strings.Join(fetchGuideIDs(t, server, tc.params), ","); got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

func TestGuideRouteRejectsInvalidQuery(t *testing.T) {
	t.Parallel()

	_, server := guideTestServer()
	tooMany := make([]string, maxGuideQueryChannels+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("ch%d", index)
	}
	for _, params := range []map[string]any{
		{"start": "abc"},
		{"end": "1.5"},
		{"start": "100", "end": "100"},
		{"start": "200", "end": "100"},
		{"channel_ids": strings.Join(tooMany, ",")},
	} {
		query, _ := structpb.NewStruct(params)
		response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/xtream/api/guide", Query: query})
		if err != nil {
			t.Fatalf("guide route: %v", err)
		}
		if response.GetStatusCode() != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", response.GetStatusCode())
		}
	}
}

func TestGuideIndexIsReusedPerSnapshotVersion(t *testing.T) {
	t.Parallel()

	store, server := guideTestServer()
	first := server.guideIndex.get(store.Current())
	if again := server.guideIndex.get(store.Current()); again != first {
		t.Fatal("expected cached guide index for unchanged snapshot")
	}
	_ = fetchGuideIDs(t, server, map[string]any{})
	if server.guideIndex.get(store.Current()) != first {
		t.Fatal("guide route should reuse the cached index")
	}

	store.ReplacePrograms([]model.Program{
		{ID: "n1", ChannelID: "a", StartUnix: 1000, EndUnix: 2000},
		{ID: "n2", ChannelID: "b", StartUnix: 1000, EndUnix: 2000},
		{ID: "n3", ChannelID: "c", StartUnix: 1000, EndUnix: 2000},
		{ID: "n4", ChannelID: "a", StartUnix: 2000, EndUnix: 3000},
	}, 0)
	rebuilt := server.guideIndex.get(store.Current())
	if rebuilt == first {
		t.Fatal("expected index rebuild after the guide changed")
	}
	if got := strings.Join(fetchGuideIDs(t, server, map[string]any{}), ","); got != "n1,n2,n3,n4" {
		t.Fatalf("expected refreshed guide, got %q", got)
	}
}
