package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	configsdk "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestRuntimeConfigureIgnoresRetiredGlobalConnection(t *testing.T) {
	t.Parallel()

	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeXtream, LiveTVEnabled: true, ChannelRefreshH: config.DefaultChannelRefreshHours, EPGRefreshH: config.DefaultEPGRefreshHours}}
	server := &runtimeServer{settings: state}
	request := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{
		{Key: "connection", Value: mustStruct(t, map[string]any{
			"source_mode": "xtream",
			"base_url":    "https://legacy.example.com",
			"username":    "legacy-user",
			"password":    "legacy-password",
		})},
	}}

	if _, err := server.Configure(context.Background(), request); err != nil {
		t.Fatalf("configure: %v", err)
	}
	settings := state.Get()
	if settings.XtreamBaseURL != "" || settings.XtreamUsername != "" || settings.XtreamPassword != "" {
		t.Fatalf("retired global connection was imported: %+v", settings)
	}
}

func TestRuntimeConfigureMigratesCompleteRetiredXtreamConnection(t *testing.T) {
	t.Parallel()

	registry := config.NewSourceRegistry(filepath.Join(t.TempDir(), "sources.json"))
	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeXtream}}
	server := &runtimeServer{settings: state, sourceRegistry: registry}
	request := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{{Key: "connection", Value: mustStruct(t, map[string]any{
		"source_mode":        "xtream",
		"base_url":           "https://provider.example.com",
		"username":           "demo",
		"password":           "secret",
		"live_stream_format": "m3u8",
	})}}}

	if _, err := server.Configure(context.Background(), request); err != nil {
		t.Fatalf("configure: %v", err)
	}
	sources, err := registry.Load()
	if err != nil {
		t.Fatalf("load migrated registry: %v", err)
	}
	if len(sources) != 1 || sources[0].BaseURL != "https://provider.example.com" || sources[0].Username != "demo" {
		t.Fatalf("expected complete legacy source to migrate once, got %+v", sources)
	}
}

func TestRuntimeConfigureMigrationPrefersRetiredXtreamAliasesAndPreservesCustomEPG(t *testing.T) {
	t.Parallel()

	registry := config.NewSourceRegistry(filepath.Join(t.TempDir(), "sources.json"))
	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeXtream}}
	server := &runtimeServer{settings: state, sourceRegistry: registry}
	request := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{{Key: "connection", Value: mustStruct(t, map[string]any{
		"source_mode":     "xtream",
		"base_url":        "https://shared-stale.example.com",
		"username":        "shared-stale-user",
		"password":        "shared-stale-password",
		"xtream_base_url": "https://provider.example.com",
		"xtream_username": "demo",
		"xtream_password": "secret",
		"epg_xml_url":     "https://guide.example.com/guide.xml",
	})}}}

	if _, err := server.Configure(context.Background(), request); err != nil {
		t.Fatalf("configure: %v", err)
	}
	sources, err := registry.Load()
	if err != nil {
		t.Fatalf("load migrated registry: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected one migrated source, got %+v", sources)
	}
	source := sources[0]
	if source.BaseURL != "https://provider.example.com" || source.Username != "demo" || source.Password != "secret" {
		t.Fatalf("expected retired Xtream-specific aliases to retain precedence, got %+v", source)
	}
	if !source.AlternateEPGEnabled || source.AlternateEPGURL != "https://guide.example.com/guide.xml" || source.AlternateEPGPolicy != config.AlternateEPGPolicyPreferAlternate {
		t.Fatalf("expected custom legacy XMLTV guide to migrate as a preferred alternate guide, got %+v", source)
	}
}

func TestRuntimeConfigureRetainsCompleteRetiredM3UConnection(t *testing.T) {
	t.Parallel()

	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeXtream}}
	server := &runtimeServer{settings: state}
	request := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{{Key: "connection", Value: mustStruct(t, map[string]any{
		"source_mode": "m3u_xmltv",
		"m3u_url":     "https://provider.example.com/playlist.m3u",
		"epg_xml_url": "https://provider.example.com/guide.xml",
	})}}}

	if _, err := server.Configure(context.Background(), request); err != nil {
		t.Fatalf("configure: %v", err)
	}
	settings := state.Get()
	if settings.SourceMode != config.SourceModeM3UXMLTV || settings.M3UURL == "" || settings.EPGXMLURL == "" {
		t.Fatalf("expected complete M3U/XMLTV connection to remain available during migration, got %+v", settings)
	}
}

