package plugin

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/app"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	upstreamhttp "github.com/theramindex/silo-plugins/pkg/livemedia/httpclient"
)

// userSourcesEnabledKey is the admin setting that switches XC from one shared
// provider registry to per-user providers.
const userSourcesEnabledKey = "userSourcesEnabled"

// DefaultUserStateDir holds one directory per Silo user in per-user mode.
const DefaultUserStateDir = "/var/lib/continuum/plugins/silo.ramindex.xtream/users"

var tenantUserIDPattern = regexp.MustCompile(`^[0-9]{1,12}$`)

// tenant is one Silo user's private XC: their own sealed source registry,
// catalog store, snapshot file, and refresh coordinator. Nothing outside the
// tenant reads its registry; the shared (admin) server never holds one.
type tenant struct {
	userID      string
	server      *HTTPRoutesServer
	coordinator *RefreshCoordinator
	settings    func() config.Settings
}

// TenantRouter serves XC's HTTP routes. With per-user providers off it passes
// every request to the shared server, exactly as before. With them on, admin
// routes stay on the shared server and every other route goes to the caller's
// own tenant, chosen by the host-stamped X-Silo-User-Id header.
type TenantRouter struct {
	pluginv1.UnimplementedHttpRoutesServer
	shared       *HTTPRoutesServer
	baseSettings func() config.Settings
	stateDir     string
	box          *config.SecretBox

	mu      sync.Mutex
	tenants map[string]*tenant
}

func NewTenantRouter(shared *HTTPRoutesServer, baseSettings func() config.Settings, stateDir string) *TenantRouter {
	if strings.TrimSpace(stateDir) == "" {
		stateDir = os.Getenv("XTREAM_USER_STATE_DIR")
	}
	if strings.TrimSpace(stateDir) == "" {
		stateDir = DefaultUserStateDir
	}
	return &TenantRouter{
		shared:       shared,
		baseSettings: baseSettings,
		stateDir:     stateDir,
		box:          config.NewSecretBox(filepath.Join(stateDir, ".sources.key")),
		tenants:      map[string]*tenant{},
	}
}

// UserSourcesEnabled reports the admin's per-user provider choice.
func (r *TenantRouter) UserSourcesEnabled() bool {
	return r.shared.adminFlag(userSourcesEnabledKey, false)
}

func (r *TenantRouter) Handle(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if !r.UserSourcesEnabled() || isSharedRoute(request) {
		return r.shared.Handle(ctx, request)
	}
	userID := strings.TrimSpace(headerValue(request.GetHeaders(), "x-silo-user-id"))
	if userID == "" && normalizePublicPath(request.GetPath()) == "/silo-compat/livetv/v1/info" {
		// Silo's bridge may probe info without a caller; say lineups are
		// per user and list no hosts, since hosts are per user too.
		return jsonHTTPResponse(http.StatusOK, livetvInfo{ContractVersion: livetvContractVersion, Name: "XC for Silo", StreamHosts: []string{}, PerUserLineup: true}), nil
	}
	if !tenantUserIDPattern.MatchString(userID) {
		return textResponse(http.StatusUnauthorized, "a signed-in Silo user is required"), nil
	}
	return r.tenant(userID).server.Handle(ctx, request)
}

// isSharedRoute lists the routes that never touch a user's providers: the
// admin page and its APIs, and static assets. Relay resumes go to the tenant
// so they are fetched under its address restrictions.
func isSharedRoute(request *pluginv1.HandleHTTPRequest) bool {
	path := normalizePublicPath(request.GetPath())
	path, _, _ = strings.Cut(path, "?")
	switch path {
	case "/dispatcharr/admin", "/xtream/admin",
		"/dispatcharr/api/admin-settings", "/dispatcharr/api/admin-sources":
		return true
	}
	return strings.HasPrefix(path, "/dispatcharr/assets/") || strings.HasPrefix(path, "/assets/") ||
		strings.HasPrefix(path, "/dispatcharr/api/sports/image")
}

func (r *TenantRouter) tenant(userID string) *tenant {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.tenants[userID]; ok {
		return existing
	}
	created := r.newTenant(userID)
	r.tenants[userID] = created
	return created
}

func (r *TenantRouter) newTenant(userID string) *tenant {
	dir := filepath.Join(r.stateDir, userID)
	registry := config.NewSealedSourceRegistry(filepath.Join(dir, "sources.sealed"), r.box)
	settings := func() config.Settings {
		return tenantSettings(r.baseSettings(), registry)
	}
	store := cache.NewStore()
	snapshots := cache.NewFileSnapshotStorage(filepath.Join(dir, "catalog-snapshot.json"))
	if snapshot, ok, err := snapshots.Load(); err == nil && ok {
		store.Replace(snapshot)
	}
	// Everything a tenant fetches comes from a URL a user typed, so it must
	// resolve to a public address (no LAN, loopback, or metadata endpoints).
	catalogClient := upstreamhttp.NewUntrusted(5 * time.Minute)
	service := app.NewService(app.Dependencies{Store: store, SnapshotStorage: snapshots, HTTPClient: catalogClient})
	coordinator := NewRefreshCoordinator(service)

	server := newHTTPRoutesServer(store, settings, nil)
	server.coordinator = coordinator
	server.upstreamClient = catalogClient
	epgClient := upstreamhttp.NewUntrusted(30 * time.Second)
	server.sourceEPGTester = func(ctx context.Context, source config.XtreamSource, channels []model.Channel) (alternateEPGTestPayload, error) {
		return testAlternateEPGSourceWith(ctx, epgClient, source, channels)
	}
	server.sourceRegistry = registry
	server.tenantUserID = userID
	server.adminSettingsFrom = r.shared
	server.relay = r.shared.relay
	server.sportsProvider = r.shared.sportsProvider
	server.sportsImages = r.shared.sportsImages
	return &tenant{userID: userID, server: server, coordinator: coordinator, settings: settings}
}

// tenantSettings is the admin's runtime settings with the provider list
// replaced by the user's own. A user without sources gets none: the admin's
// providers are never a fallback.
func tenantSettings(base config.Settings, registry *config.SourceRegistry) config.Settings {
	base.SourceMode = config.SourceModeXtream
	base.XtreamBaseURL = ""
	base.XtreamUsername = ""
	base.XtreamPassword = ""
	base.XtreamSources = []config.XtreamSource{}
	if sources, err := registry.Load(); err == nil && sources != nil {
		base.XtreamSources = sources
	}
	return base
}

// persistedTenants loads every user that has saved sources, so scheduled
// refreshes cover users who have not opened XC since the plugin started.
func (r *TenantRouter) persistedTenants() []*tenant {
	entries, err := os.ReadDir(r.stateDir)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && tenantUserIDPattern.MatchString(entry.Name()) {
			if _, statErr := os.Stat(filepath.Join(r.stateDir, entry.Name(), "sources.sealed")); statErr == nil {
				ids = append(ids, entry.Name())
			}
		}
	}
	sort.Strings(ids)
	tenants := make([]*tenant, 0, len(ids))
	for _, id := range ids {
		tenants = append(tenants, r.tenant(id))
	}
	return tenants
}
