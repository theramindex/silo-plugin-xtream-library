package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	goruntime "runtime"
	"strings"
	"sync"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
	sdkruntime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimedefault"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/app"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/cache"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	pluginimpl "github.com/theramindex/silo-plugin-dispatcharr/internal/plugin"
)

//go:embed manifest.json
var manifestJSON []byte

// buildVersion is injected by CI via -ldflags:
//
//	-X main.buildVersion=<semver>
var buildVersion string

type runtimeServer struct {
	runtimedefault.Server
	manifest *pluginv1.PluginManifest
	settings *settingsState
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
	for _, entry := range request.GetConfig() {
		values := entry.GetValue().AsMap()
		switch entry.GetKey() {
		case "connection":
			applyConnectionConfig(&current, values)
		case "general":
			applyLegacyGeneralConfig(&current, values)
		case "dispatcharr":
			applyDispatcharrConfig(&current, values)
		case "xtream":
			applyXtreamConfig(&current, values)
		case "m3u_xmltv":
			applyM3UConfig(&current, values)
		case "category_settings":
			if encoded, err := json.Marshal(values); err == nil {
				current.AdminSettings = encoded
			}
		}
	}
	if current.ChannelRefreshH == 0 {
		current.ChannelRefreshH = config.DefaultChannelRefreshHours
	}
	if current.EPGRefreshH == 0 {
		current.EPGRefreshH = config.DefaultEPGRefreshHours
	}
	s.settings.Set(current)
	return &pluginv1.ConfigureResponse{}, nil
}

func applyConnectionConfig(settings *config.Settings, values map[string]any) {
	applySourceMode(settings, values)
	applyDispatcharrConfig(settings, values)
	applyFirstPresentString(&settings.XtreamBaseURL, values, "xtream_base_url", "base_url")
	applyFirstPresentString(&settings.XtreamUsername, values, "xtream_username", "username")
	applyFirstPresentString(&settings.XtreamPassword, values, "xtream_password", "password")
	applyM3UConfig(settings, values)
	applyLegacyScheduleConfig(settings, values)
	if settings.SourceMode == "" {
		settings.SourceMode = config.SourceModeXtream
	}
}

func applyLegacyGeneralConfig(settings *config.Settings, values map[string]any) {
	applySourceMode(settings, values)
	applyLegacyScheduleConfig(settings, values)
}

func applySourceMode(settings *config.Settings, values map[string]any) {
	if value, ok := values["source_mode"].(string); ok {
		switch config.SourceMode(value) {
		case config.SourceModeXtream, config.SourceModeM3UXMLTV:
			settings.SourceMode = config.SourceMode(value)
		}
	}
}

func applyDispatcharrConfig(settings *config.Settings, values map[string]any) {
	applyStringIfPresent(&settings.DispatcharrURL, values, "base_url")
	applyStringIfPresent(&settings.DispatcharrUser, values, "username")
	applyStringIfPresent(&settings.DispatcharrPass, values, "password")
	applyStringIfPresent(&settings.DispatcharrAPIKey, values, "api_key")
	applyStringIfPresent(&settings.ChannelProfile, values, "channel_profile")
}

func applyXtreamConfig(settings *config.Settings, values map[string]any) {
	applyStringIfPresent(&settings.XtreamBaseURL, values, "base_url")
	applyStringIfPresent(&settings.XtreamUsername, values, "username")
	applyStringIfPresent(&settings.XtreamPassword, values, "password")
}

func applyM3UConfig(settings *config.Settings, values map[string]any) {
	applyStringIfPresent(&settings.M3UURL, values, "m3u_url")
	applyStringIfPresent(&settings.EPGXMLURL, values, "epg_xml_url")
}

func applyStringIfPresent(target *string, values map[string]any, key string) {
	value, exists := values[key]
	if !exists {
		return
	}
	*target = asString(value)
}

func applyFirstPresentString(target *string, values map[string]any, keys ...string) {
	for _, key := range keys {
		value, exists := values[key]
		if !exists {
			continue
		}
		if stringValue := asString(value); stringValue != "" {
			*target = stringValue
			return
		}
		*target = ""
	}
}

func applyLegacyScheduleConfig(settings *config.Settings, values map[string]any) {
	if value, ok := values["live_tv_enabled"].(bool); ok {
		settings.LiveTVEnabled = value
	}
	if value, ok := values["channel_refresh_hours"].(float64); ok {
		settings.ChannelRefreshH = int(value)
	}
	if value, ok := values["epg_refresh_hours"].(float64); ok {
		settings.EPGRefreshH = int(value)
	}
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
	if snapshot, ok, err := snapshotStorage.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "dispatcharr: load catalog snapshot: %v\n", err)
	} else if ok {
		store.Replace(snapshot)
	}
	settings := &settingsState{settings: config.Settings{SourceMode: config.SourceModeXtream, LiveTVEnabled: true, ChannelRefreshH: config.DefaultChannelRefreshHours, EPGRefreshH: config.DefaultEPGRefreshHours}}
	service := app.NewService(app.Dependencies{Store: store, SnapshotStorage: snapshotStorage})
	coordinator := pluginimpl.NewRefreshCoordinator(service)

	sdkruntime.Serve(sdkruntime.ServeConfig{
		Servers: sdkruntime.CapabilityServers{
			Runtime:       &runtimeServer{manifest: manifest, settings: settings},
			ScheduledTask: pluginimpl.NewScheduledTaskServerWithCoordinator(coordinator, settings.Get),
			HttpRoutes:    pluginimpl.NewHTTPRoutesServerWithCoordinatorAndAdminSettingsFile(store, settings.Get, coordinator, ""),
		},
	})
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
	manifest.GlobalConfigSchema = config.GlobalConfigSchema()
	manifest.UserConfigSchema = config.UserConfigSchema()
	rewritePublicManifestForXtream(manifest)

	return manifest, nil
}

func rewritePublicManifestForXtream(manifest *pluginv1.PluginManifest) {
	for _, route := range manifest.GetHttpRoutes() {
		route.Id = strings.Replace(route.GetId(), "dispatcharr", "xtream", 1)
		route.Path = strings.Replace(route.GetPath(), "/dispatcharr", "/xtream", 1)
	}
	for _, capability := range manifest.GetCapabilities() {
		capability.Id = strings.Replace(capability.GetId(), "dispatcharr", "xtream", 1)
		capability.DisplayName = strings.ReplaceAll(capability.GetDisplayName(), "Dispatcharr", "Xtreme Codes")
		capability.Description = strings.ReplaceAll(capability.GetDescription(), "Dispatcharr", "Xtream")
	}
}

func asString(value any) string {
	if stringValue, ok := value.(string); ok {
		return stringValue
	}
	return ""
}
