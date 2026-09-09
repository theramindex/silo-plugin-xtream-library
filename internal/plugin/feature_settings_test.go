package plugin

import (
	"context"
	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"
)

func newEnabledSportsTestServer(store *cache.Store) *HTTPRoutesServer {
	if store != nil && !store.HasAdminSettings() {
		store.SetAdminSettings([]byte(`{"sportsEnabled":true}`))
	}
	return NewHTTPRoutesServer(store)
}

func TestXtreamSportsRequiresAdminOptIn(t *testing.T) {
	store := cache.NewStore()
	server := NewHTTPRoutesServer(store)
	if server.sportsFeatureEnabled() {
		t.Fatal("sports must default to disabled")
	}
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/xtream/api/sports"})
	if err != nil || response.GetStatusCode() != http.StatusOK {
		t.Fatalf("disabled sports response: %v %v", response, err)
	}
	store.SetAdminSettings([]byte(`{"sportsEnabled":true}`))
	if !server.sportsFeatureEnabled() {
		t.Fatal("admin opt-in must enable sports")
	}
	store.SetAdminSettings([]byte(`{"sportsEnabled":false}`))
	if server.sportsFeatureEnabled() {
		t.Fatal("admin opt-out must disable sports")
	}
}

func TestXtreamSportsAdminSettingSurvivesRestart(t *testing.T) {
	storagePath := filepath.Join(t.TempDir(), "admin-settings.json")
	server := NewHTTPRoutesServerWithCoordinatorAndAdminSettingsFile(cache.NewStore(), nil, nil, storagePath)
	server.adminPersister = func(context.Context, map[string]any) error { return nil }
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "POST", Path: "/xtream/api/admin-settings", Headers: map[string]string{"x-silo-user-role": "admin"}, Body: []byte(`{"sportsEnabled":true}`)})
	if err != nil || response.GetStatusCode() != http.StatusOK {
		t.Fatalf("save sports opt-in: %v %v", response, err)
	}
	restarted := NewHTTPRoutesServerWithCoordinatorAndAdminSettingsFile(cache.NewStore(), nil, nil, storagePath)
	if !restarted.sportsFeatureEnabled() {
		t.Fatal("sports opt-in must survive a restart before the first admin request")
	}
}

func TestXtreamFeatureBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for UI behavior checks")
	}
	command := exec.Command(node, filepath.Join("..", "..", "scripts", "test-features.cjs"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("feature behavior: %v\n%s", err, output)
	}
}