func TestRuntimeConfigureDurableXtreamRegistryOverridesRetiredM3UConnection(t *testing.T) {
	t.Parallel()

	registry := config.NewSourceRegistry(filepath.Join(t.TempDir(), "sources.json"))
	durable := config.XtreamSource{ID: "durable", Name: "Durable", BaseURL: "https://provider.example.com", Username: "demo", Password: "secret", Enabled: true}
	if err := registry.Save([]config.XtreamSource{durable}); err != nil {
		t.Fatalf("save durable source: %v", err)
	}
	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeM3UXMLTV, M3UURL: "https://legacy.example.com/playlist.m3u", EPGXMLURL: "https://legacy.example.com/guide.xml"}}
	server := &runtimeServer{settings: state, sourceRegistry: registry}
	request := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{{Key: "connection", Value: mustStruct(t, map[string]any{
		"source_mode": "m3u_xmltv",
		"m3u_url":     "https://legacy.example.com/playlist.m3u",
		"epg_xml_url": "https://legacy.example.com/guide.xml",
	})}}}

	if _, err := server.Configure(context.Background(), request); err != nil {
		t.Fatalf("configure: %v", err)
	}
	settings := state.Get()
	if settings.SourceMode != config.SourceModeXtream || len(settings.XtreamSources) != 1 || settings.XtreamSources[0].ID != "durable" {
		t.Fatalf("durable XC Admin source must override retired M3U settings, got %+v", settings)
	}
}

func TestRuntimeConfigureDoesNotOverwriteUnreadableRegistryDuringMigration(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sources.json")
	original := []byte("not-json")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write corrupt registry: %v", err)
	}
	registry := config.NewSourceRegistry(path)
	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeXtream}}
	server := &runtimeServer{settings: state, sourceRegistry: registry}
	request := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{{Key: "connection", Value: mustStruct(t, map[string]any{
		"source_mode": "xtream",
		"base_url":    "https://legacy.example.com",
		"username":    "legacy",
		"password":    "secret",
	})}}}

	if _, err := server.Configure(context.Background(), request); err != nil {
		t.Fatalf("configure must keep routes available: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read registry after configure: %v", err)
	}
	if string(after) != string(original) {
		t.Fatalf("migration overwrote unreadable registry: %q", after)
	}
}

func TestSettingsWithRegisteredSourcesSwitchesLegacyM3UStateToXtream(t *testing.T) {
	t.Parallel()

	registry := config.NewSourceRegistry(filepath.Join(t.TempDir(), "sources.json"))
	durable := config.XtreamSource{ID: "provider", Name: "Provider", BaseURL: "https://provider.example.com", Username: "demo", Password: "secret", Enabled: true}
	if err := registry.Save([]config.XtreamSource{durable}); err != nil {
		t.Fatalf("save durable source: %v", err)
	}
	legacy := config.Settings{SourceMode: config.SourceModeM3UXMLTV, M3UURL: "https://legacy.example.com/playlist.m3u", EPGXMLURL: "https://legacy.example.com/guide.xml"}

	got := settingsWithRegisteredSources(legacy, registry)
	if got.SourceMode != config.SourceModeXtream || len(got.XtreamSources) != 1 || got.XtreamSources[0].ID != "provider" {
		t.Fatalf("durable XC Admin sources must immediately become authoritative, got %+v", got)
	}
}

func TestManifestOmitsRetiredGlobalConfiguration(t *testing.T) {
	t.Parallel()

	manifest, err := loadManifest()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if len(manifest.GetGlobalConfigSchema()) != 0 {
		t.Fatalf("expected XC Admin to be the only provider configuration surface, got %+v", manifest.GetGlobalConfigSchema())
	}
}

func TestManifestIdentifiesStandaloneXtremeCodesPlugin(t *testing.T) {
	t.Parallel()

	manifest, err := loadManifest()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if manifest.GetPluginId() != "silo.ramindex.xtream" {
		t.Fatalf("expected standalone plugin id, got %q", manifest.GetPluginId())
	}
	if displayName, _ := manifest.GetMetadata().AsMap()["display_name"].(string); displayName != "XC for Silo" {
		t.Fatalf("expected standalone display name, got %q", displayName)
	}
}

func TestModuleIdentifiesXtremePluginRepository(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !strings.HasPrefix(string(contents), "module github.com/theramindex/silo-plugin-xtream-library\n") {
		t.Fatalf("expected Xtreme module identity, got %q", strings.SplitN(string(contents), "\n", 2)[0])
	}
}

