package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	goruntime "runtime"
	"strings"
	"sync"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
	sdkruntime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimedefault"
	"github.com/theramindex/silo-plugin-xtream-library/internal/app"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	pluginimpl "github.com/theramindex/silo-plugin-xtream-library/internal/plugin"
)

//go:embed manifest.json
var manifestJSON []byte

// buildVersion is injected by CI via -ldflags:
//
//	-X main.buildVersion=<semver>
var buildVersion string

type runtimeServer struct {
	runtimedefault.Server
	manifest       *pluginv1.PluginManifest
	settings       *settingsState
	sourceRegistry *config.SourceRegistry
}

type settingsState struct {
	mu       sync.RWMutex
	settings config.Settings
}

func (s *runtimeServer) GetManifest(context.Context, *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *runtimeServer) Configure(_ context.Context, request *pluginv1.ConfigureRequest) (*pluginv1.ConfigureResponse, error) {
	if s.settings == nil {
		return &pluginv1.ConfigureResponse{}, nil
	}

	current := s.settings.Get()
	s.migrateRetiredConnection(request, &current)
	if current.ChannelRefreshH == 0 {
		current.ChannelRefreshH = config.DefaultChannelRefreshHours
	}
	if current.EPGRefreshH == 0 {
		current.EPGRefreshH = config.DefaultEPGRefreshHours
	}
	// Provider settings are owned by XC Admin's durable source registry. Retired
	// Silo global values are accepted only as a best-effort upgrade migration;
	// incomplete values are ignored and can never block route registration.
	s.settings.Set(current)
	return &pluginv1.ConfigureResponse{}, nil
}

func (s *runtimeServer) migrateRetiredConnection(request *pluginv1.ConfigureRequest, current *config.Settings) {
	if s.sourceRegistry != nil {
		sources, err := s.sourceRegistry.Load()
		if err != nil {
			log.Printf("xtream: inspect source registry before legacy migration failed: %v", err)
			return
		}
		if len(sources) > 0 {
			current.SourceMode = config.SourceModeXtream
			current.XtreamSources = sources
			return
		}
	}

	for _, entry := range request.GetConfig() {
		if entry.GetKey() != "connection" {
			continue
		}
		values := entry.GetValue().AsMap()
		if config.SourceMode(asString(values["source_mode"])) == config.SourceModeM3UXMLTV {
			m3uURL := asString(values["m3u_url"])
			epgURL := asString(values["epg_xml_url"])
			if strings.TrimSpace(m3uURL) != "" && strings.TrimSpace(epgURL) != "" {
				current.SourceMode = config.SourceModeM3UXMLTV
				current.M3UURL = m3uURL
				current.EPGXMLURL = epgURL
			}
			return
		}

		baseURL := firstString(values, "xtream_base_url", "base_url")
		username := firstString(values, "xtream_username", "username")
		password := firstString(values, "xtream_password", "password")
		if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
			return
		}
		source := config.XtreamSource{ID: config.DeriveXtreamSourceID(baseURL, username), Name: config.DefaultXtreamSourceName(baseURL, username), BaseURL: baseURL, Username: username, Password: password, LiveFormat: firstString(values, "live_stream_format"), Enabled: true}
		if epgURL := strings.TrimSpace(asString(values["epg_xml_url"])); epgURL != "" {
			source.AlternateEPGEnabled = true
			source.AlternateEPGURL = epgURL
			source.AlternateEPGPolicy = config.AlternateEPGPolicyPreferAlternate
		}
		if source.LiveFormat == "" {
			source.LiveFormat = "m3u8"
		}
		if s.sourceRegistry != nil {
			if err := s.sourceRegistry.Save([]config.XtreamSource{source}); err != nil {
				log.Printf("xtream: migrate retired source failed: %v", err)
				return
			}
		}
		current.SourceMode = config.SourceModeXtream
		current.XtreamSources = []config.XtreamSource{source}
		return
	}
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := asString(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func (s *settingsState) Get() config.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *settingsState) Set(settings config.Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
}

