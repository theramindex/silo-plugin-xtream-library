package main

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	configsdk "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/config"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestRuntimeConfigureReadsObjectShapedConfigEntries(t *testing.T) {
	t.Parallel()

	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeXtream, LiveTVEnabled: true, ChannelRefreshH: config.DefaultChannelRefreshHours, EPGRefreshH: config.DefaultEPGRefreshHours}}
	server := &runtimeServer{settings: state}

	req := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{
		{Key: "connection", Value: mustStruct(t, map[string]any{"source_mode": "xtream", "base_url": "https://provider.example.com", "username": "demo", "password": "secret", "live_tv_enabled": true})},
	}}

	if _, err := server.Configure(context.Background(), req); err != nil {
		t.Fatalf("configure: %v", err)
	}

	settings := state.Get()
	if settings.SourceMode != config.SourceModeXtream {
		t.Fatalf("expected source mode to update, got %q", settings.SourceMode)
	}
	if settings.XtreamBaseURL == "" || settings.XtreamUsername == "" || settings.XtreamPassword == "" {
		t.Fatalf("expected xtream connection to be loaded, got %+v", settings)
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
	if displayName, _ := manifest.GetMetadata().AsMap()["display_name"].(string); displayName != "Xtreme Codes for Silo" {
		t.Fatalf("expected standalone display name, got %q", displayName)
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

func TestRuntimeConfigurePreservesSecretsOmittedByHost(t *testing.T) {
	t.Parallel()

	state := &settingsState{settings: config.Settings{
		SourceMode:        config.SourceModeAPIKey,
		DispatcharrURL:    "https://dispatcharr.example.com",
		DispatcharrAPIKey: "existing-secret",
		ChannelRefreshH:   config.DefaultChannelRefreshHours,
		EPGRefreshH:       config.DefaultEPGRefreshHours,
	}}
	server := &runtimeServer{settings: state}
	request := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{
		{Key: "connection", Value: mustStruct(t, map[string]any{
			"source_mode":     "api_key",
			"base_url":        "https://dispatcharr.example.com",
			"channel_profile": "The Ramindex | NY",
		})},
	}}

	if _, err := server.Configure(context.Background(), request); err != nil {
		t.Fatalf("configure: %v", err)
	}
	settings := state.Get()
	if settings.DispatcharrAPIKey != "existing-secret" {
		t.Fatalf("omitted secret was erased: %+v", settings)
	}
	if settings.ChannelProfile != "The Ramindex | NY" {
		t.Fatalf("expected present profile field to update: %+v", settings)
	}
}

func TestRuntimeConfigureMapsXtreamSharedConnectionFields(t *testing.T) {
	t.Parallel()

	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeDirectLogin, LiveTVEnabled: true, ChannelRefreshH: config.DefaultChannelRefreshHours, EPGRefreshH: config.DefaultEPGRefreshHours}}
	server := &runtimeServer{settings: state}

	req := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{
		{Key: "connection", Value: mustStruct(t, map[string]any{
			"source_mode":     "xtream",
			"base_url":        "https://dispatcharr.example.com",
			"username":        "xc-user",
			"password":        "xc-pass",
			"epg_xml_url":     "https://dispatcharr.example.com/xmltv.php?username=xc-user&password=xc-pass",
			"live_tv_enabled": true,
		})},
	}}

	if _, err := server.Configure(context.Background(), req); err != nil {
		t.Fatalf("configure: %v", err)
	}

	settings := state.Get()
	if settings.SourceMode != config.SourceModeXtream {
		t.Fatalf("expected xtream source mode, got %q", settings.SourceMode)
	}
	if settings.XtreamBaseURL != "https://dispatcharr.example.com" || settings.XtreamUsername != "xc-user" || settings.XtreamPassword != "xc-pass" {
		t.Fatalf("expected xtream connection to be loaded, got %+v", settings)
	}
	if settings.EPGXMLURL == "" {
		t.Fatalf("expected custom xmltv url to be saved, got %+v", settings)
	}
}

func TestRuntimeConfigureRejectsLegacyDispatcharrSourceMode(t *testing.T) {
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

func TestRuntimeConfigureMapsM3UXMLTVFromConnectionEntry(t *testing.T) {
	t.Parallel()

	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeDirectLogin, LiveTVEnabled: true, ChannelRefreshH: config.DefaultChannelRefreshHours, EPGRefreshH: config.DefaultEPGRefreshHours}}
	server := &runtimeServer{settings: state}

	req := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{
		{Key: "connection", Value: mustStruct(t, map[string]any{
			"source_mode": "m3u_xmltv",
			"m3u_url":     "https://provider.example.com/playlist.m3u",
			"epg_xml_url": "https://provider.example.com/guide.xml",
		})},
	}}

	if _, err := server.Configure(context.Background(), req); err != nil {
		t.Fatalf("configure: %v", err)
	}

	settings := state.Get()
	if settings.SourceMode != config.SourceModeM3UXMLTV || settings.M3UURL == "" || settings.EPGXMLURL == "" {
		t.Fatalf("expected m3u/xmltv connection to be loaded, got %+v", settings)
	}
}

