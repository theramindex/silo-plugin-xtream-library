package config

import (
	"encoding/json"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	sdkconfig "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/config"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestValidate_XtreamRequiresCredentials(t *testing.T) {
	t.Parallel()

	cfg := Settings{SourceMode: SourceModeXtream}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing xtream credentials")
	}
}

func TestValidate_DirectLoginRequiresDispatcharrCredentials(t *testing.T) {
	t.Parallel()

	cfg := Settings{SourceMode: SourceModeDirectLogin}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing Dispatcharr credentials")
	}
}

func TestValidate_M3UXMLTVRequiresURLs(t *testing.T) {
	t.Parallel()

	cfg := Settings{SourceMode: SourceModeM3UXMLTV}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing playlist and epg urls")
	}
}

func TestValidate_EPGRequiredForV1(t *testing.T) {
	t.Parallel()

	cfg := Settings{
		SourceMode: SourceModeM3UXMLTV,
		M3UURL:     "https://example.com/playlist.m3u",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error when epg url is missing")
	}
}

func TestValidate_XtreamConfigPasses(t *testing.T) {
	t.Parallel()

	cfg := Settings{
		SourceMode:      SourceModeXtream,
		XtreamBaseURL:   "https://dispatcharr.example.com",
		XtreamUsername:  "demo",
		XtreamPassword:  "secret",
		LiveTVEnabled:   true,
		ChannelRefreshH: DefaultChannelRefreshHours,
		EPGRefreshH:     DefaultEPGRefreshHours,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid settings, got %v", err)
	}
}

func TestValidate_ExplicitSourceModeWinsOverLegacyAPIKey(t *testing.T) {
	t.Parallel()

	cfg := Settings{
		SourceMode:        SourceModeXtream,
		DispatcharrAPIKey: "legacy-key",
		XtreamBaseURL:     "https://provider.example.com",
		XtreamUsername:    "demo",
		XtreamPassword:    "secret",
		ChannelRefreshH:   DefaultChannelRefreshHours,
		EPGRefreshH:       DefaultEPGRefreshHours,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid settings, got %v", err)
	}
}

func TestValidate_LegacyAPIKeyInfersAPIKeyBeforeDirectLogin(t *testing.T) {
	t.Parallel()

	cfg := Settings{
		DispatcharrURL:    "https://dispatcharr.example.com",
		DispatcharrAPIKey: "admin-api-key",
		ChannelRefreshH:   DefaultChannelRefreshHours,
		EPGRefreshH:       DefaultEPGRefreshHours,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid API key settings, got %v", err)
	}
	if cfg.SourceMode != SourceModeAPIKey {
		t.Fatalf("expected API key source mode, got %q", cfg.SourceMode)
	}
}

func TestValidate_DirectLoginConfigPasses(t *testing.T) {
	t.Parallel()

	cfg := Settings{
		SourceMode:      SourceModeDirectLogin,
		DispatcharrURL:  "https://dispatcharr.example.com",
		DispatcharrUser: "demo",
		DispatcharrPass: "secret",
		LiveTVEnabled:   true,
		ChannelRefreshH: DefaultChannelRefreshHours,
		EPGRefreshH:     DefaultEPGRefreshHours,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid settings, got %v", err)
	}
}

func TestGlobalConfigSchema_ContainsExpectedFields(t *testing.T) {
	t.Parallel()

	schema := GlobalConfigSchema()
	if len(schema) != 1 {
		t.Fatalf("expected one config schema entry, got %d", len(schema))
	}

	byKey := map[string]bool{}
	for _, item := range schema {
		byKey[item.GetKey()] = true
	}

	for _, key := range []string{"connection"} {
		if !byKey[key] {
			t.Fatalf("expected schema key %q", key)
		}
	}
}

func TestGlobalConfigSchemaDirectsAdministratorsToXCAdmin(t *testing.T) {
	t.Parallel()

	connection := mustFindSchema(t, GlobalConfigSchema(), "connection")
	if fields := connection.GetAdminForm().GetFields(); len(fields) != 0 {
		t.Fatalf("expected no legacy configuration fields, got %+v", fields)
	}
	if !strings.Contains(connection.GetDescription(), "XC Admin plugin app") {
		t.Fatalf("expected XC Admin disclaimer, got %q", connection.GetDescription())
	}
	if connection.GetAdminForm().GetSubmitLabel() != "" {
		t.Fatalf("expected informational schema without submit action, got %q", connection.GetAdminForm().GetSubmitLabel())
	}
}

