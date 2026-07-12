package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/cache"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/timeshift"
)

func TestTimeShiftRoutesShareBuffersAndGateAdminOperations(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer upstream.Close()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeDirectLogin),
		Channels: []model.Channel{{ID: "channel-1", Name: "Channel 1", StreamURL: upstream.URL}},
	}})
	store.SetAdminSettings(json.RawMessage(`{"liveRewindEnabled":true,"liveRewindCacheGB":1,"liveRewindWindowMinutes":15,"liveRewindMinFreeGB":1,"liveRewindMaxChannels":2}`))
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings { return config.Settings{SourceMode: config.SourceModeDirectLogin} })
	server.timeShift = timeshift.NewManager(t.TempDir())

	start := func() map[string]any {
		response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
			Path: "/dispatcharr/api/timeshift/start", Method: http.MethodPost, Body: []byte(`{"channelId":"channel-1"}`),
		})
		if err != nil || response.GetStatusCode() != http.StatusAccepted {
			t.Fatalf("start rewind: status=%d err=%v body=%s", response.GetStatusCode(), err, response.GetBody())
		}
		var payload map[string]any
		if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
			t.Fatalf("decode start response: %v", err)
		}
		return payload
	}
	first := start()
	second := start()
	if first["bufferId"] != second["bufferId"] || first["leaseId"] == second["leaseId"] {
		t.Fatalf("expected shared buffer and unique leases: first=%v second=%v", first, second)
	}

	unauthorized, _ := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Path: "/dispatcharr/api/timeshift/admin-status", Method: http.MethodGet})
	if unauthorized.GetStatusCode() != http.StatusForbidden {
		t.Fatalf("expected admin status to require admin role, got %d", unauthorized.GetStatusCode())
	}
	authorized, _ := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Path: "/dispatcharr/api/timeshift/admin-status", Method: http.MethodGet, Headers: map[string]string{"x-silo-user-role": "admin"},
	})
	if authorized.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected admin status, got %d: %s", authorized.GetStatusCode(), authorized.GetBody())
	}
	var stats timeshift.Stats
	if err := json.Unmarshal(authorized.GetBody(), &stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.ActiveBuffers != 1 || stats.ActiveLeases != 2 {
		t.Fatalf("expected shared runtime usage, got %+v", stats)
	}

	cleared, _ := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Path: "/dispatcharr/api/timeshift/clear", Method: http.MethodPost, Headers: map[string]string{"x-silo-user-role": "admin"},
	})
	if cleared.GetStatusCode() != http.StatusOK {
		t.Fatalf("clear rewind cache: %d %s", cleared.GetStatusCode(), cleared.GetBody())
	}
}

func TestTimeShiftStartFallsBackWhenDisabledOrNotDirect(t *testing.T) {
	t.Parallel()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{Channels: []model.Channel{{ID: "channel", StreamURL: "http://example.test/live.ts"}}}})
	store.SetAdminSettings(json.RawMessage(`{"liveRewindEnabled":false}`))
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings { return config.Settings{SourceMode: config.SourceModeXtream} })
	server.timeShift = timeshift.NewManager(t.TempDir())
	response, _ := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Path: "/dispatcharr/api/timeshift/start", Method: http.MethodPost, Body: []byte(`{"channelId":"channel"}`),
	})
	if response.GetStatusCode() != http.StatusConflict {
		t.Fatalf("expected unavailable response, got %d", response.GetStatusCode())
	}
}