func TestRuntimeConfigureReadsCategorySettings(t *testing.T) {
	t.Parallel()

	state := &settingsState{settings: config.Settings{SourceMode: config.SourceModeDirectLogin, LiveTVEnabled: true, ChannelRefreshH: config.DefaultChannelRefreshHours, EPGRefreshH: config.DefaultEPGRefreshHours}}
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

	var settings map[string]any
	if err := json.Unmarshal(state.Get().AdminSettings, &settings); err != nil {
		t.Fatalf("decode admin settings: %v", err)
	}
	if settings["mode"] != "delimiter" || settings["delimiter"] != "pipe" {
		t.Fatalf("expected category settings to be loaded, got %+v", settings)
	}
}

func TestManifestGlobalConfigSchemasValidateExpectedObjects(t *testing.T) {
	t.Parallel()

	manifest, err := loadManifest()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if err := configsdk.ValidateManifestGlobalValue(manifest, "connection", map[string]any{"source_mode": "xtream", "base_url": "https://provider.example.com", "username": "demo", "password": "secret", "epg_xml_url": "https://provider.example.com/guide.xml"}); err != nil {
		t.Fatalf("validate xtream connection schema: %v", err)
	}
	if err := configsdk.ValidateManifestGlobalValue(manifest, "connection", map[string]any{"source_mode": "m3u_xmltv", "m3u_url": "https://provider.example.com/playlist.m3u", "epg_xml_url": "https://provider.example.com/guide.xml"}); err != nil {
		t.Fatalf("validate m3u/xmltv connection schema: %v", err)
	}
	if err := configsdk.ValidateManifestGlobalValue(manifest, "connection", map[string]any{"source_mode": "m3u_xmltv", "m3u_url": "https://provider.example.com/playlist.m3u"}); err == nil {
		t.Fatalf("expected incomplete m3u/xmltv connection to fail validation")
	}
	if err := configsdk.ValidateManifestGlobalValue(manifest, "connection", map[string]any{"source_mode": "direct_login", "base_url": "https://provider.example.com", "username": "demo", "password": "secret"}); err == nil {
		t.Fatal("expected legacy Dispatcharr source mode to fail validation")
	}
	if err := configsdk.ValidateManifestGlobalValue(manifest, "category_settings", map[string]any{
		"mode":                           "delimiter",
		"delimiter":                      "pipe",
		"virtualGroupSource":             "profile_group",
		"collapseDuplicateVirtualGroups": true,
		"ecmEnabled":                     false,
		"ecmURL":                         "https://ecm.example.test/manage",
		"categoryAliases": []any{
			map[string]any{"sourcePath": "International | Arabic | Sports", "aliasPath": "Sports | Arabic"},
			map[string]any{"sourcePath": "International | Arabic | Sports", "aliasPath": "World Cup | Arabic"},
		},
		"eventKeywords": []any{
			map[string]any{"categoryId": "entertainment", "categoryName": "Entertainment", "keywords": []any{"Festival"}},
		},
	}); err != nil {
		t.Fatalf("validate category settings schema: %v", err)
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
	if err := configsdk.ValidateManifestUserValue(manifest, "preferences", preferences); err != nil {
		t.Fatalf("validate browser preference payload: %v", err)
	}
}

func TestManifestExposesAdminNavigationRoute(t *testing.T) {
	t.Parallel()

	manifest, err := loadManifest()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	for _, route := range manifest.GetHttpRoutes() {
		if route.GetPath() != "/dispatcharr/admin" {
			continue
		}
		if !route.GetNavigable() || route.GetNavigationKind() != "admin" || route.GetNavigationLabel() != "Live TV Admin" || route.GetAccess() != "admin" {
			t.Fatalf("unexpected admin route metadata: %+v", route)
		}
		return
	}
	t.Fatalf("expected manifest to expose /dispatcharr/admin as a navigable admin route")
}

func TestManifestExposesAdminSettingsAPIRoutes(t *testing.T) {
	t.Parallel()

	manifest, err := loadManifest()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	found := map[string]bool{}
	for _, route := range manifest.GetHttpRoutes() {
		if route.GetPath() != "/dispatcharr/api/admin-settings" {
			continue
		}
		expectedAccess := "authenticated"
		if route.GetMethod() == http.MethodPost {
			expectedAccess = "admin"
		}
		if route.GetAccess() != expectedAccess || route.GetNavigable() {
			t.Fatalf("unexpected admin settings route metadata: %+v", route)
		}
		found[route.GetMethod()] = true
	}
	if !found["GET"] || !found["POST"] {
		t.Fatalf("expected manifest to expose admin settings GET and POST routes, got %+v", found)
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
	want := []string{"dispatcharr-sync", "dispatcharr-refresh-channels", "dispatcharr-refresh-epg"}
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
