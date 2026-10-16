package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
)

var adminSource = config.XtreamSource{ID: "admin-provider", Name: "Admin", BaseURL: "https://admin-provider.example", Username: "admin-user", Password: "admin-secret", Enabled: true}

func newTestTenantRouter(t *testing.T, perUser bool) (*TenantRouter, string) {
	t.Helper()
	dir := t.TempDir()
	base := func() config.Settings {
		return config.Settings{SourceMode: config.SourceModeXtream, XtreamSources: []config.XtreamSource{adminSource}, ChannelRefreshH: 24, EPGRefreshH: 24}
	}
	shared := NewHTTPRoutesServerWithSettings(cache.NewStore(), base)
	shared.adminStorage = NewFileAdminSettingsStorage(filepath.Join(dir, "admin-settings.json"))
	shared.sourceRegistry = config.NewSourceRegistry(filepath.Join(dir, "sources.json"))
	if err := shared.sourceRegistry.Save([]config.XtreamSource{adminSource}); err != nil {
		t.Fatalf("seed admin registry: %v", err)
	}
	router := NewTenantRouter(shared, base, filepath.Join(dir, "users"))
	if perUser {
		response := routeRequest(t, router, http.MethodPost, "/xtream/api/admin-settings", "1", "admin", `{"userSourcesEnabled":true}`)
		if response.GetStatusCode() != http.StatusOK {
			t.Fatalf("enable per-user providers: %d %s", response.GetStatusCode(), response.GetBody())
		}
	}
	return router, dir
}

func routeRequest(t *testing.T, router *TenantRouter, method, path, userID, role, body string) *pluginv1.HandleHTTPResponse {
	t.Helper()
	headers := map[string]string{}
	if userID != "" {
		headers["X-Silo-User-Id"] = userID
	}
	if role != "" {
		headers["X-Silo-User-Role"] = role
	}
	response, err := router.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: method, Path: path, Headers: headers, Body: []byte(body)})
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return response
}

func TestTenantRouterSharedModeIsUnchanged(t *testing.T) {
	t.Parallel()
	router, _ := newTestTenantRouter(t, false)
	if router.UserSourcesEnabled() {
		t.Fatal("per-user providers must default to off")
	}
	if response := routeRequest(t, router, http.MethodGet, "/xtream/api/my-sources", "7", "user", ""); response.GetStatusCode() != http.StatusNotFound {
		t.Fatalf("my-sources must not exist in shared mode, got %d", response.GetStatusCode())
	}
	response := routeRequest(t, router, http.MethodGet, "/xtream/api/admin-sources", "1", "admin", "")
	if !strings.Contains(string(response.GetBody()), "admin-provider") {
		t.Fatalf("admin must still see the shared registry: %s", response.GetBody())
	}
}

func TestTenantRouterRequiresSignedInUser(t *testing.T) {
	t.Parallel()
	router, _ := newTestTenantRouter(t, true)
	for _, userID := range []string{"", "../1", "1 OR 1"} {
		if response := routeRequest(t, router, http.MethodGet, "/xtream/api/app", userID, "user", ""); response.GetStatusCode() != http.StatusUnauthorized {
			t.Fatalf("user id %q: expected 401, got %d", userID, response.GetStatusCode())
		}
	}
}

func TestTenantSourcesArePrivateSealedAndWriteOnly(t *testing.T) {
	t.Parallel()
	router, dir := newTestTenantRouter(t, true)
	save := routeRequest(t, router, http.MethodPost, "/xtream/api/my-sources", "7", "user",
		`{"name":"Mine","baseUrl":"https://user-provider.example","username":"seven-user","password":"seven-secret","liveFormat":"m3u8","enabled":true}`)
	if save.GetStatusCode() != http.StatusOK {
		t.Fatalf("save own source: %d %s", save.GetStatusCode(), save.GetBody())
	}
	if strings.Contains(string(save.GetBody()), "seven-secret") || !strings.Contains(string(save.GetBody()), `"passwordConfigured":true`) {
		t.Fatalf("own source response must not echo the password: %s", save.GetBody())
	}

	sealed, err := os.ReadFile(filepath.Join(dir, "users", "7", "sources.sealed"))
	if err != nil {
		t.Fatalf("read sealed registry: %v", err)
	}
	for _, secret := range []string{"seven-secret", "seven-user", "user-provider.example"} {
		if strings.Contains(string(sealed), secret) {
			t.Fatalf("sealed registry leaks %q in plaintext", secret)
		}
	}

	other := routeRequest(t, router, http.MethodGet, "/xtream/api/my-sources", "8", "user", "")
	if strings.Contains(string(other.GetBody()), "user-provider") || strings.Contains(string(other.GetBody()), "admin-provider") {
		t.Fatalf("user 8 must see neither user 7's nor the admin's sources: %s", other.GetBody())
	}
	admin := routeRequest(t, router, http.MethodGet, "/xtream/api/admin-sources", "1", "admin", "")
	if strings.Contains(string(admin.GetBody()), "user-provider") || strings.Contains(string(admin.GetBody()), "seven") {
		t.Fatalf("admin source list must not include a user's sources: %s", admin.GetBody())
	}
	// An admin's own tenant is a separate user too.
	adminOwn := routeRequest(t, router, http.MethodGet, "/xtream/api/my-sources", "1", "admin", "")
	if strings.Contains(string(adminOwn.GetBody()), "user-provider") {
		t.Fatalf("admin's my-sources must not include user 7's sources: %s", adminOwn.GetBody())
	}
}

func TestTenantNeverFallsBackToAdminSources(t *testing.T) {
	t.Parallel()
	router, _ := newTestTenantRouter(t, true)
	response := routeRequest(t, router, http.MethodGet, "/xtream/api/app", "9", "user", "")
	var app AppPayload
	if err := json.Unmarshal(response.GetBody(), &app); err != nil {
		t.Fatalf("decode app payload: %v (%s)", err, response.GetBody())
	}
	if !app.UserSources || app.SourcesConfigured {
		t.Fatalf("expected userSources without configured sources, got %+v", app)
	}
	if settings := router.tenant("9").settings(); len(settings.XtreamSources) != 0 {
		t.Fatalf("tenant settings must not inherit admin sources: %+v", settings.XtreamSources)
	}
}

func TestTenantConnectionTestRefusesPrivateAddresses(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"user_info":{"auth":1,"status":"Active"}}`))
	}))
	defer upstream.Close()
	router, _ := newTestTenantRouter(t, true)
	body := `{"action":"test","name":"Lan","baseUrl":"` + upstream.URL + `","username":"u","password":"p","enabled":true}`
	response := routeRequest(t, router, http.MethodPost, "/xtream/api/my-sources", "7", "user", body)
	if response.GetStatusCode() == http.StatusOK || !strings.Contains(string(response.GetBody()), "not allowed") {
		t.Fatalf("a user's provider on a loopback address must be refused, got %d %s", response.GetStatusCode(), response.GetBody())
	}
}
