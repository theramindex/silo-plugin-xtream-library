package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/cache"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
)

func TestHTTPRoutesServerEventsDetectsGuidePrograms(t *testing.T) {
	t.Parallel()

	start := time.Now().Add(24 * time.Hour).Unix()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source: model.LiveTVSource(model.SourceModeXtream),
			Channels: []model.Channel{
				{ID: "ch:abc", Name: "ABC East", CategoryID: "abc", CategoryName: "US | Locals | ABC"},
				{ID: "ch:news", Name: "News Now", CategoryID: "news", CategoryName: "News"},
			},
			Programs: []model.Program{
				{ID: "p:oscars", ChannelID: "ch:abc", Title: "The Oscars", Summary: "Academy Awards ceremony", StartUnix: start, EndUnix: start + 3*3600},
				{ID: "p:news", ChannelID: "ch:news", Title: "Evening News", StartUnix: start, EndUnix: start + 3600},
			},
			Content: model.ContentState{
				LiveCategories: []model.Category{
					{ID: "abc", Name: "US | Locals | ABC", Kind: "live"},
					{ID: "news", Name: "News", Kind: "live"},
				},
			},
		},
	})
	server := NewHTTPRoutesServer(store)

	payload := fetchEventsPayload(t, server)
	if payload.Source != "epg" || len(payload.Events) != 1 {
		t.Fatalf("unexpected events payload: %+v", payload)
	}
	event := payload.Events[0]
	if event.CategoryID != "awards" || event.Keyword == "" {
		t.Fatalf("expected awards event with matched keyword, got %+v", event)
	}
	assertBroadcastEventMatch(t, event.Channels, "ch:abc")
	assertNoBroadcastEventMatch(t, event.Channels, "ch:news")
}

func TestHTTPRoutesServerEventsUsesAdminKeywordRules(t *testing.T) {
	t.Parallel()

	start := time.Now().Add(24 * time.Hour).Unix()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Channels: []model.Channel{{ID: "ch:local", Name: "Local 5", CategoryID: "local", CategoryName: "US | Locals"}},
			Programs: []model.Program{{
				ID:        "p:town",
				ChannelID: "ch:local",
				Title:     "City Council Special",
				StartUnix: start,
				EndUnix:   start + 3600,
			}},
			Content: model.ContentState{
				LiveCategories: []model.Category{{ID: "local", Name: "US | Locals", Kind: "live"}},
			},
		},
	})
	store.SetAdminSettings(json.RawMessage(`{"eventKeywords":[{"categoryId":"civic","categoryName":"Civic","keywords":["City Council Special"]}]}`))
	server := NewHTTPRoutesServer(store)

	payload := fetchEventsPayload(t, server)
	if len(payload.Events) != 1 {
		t.Fatalf("expected custom keyword event, got %+v", payload)
	}
	event := payload.Events[0]
	if event.CategoryID != "civic" || event.Keyword != "City Council Special" {
		t.Fatalf("expected custom civic event, got %+v", event)
	}
	assertBroadcastEventMatch(t, event.Channels, "ch:local")
}

func TestHTTPRoutesServerEventsExcludesEventSeriesStudioPrograms(t *testing.T) {
	t.Parallel()

	start := time.Now().Add(24 * time.Hour).Unix()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{
			{ID: "ch:golf", Name: "Golf Network", CategoryID: "golf", CategoryName: "Sports | Golf"},
			{ID: "ch:studio", Name: "Sports Studio", CategoryID: "studio", CategoryName: "Sports"},
		},
		Programs: []model.Program{
			{ID: "p:coverage", ChannelID: "ch:golf", Title: "PGA Tour: Second Round", Summary: "Live tournament coverage", StartUnix: start, EndUnix: start + 4*3600},
			{ID: "p:studio", ChannelID: "ch:studio", Title: "Golf Central", Summary: "PGA Tour highlights and recap", StartUnix: start, EndUnix: start + 3600},
		},
		Content: model.ContentState{LiveCategories: []model.Category{
			{ID: "golf", Name: "Sports | Golf", Kind: "live"},
			{ID: "studio", Name: "Sports", Kind: "live"},
		}},
	}})

	payload := fetchEventsPayload(t, NewHTTPRoutesServer(store))
	if len(payload.Events) != 1 {
		t.Fatalf("expected one tournament event after exclusions, got %+v", payload.Events)
	}
	event := payload.Events[0]
	if !event.EventSeries || event.CategoryID != "golf" {
		t.Fatalf("expected golf event series, got %+v", event)
	}
	assertBroadcastEventMatch(t, event.Channels, "ch:golf")
	assertNoBroadcastEventMatch(t, event.Channels, "ch:studio")
}