func TestGlobalConfigSchemaAcceptsEmptyLegacyConnection(t *testing.T) {
	t.Parallel()

	connection := mustFindSchema(t, GlobalConfigSchema(), "connection")
	resource, err := jsonschema.UnmarshalJSON(strings.NewReader(connection.GetJsonSchema()))
	if err != nil {
		t.Fatalf("decode connection schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("connection.schema.json", resource); err != nil {
		t.Fatalf("add connection schema: %v", err)
	}
	schema, err := compiler.Compile("connection.schema.json")
	if err != nil {
		t.Fatalf("compile connection schema: %v", err)
	}
	if err := schema.Validate(map[string]any{}); err != nil {
		t.Fatalf("empty legacy connection should be valid after source management moved to XC Admin: %v", err)
	}
}

func TestGlobalConfigSchemaOmitsHostOwnedSchedulingAndDynamicRouteControls(t *testing.T) {
	t.Parallel()

	connection := GlobalConfigSchema()[0]
	for _, removed := range []string{"live_tv_enabled", "channel_refresh_hours", "epg_refresh_hours"} {
		if strings.Contains(connection.GetJsonSchema(), `"`+removed+`"`) {
			t.Fatalf("connection schema exposes unsupported control %q", removed)
		}
		for _, field := range connection.GetAdminForm().GetFields() {
			if field.GetKey() == removed {
				t.Fatalf("admin form exposes unsupported control %q", removed)
			}
		}
	}
}

func TestUserConfigSchema_DeclaresCurrentPreferenceShape(t *testing.T) {
	t.Parallel()

	userSchema := UserConfigSchema()
	if len(userSchema) != 1 {
		t.Fatalf("expected one user config schema entry, got %d", len(userSchema))
	}

	byKey := map[string]bool{}
	for _, item := range userSchema {
		byKey[item.GetKey()] = true
	}
	for _, key := range []string{"preferences"} {
		if !byKey[key] {
			t.Fatalf("expected user schema key %q", key)
		}
	}

	preferences := mustFindSchema(t, UserConfigSchema(), "preferences")
	var schema map[string]any
	if err := json.Unmarshal([]byte(preferences.GetJsonSchema()), &schema); err != nil {
		t.Fatalf("decode preferences schema: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected preferences schema properties, got %q", preferences.GetJsonSchema())
	}
	for _, key := range []string{"favorites", "hiddenCategories", "recentSearches", "recentChannels", "continueWatching", "playback", "customGroups", "customGroupMemberships"} {
		if _, ok := properties[key]; !ok {
			t.Fatalf("expected preferences schema to declare %q", key)
		}
	}
	if _, ok := properties["auto_favorites"]; ok {
		t.Fatal("preferences schema should use the camelCase frontend preference keys")
	}
}

func TestUserConfigSchema_AcceptsProfileSelection(t *testing.T) {
	t.Parallel()
	t.Skip("Dispatcharr profile selection is not part of Xtreme preferences")

	manifest := &pluginv1.PluginManifest{UserConfigSchema: UserConfigSchema()}
	value := map[string]any{
		"profileSelection": map[string]any{
			"mode":       "selected",
			"profileIds": []any{"profile-ny", "profile-arabic"},
		},
	}
	if err := sdkconfig.ValidateManifestUserValue(manifest, "preferences", value); err != nil {
		t.Fatalf("expected profile selection to satisfy the SDK user config schema: %v", err)
	}
}

func TestUserConfigSchema_DeclaresAdminCategorySettingsShape(t *testing.T) {
	t.Parallel()
	t.Skip("retired Dispatcharr admin category settings are not part of Xtreme")

	adminSettings := mustFindSchema(t, UserConfigSchema(), "adminCategorySettings")
	var schema map[string]any
	if err := json.Unmarshal([]byte(adminSettings.GetJsonSchema()), &schema); err != nil {
		t.Fatalf("decode admin category settings schema: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected admin category settings schema properties, got %q", adminSettings.GetJsonSchema())
	}
	for _, key := range []string{"mode", "delimiter", "virtualGroupLabel", "virtualGroupSource", "ecmEnabled", "ecmURL", "allowRecordingsByDefault", "sportsFirstPlayerEnabled", "liveRewindEnabled", "liveRewindCacheGB", "liveRewindWindowMinutes", "liveRewindMinFreeGB", "liveRewindMaxChannels", "collapseDuplicateVirtualGroups", "inferChannelNameGroups", "categoryRenames", "categoryAliases", "eventKeywords"} {
		if _, ok := properties[key]; !ok {
			t.Fatalf("expected admin category settings schema to declare %q", key)
		}
	}
	if property, ok := properties["sportsFirstPlayerEnabled"].(map[string]any); !ok || property["default"] != false {
		t.Fatalf("expected sportsFirstPlayerEnabled schema default to be false, got %+v", properties["sportsFirstPlayerEnabled"])
	}
	if additionalProperties, ok := schema["additionalProperties"].(bool); !ok || additionalProperties {
		t.Fatalf("expected admin category settings schema to reject unknown keys, got %+v", schema["additionalProperties"])
	}
}

func TestUserConfigSchema_DeclaresEventKeywordRuleOptions(t *testing.T) {
	t.Parallel()
	t.Skip("retired Dispatcharr event settings are not part of Xtreme")

	adminSettings := mustFindSchema(t, UserConfigSchema(), "adminCategorySettings")
	var schema map[string]any
	if err := json.Unmarshal([]byte(adminSettings.GetJsonSchema()), &schema); err != nil {
		t.Fatalf("decode admin category settings schema: %v", err)
	}
	properties := schema["properties"].(map[string]any)
	eventKeywords := properties["eventKeywords"].(map[string]any)
	items := eventKeywords["items"].(map[string]any)
	ruleProperties := items["properties"].(map[string]any)

	for _, key := range []string{"excludeKeywords", "eventSeries", "groupWindowMinutes"} {
		if _, ok := ruleProperties[key]; !ok {
			t.Fatalf("expected event keyword rule schema to declare %q", key)
		}
	}
	if property := ruleProperties["eventSeries"].(map[string]any); property["default"] != false {
		t.Fatalf("expected eventSeries default false, got %+v", property["default"])
	}
	if property := ruleProperties["groupWindowMinutes"].(map[string]any); property["default"] != float64(60) || property["minimum"] != float64(15) || property["maximum"] != float64(360) {
		t.Fatalf("expected groupWindowMinutes bounds/default, got %+v", property)
	}
}

func TestGlobalConfigSchema_SecretsAndOptionalLegacyStatus(t *testing.T) {
	t.Parallel()

	schema := GlobalConfigSchema()
	connection := mustFindSchema(t, schema, "connection")

	if !strings.Contains(connection.GetJsonSchema(), "writeOnly") {
		t.Fatalf("expected connection schema to declare writeOnly secret fields, got %q", connection.GetJsonSchema())
	}

	if connection.GetRequired() {
		t.Fatal("expected legacy connection schema to remain optional now that Xtream Codes Admin owns sources")
	}
}

func TestGlobalConfigSchema_UsesObjectSchemasForConfigurePayloads(t *testing.T) {
	t.Parallel()

	connection := mustFindSchema(t, GlobalConfigSchema(), "connection")
	var schema map[string]any
	if err := json.Unmarshal([]byte(connection.GetJsonSchema()), &schema); err != nil {
		t.Fatalf("decode connection schema: %v", err)
	}
	if schema["type"] != "object" {
		t.Fatalf("expected connection schema to be object-shaped, got %q", connection.GetJsonSchema())
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected connection schema properties, got %q", connection.GetJsonSchema())
	}
	for _, key := range []string{"source_mode", "base_url", "username", "password", "live_stream_format", "m3u_url", "epg_xml_url"} {
		if _, ok := properties[key]; !ok {
			t.Fatalf("expected connection schema property %q", key)
		}
	}
}

func TestGlobalConfigSchema_HidesLegacyFieldsFromSiloUI(t *testing.T) {
	t.Parallel()

	connection := mustFindSchema(t, GlobalConfigSchema(), "connection")

	if connection.GetAdminForm() == nil {
		t.Fatal("expected explicit informational admin form")
	}
	if len(connection.GetAdminForm().GetFields()) != 0 {
		t.Fatalf("expected no editable legacy fields, got %+v", connection.GetAdminForm().GetFields())
	}
}

func TestCatalogCacheKeyChangesForDifferentSourceSettings(t *testing.T) {
	t.Parallel()

	direct := Settings{
		SourceMode:      SourceModeDirectLogin,
		DispatcharrURL:  "https://dispatcharr.example.com",
		DispatcharrUser: "demo",
	}
	xtream := Settings{
		SourceMode:     SourceModeXtream,
		XtreamBaseURL:  "https://dispatcharr.example.com",
		XtreamUsername: "demo",
	}

	if CatalogCacheKey(direct) == CatalogCacheKey(xtream) {
		t.Fatal("expected source settings to produce different catalog cache keys")
	}
	if CatalogCacheKey(direct) == "" {
		t.Fatal("expected non-empty catalog cache key")
	}
}

func TestSettingsValidateAcceptsMultipleXtreamSources(t *testing.T) {
	t.Parallel()
	settings := Settings{SourceMode: SourceModeXtream, XtreamSources: []XtreamSource{
		{ID: "primary", Name: "Primary", BaseURL: "https://one.example", Username: "one", Password: "secret", Enabled: true},
		{ID: "backup", Name: "Backup", BaseURL: "https://two.example", Username: "two", Password: "secret", LiveFormat: "ts", Enabled: true},
	}}
	if err := settings.Validate(); err != nil {
		t.Fatalf("validate multiple sources: %v", err)
	}
	if got := settings.EffectiveXtreamSources(); len(got) != 2 || got[1].EffectiveLiveFormat() != "ts" {
		t.Fatalf("unexpected effective sources: %+v", got)
	}
}

func TestSettingsValidateRejectsNoEnabledXtreamSources(t *testing.T) {
	t.Parallel()
	settings := Settings{SourceMode: SourceModeXtream, XtreamSources: []XtreamSource{{ID: "off", BaseURL: "https://one.example", Username: "one", Password: "secret", Enabled: false}}}
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "enabled xtream source") {
		t.Fatalf("expected enabled source validation error, got %v", err)
	}
}

func mustFindSchema(t *testing.T, schema []*ConfigSchema, key string) *ConfigSchema {
	t.Helper()
	for _, item := range schema {
		if item.GetKey() == key {
			return item
		}
	}
	t.Fatalf("missing schema %q", key)
	return nil
}