func TestManifestExposesOnlyXtreamPublicRoutes(t *testing.T) {
	t.Parallel()

	manifest, err := loadManifest()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	for _, route := range manifest.GetHttpRoutes() {
		if strings.HasPrefix(route.GetPath(), "/dispatcharr") {
			t.Fatalf("manifest exposes legacy public route %q", route.GetPath())
		}
	}
	for _, route := range manifest.GetHttpRoutes() {
		for _, retired := range []string{"recordings", "sports", "events", "timeshift"} {
			if strings.Contains(route.GetPath(), retired) {
				t.Fatalf("manifest exposes retired Xtreme route %q", route.GetPath())
			}
		}
	}
	for _, expected := range []string{
		"/xtream",
		"/xtream/api/app",
		"/xtream/assets/app.js",
		"/xtream/assets/xc-runtime-a.js",
		"/xtream/assets/xc-runtime-b.js",
	} {
		found := false
		for _, route := range manifest.GetHttpRoutes() {
			found = found || route.GetPath() == expected
		}
		if !found {
			t.Fatalf("manifest is missing public Xtreme route %q", expected)
		}
	}
}

func TestManifestPlayerRuntimeAssetsDoNotRequireBrowserAuthorization(t *testing.T) {
	t.Parallel()

	manifest, err := loadManifest()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	want := map[string]bool{
		"/xtream/assets/xc-runtime-a.js": false,
		"/xtream/assets/xc-runtime-b.js": false,
	}
	for _, route := range manifest.GetHttpRoutes() {
		if _, ok := want[route.GetPath()]; !ok {
			continue
		}
		want[route.GetPath()] = true
		if route.GetAccess() != "public" {
			t.Fatalf("player runtime route %q requires %q access; native script requests cannot attach Silo authorization", route.GetPath(), route.GetAccess())
		}
	}
	for path, found := range want {
		if !found {
			t.Fatalf("manifest is missing player runtime route %q", path)
		}
	}
}

func TestSDKHTTPRouteContractHasNoTypedViewerIdentityOrStreamingBody(t *testing.T) {
	t.Parallel()

	requestType := reflect.TypeOf(pluginv1.HandleHTTPRequest{})
	requestFields := exportedFieldNames(requestType)
	if !reflect.DeepEqual(requestFields, []string{"Body", "Headers", "Method", "Path", "Query"}) {
		t.Fatalf("unexpected SDK request fields: %v", requestFields)
	}

	responseBody, ok := reflect.TypeOf(pluginv1.HandleHTTPResponse{}).FieldByName("Body")
	if !ok {
		t.Fatal("SDK HTTP response must expose a body field")
	}
	if responseBody.Type != reflect.TypeOf([]byte(nil)) {
		t.Fatalf("expected finite []byte SDK response body, got %s", responseBody.Type)
	}
}

func exportedFieldNames(typ reflect.Type) []string {
	fields := make([]string, 0, typ.NumField())
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.PkgPath == "" {
			fields = append(fields, field.Name)
		}
	}
	sort.Strings(fields)
	return fields
}

func TestRuntimeConfigureIgnoresRetiredDispatcharrSourceMode(t *testing.T) {
	t.Parallel()

	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "https://provider.example.com", XtreamUsername: "demo", XtreamPassword: "secret"}}
	server := &runtimeServer{settings: state}
	request := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{
		{Key: "connection", Value: mustStruct(t, map[string]any{
			"source_mode": "direct_login",
			"base_url":    "https://dispatcharr.example.com",
			"username":    "legacy",
			"password":    "legacy-secret",
		})},
	}}

	if _, err := server.Configure(context.Background(), request); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if sourceMode := state.Get().SourceMode; sourceMode != config.SourceModeXtream {
		t.Fatalf("expected legacy source mode to be ignored, got %q", sourceMode)
	}
}

func TestRuntimeConfigureIgnoresInheritedDispatcharrConfigEntry(t *testing.T) {
	t.Parallel()

	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "https://provider.example.com", XtreamUsername: "demo", XtreamPassword: "secret"}}
	server := &runtimeServer{settings: state}
	request := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{
		{Key: "dispatcharr", Value: mustStruct(t, map[string]any{"base_url": "https://legacy.example.com", "username": "legacy", "password": "legacy-secret", "api_key": "legacy-key"})},
	}}

	if _, err := server.Configure(context.Background(), request); err != nil {
		t.Fatalf("configure: %v", err)
	}
	settings := state.Get()
	if settings.DispatcharrURL != "" || settings.DispatcharrUser != "" || settings.DispatcharrPass != "" || settings.DispatcharrAPIKey != "" {
		t.Fatalf("inherited Dispatcharr entry changed standalone settings: %+v", settings)
	}
	if settings.XtreamBaseURL != "https://provider.example.com" || settings.XtreamUsername != "demo" {
		t.Fatalf("inherited entry changed Xtreme settings: %+v", settings)
	}
}