func main() {
	manifest, err := loadManifest()
	if err != nil {
		panic(err)
	}
	store := cache.NewStore()
	snapshotStorage := cache.NewFileSnapshotStorage("")
	if snapshot, ok, loadErr := snapshotStorage.Load(); loadErr != nil {
		log.Printf("xtream: load catalog snapshot failed: %v", loadErr)
	} else if ok {
		store.Replace(snapshot)
	}
	settings := &settingsState{settings: config.Settings{SourceMode: config.SourceModeXtream, LiveTVEnabled: true, ChannelRefreshH: config.DefaultChannelRefreshHours, EPGRefreshH: config.DefaultEPGRefreshHours}}
	sourceRegistry := config.NewSourceRegistry("")
	settingsProvider := func() config.Settings {
		return settingsWithRegisteredSources(settings.Get(), sourceRegistry)
	}
	service := app.NewService(app.Dependencies{Store: store, SnapshotStorage: snapshotStorage})
	coordinator := pluginimpl.NewRefreshCoordinator(service)

	sdkruntime.Serve(sdkruntime.ServeConfig{
		Servers: sdkruntime.CapabilityServers{
			Runtime:       &runtimeServer{manifest: manifest, settings: settings, sourceRegistry: sourceRegistry},
			ScheduledTask: pluginimpl.NewScheduledTaskServerWithCoordinator(coordinator, settingsProvider),
			HttpRoutes:    pluginimpl.NewHTTPRoutesServerWithCoordinatorAndAdminSettingsFile(store, settingsProvider, coordinator, ""),
		},
	})
}

func settingsWithRegisteredSources(current config.Settings, sourceRegistry *config.SourceRegistry) config.Settings {
	if sourceRegistry == nil {
		return current
	}
	if sources, loadErr := sourceRegistry.Load(); loadErr == nil && len(sources) > 0 {
		current.SourceMode = config.SourceModeXtream
		current.XtreamSources = sources
	}
	return current
}

func loadManifest() (*pluginv1.PluginManifest, error) {
	manifest, err := publicmanifest.Load(manifestJSON)
	if err != nil {
		return nil, fmt.Errorf("load embedded manifest: %w", err)
	}

	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}

	binaryData, err := os.ReadFile(executablePath)
	if err != nil {
		return nil, fmt.Errorf("read executable %q: %w", executablePath, err)
	}

	checksum := sha256.Sum256(binaryData)
	manifest.Checksum = hex.EncodeToString(checksum[:])
	if buildVersion != "" {
		manifest.Version = buildVersion
	}
	if len(manifest.GetSupportedPlatforms()) == 0 {
		manifest.SupportedPlatforms = []*pluginv1.SupportedPlatform{{
			Os:   goruntime.GOOS,
			Arch: goruntime.GOARCH,
		}}
	}
	manifest.GlobalConfigSchema = nil
	manifest.UserConfigSchema = config.UserConfigSchema()
	rewritePublicManifestForXtream(manifest)

	return manifest, nil
}

func rewritePublicManifestForXtream(manifest *pluginv1.PluginManifest) {
	routes := make([]*pluginv1.HttpRouteDescriptor, 0, len(manifest.GetHttpRoutes()))
	for _, route := range manifest.GetHttpRoutes() {
		if isRetiredPublicRoute(route.GetPath()) {
			continue
		}
		route.Id = strings.Replace(route.GetId(), "dispatcharr", "xtream", 1)
		route.Path = strings.Replace(route.GetPath(), "/dispatcharr", "/xtream", 1)
		routes = append(routes, route)
	}
	manifest.HttpRoutes = routes
	for _, capability := range manifest.GetCapabilities() {
		capability.Id = strings.Replace(capability.GetId(), "dispatcharr", "xtream", 1)
		capability.DisplayName = strings.ReplaceAll(capability.GetDisplayName(), "Dispatcharr", "Xtreme Codes")
		capability.Description = strings.ReplaceAll(capability.GetDescription(), "Dispatcharr", "Xtream")
	}
}

func isRetiredPublicRoute(path string) bool {
	for _, segment := range []string{"/recordings", "/sports", "/events", "/timeshift"} {
		if strings.Contains(path, segment) {
			return true
		}
	}
	return false
}

func asString(value any) string {
	if stringValue, ok := value.(string); ok {
		return stringValue
	}
	return ""
}