func TestHTTPRoutesServerEventsGroupsEventSeriesBroadcastWindows(t *testing.T) {
	t.Parallel()

	day := time.Now().UTC().AddDate(0, 0, 2)
	start := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.UTC).Unix()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{
			{ID: "ch:one", Name: "Golf One", CategoryID: "golf", CategoryName: "Sports | Golf"},
			{ID: "ch:two", Name: "Golf Two", CategoryID: "golf", CategoryName: "Sports | Golf"},
		},
		Programs: []model.Program{
			{ID: "p:late", ChannelID: "ch:one", Title: "PGA Tour Championship", StartUnix: start + 4*3600, EndUnix: start + 6*3600},
			{ID: "p:early-two", ChannelID: "ch:two", Title: "PGA Tour Championship", StartUnix: start + 30*60, EndUnix: start + 2*3600},
			{ID: "p:early-one", ChannelID: "ch:one", Title: "PGA Tour Championship", StartUnix: start, EndUnix: start + 2*3600},
		},
		Content: model.ContentState{LiveCategories: []model.Category{{ID: "golf", Name: "Sports | Golf", Kind: "live"}}},
	}})

	payload := fetchEventsPayload(t, NewHTTPRoutesServer(store))
	if len(payload.Events) != 1 {
		t.Fatalf("expected one event-series card, got %+v", payload.Events)
	}
	event := payload.Events[0]
	if len(event.Windows) != 2 {
		t.Fatalf("expected two coverage windows, got %+v", event.Windows)
	}
	if event.Windows[0].StartUnix != start || event.Windows[1].StartUnix != start+4*3600 {
		t.Fatalf("expected deterministic window order, got %+v", event.Windows)
	}
	if len(event.Windows[0].Channels) != 2 || len(event.Channels) != 2 {
		t.Fatalf("expected deduplicated window and flat channels, got windows=%+v channels=%+v", event.Windows, event.Channels)
	}
	if event.StartUnix != start || event.EndUnix != start+6*3600 {
		t.Fatalf("expected legacy bounds to span the coverage windows, got %+v", event)
	}
}

func TestHTTPRoutesServerEventsKeepsDifferentSeriesTitlesSeparate(t *testing.T) {
	t.Parallel()

	start := time.Now().Add(72 * time.Hour).Unix()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Channels: []model.Channel{{ID: "ch:tennis", Name: "Tennis", CategoryID: "tennis", CategoryName: "Sports | Tennis"}},
		Programs: []model.Program{
			{ID: "p:one", ChannelID: "ch:tennis", Title: "Wimbledon: Centre Court", StartUnix: start, EndUnix: start + 3*3600},
			{ID: "p:two", ChannelID: "ch:tennis", Title: "Wimbledon: Court One", StartUnix: start + 20*60, EndUnix: start + 3*3600},
		},
		Content: model.ContentState{LiveCategories: []model.Category{{ID: "tennis", Name: "Sports | Tennis", Kind: "live"}}},
	}})

	payload := fetchEventsPayload(t, NewHTTPRoutesServer(store))
	if len(payload.Events) != 2 {
		t.Fatalf("expected distinct tournament titles to remain separate, got %+v", payload.Events)
	}
}

func fetchEventsPayload(t *testing.T, server *HTTPRoutesServer) EventsPayload {
	t.Helper()

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/dispatcharr/api/events"})
	if err != nil {
		t.Fatalf("events route: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.GetStatusCode(), string(response.GetBody()))
	}
	var payload EventsPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

func assertBroadcastEventMatch(t *testing.T, matches []SportsChannelMatch, channelID string) {
	t.Helper()

	for _, match := range matches {
		if match.ID == channelID {
			return
		}
	}
	t.Fatalf("expected %s in event matches: %+v", channelID, matches)
}

func assertNoBroadcastEventMatch(t *testing.T, matches []SportsChannelMatch, channelID string) {
	t.Helper()

	for _, match := range matches {
		if match.ID == channelID {
			t.Fatalf("did not expect %s in event matches: %+v", channelID, matches)
		}
	}
}