func TestRuntimeConfigureIgnoresRetiredCategorySettings(t *testing.T) {
	t.Parallel()

	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "https://provider.example.com", XtreamUsername: "demo", XtreamPassword: "secret", LiveTVEnabled: true, ChannelRefreshH: config.DefaultChannelRefreshHours, EPGRefreshH: config.DefaultEPGRefreshHours}}
	server := &runtimeServer{settings: state}

	req := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{
		{Key: "category_settings", Value: mustStruct(t, map[string]any{
			"mode":      "delimiter",
			"delimiter": "pipe",
		})},
	}}

	if _, err := server.Configure(context.Background(), req); err != nil {
		t.Fatalf("configure: %v", err)
	}

	if len(state.Get().AdminSettings) != 0 {
		t.Fatalf("retired category settings changed standalone settings: %+v", state.Get())
	}
}

func TestManifestUserPreferenceSchemaAcceptsBrowserPayload(t *testing.T) {
	t.Parallel()

	manifest, err := loadManifest()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	preferences := map[string]any{
		"favorites":           map[string]any{"channel:1": true},
		"favoriteOrder":       []any{"channel:1"},
		"autoFavorites":       map[string]any{},
		"hiddenCategories":    map[string]any{},
		"sportsFavoriteTeams": map[string]any{},
		"keywordPasses":       []any{map[string]any{"id": "pass:1", "keyword": "World Cup", "createdAt": float64(100)}},
		"recentSearches":      []any{"World Cup"},
		"recentChannels":      []any{"channel:1"},
		"continueWatching":    map[string]any{"channel:1": map[string]any{"plays": float64(1)}},
		"playback":            map[string]any{"streamMode": "redirect", "outputFormat": "ts"},
		"categoryParsing":     map[string]any{"enabled": true, "mode": "delimiter", "delimiter": "pipe", "regex": "", "output": ""},
		"customGroups":        []any{map[string]any{"id": "group:news", "name": "News", "order": float64(1)}},
		"customGroupMemberships": map[string]any{
			"group:news": []any{"channel:1"},
		},
	}
	delete(preferences, "autoFavorites")
	delete(preferences, "sportsFavoriteTeams")
	delete(preferences, "keywordPasses")
	delete(preferences, "categoryParsing")
	if err := configsdk.ValidateManifestUserValue(manifest, "preferences", preferences); err != nil {
		t.Fatalf("validate browser preference payload: %v", err)
	}
}

func TestManifestExposesXtremeAdminRoutes(t *testing.T) {
	t.Parallel()

	manifest, err := loadManifest()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	paths := map[string]bool{}
	for _, route := range manifest.GetHttpRoutes() {
		paths[route.GetPath()] = true
		if route.GetPath() == "/xtream" && route.GetNavigationLabel() != "Live TV (XC)" {
			t.Fatalf("expected Xtreme user navigation label, got %q", route.GetNavigationLabel())
		}
		if route.GetPath() == "/xtream/admin" && route.GetNavigationLabel() != "XC Admin" {
			t.Fatalf("expected Xtreme admin navigation label, got %q", route.GetNavigationLabel())
		}
	}
	for _, path := range []string{"/xtream/admin", "/xtream/api/admin-settings", "/xtream/api/admin-sources"} {
		if !paths[path] {
			t.Fatalf("manifest must expose Xtreme admin route %s", path)
		}
	}
}

func TestManifestExposesRefreshTaskCapabilities(t *testing.T) {
	t.Parallel()

	manifest, err := loadManifest()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	scheduledTaskIDs := make([]string, 0)
	for _, capability := range manifest.GetCapabilities() {
		if capability.GetType() == "scheduled_task.v1" {
			scheduledTaskIDs = append(scheduledTaskIDs, capability.GetId())
		}
	}
	want := []string{"xtream-sync", "xtream-refresh-channels", "xtream-refresh-epg"}
	if !reflect.DeepEqual(scheduledTaskIDs, want) {
		t.Fatalf("expected scheduled task capabilities %+v, got %+v", want, scheduledTaskIDs)
	}
}

func mustStruct(t *testing.T, value map[string]any) *structpb.Struct {
	t.Helper()
	result, err := structpb.NewStruct(value)
	if err != nil {
		t.Fatalf("new struct: %v", err)
	}
	return result
}
