package plugin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

func TestViewerCatalogsHideSourceLabelsAndPreserveCategoryIdentity(t *testing.T) {
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Source: model.LiveTVSource(model.SourceModeXtream),
		Channels: []model.Channel{
			{ID: "xtream:backup:1", CategoryID: "backup:10", CategoryName: "Backup Provider · USA | Sports"},
			{ID: "xtream:other:1", CategoryID: "other:10", CategoryName: "Other Provider · USA | Sports"},
		},
		Content: model.ContentState{
			LiveCategories:   []model.Category{{ID: "backup:10", Name: "Backup Provider · USA | Sports"}, {ID: "other:10", Name: "Other Provider · USA | Sports"}, {ID: "primary:20", Name: "Studio · Classics"}},
			VODCategories:    []model.Category{{ID: "backup:30", Name: "Backup Provider · Movies"}},
			SeriesCategories: []model.Category{{ID: "backup:40", Name: "Backup Provider · Shows"}},
		},
	}})
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings {
		return config.Settings{SourceMode: config.SourceModeXtream, XtreamSources: []config.XtreamSource{
			{ID: "backup", Name: "Backup Provider", Enabled: true, BaseURL: "https://backup.example", Username: "viewer", Password: "test"},
			{ID: "other", Name: "Other Provider", Enabled: true, BaseURL: "https://other.example", Username: "viewer", Password: "test"},
		}}
	})
	for _, endpoint := range []string{"app", "channels", "categories", "vod", "series"} {
		t.Run(endpoint, func(t *testing.T) {
			response, err := server.Handle(t.Context(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/xtream/api/" + endpoint})
			if err != nil || response.GetStatusCode() != http.StatusOK {
				t.Fatalf("catalog response: %v, %v", response, err)
			}
			body := string(response.GetBody())
			if strings.Contains(body, "Backup Provider · ") || strings.Contains(body, "Other Provider · ") {
				t.Fatalf("source label is visible: %s", body)
			}
			if endpoint == "app" || endpoint == "channels" {
				var payload AppPayload
				if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
					t.Fatal(err)
				}
				if len(payload.Channels) != 2 || payload.Channels[0].ID != "xtream:backup:1" || payload.Channels[1].ID != "xtream:other:1" || payload.Channels[0].CategoryName != "USA | Sports" || payload.Channels[1].CategoryName != "USA | Sports" {
					t.Fatalf("channel identity or category changed: %+v", payload.Channels)
				}
				if len(payload.Categories) != 3 || payload.Categories[0].ID != "backup:10" || payload.Categories[1].ID != "other:10" || payload.Categories[2].Name != "Studio · Classics" {
					t.Fatalf("category identity or genuine middle dot changed: %+v", payload.Categories)
				}
			}
		})
	}
	if store.Current().Catalog.Content.LiveCategories[0].Name != "Backup Provider · USA | Sports" {
		t.Fatal("presentation mutated the stored catalog")
	}
}
