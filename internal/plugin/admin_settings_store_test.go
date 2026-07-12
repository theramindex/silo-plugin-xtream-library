package plugin

import (
	"strings"
	"testing"
)

func TestNormalizeAdminSettingsEventKeywordRuleDefaultsAndClamps(t *testing.T) {
	t.Parallel()

	normalized := normalizeAdminSettingsPayload(map[string]any{
		"eventKeywords": []any{
			map[string]any{
				"categoryId":         "  civic ",
				"categoryName":       " Civic ",
				"keywords":           []any{" State of the Union ", ""},
				"excludeKeywords":    []any{"  rehearsal ", ""},
				"eventSeries":        true,
				"groupWindowMinutes": float64(999),
			},
			map[string]any{
				"categoryId":         "parades",
				"keywords":           []any{"Parade"},
				"groupWindowMinutes": float64(1),
			},
		},
	})

	rules := normalized["eventKeywords"].([]map[string]any)
	if len(rules) != 2 {
		t.Fatalf("expected two normalized rules, got %d", len(rules))
	}
	if rules[0]["excludeKeywords"].([]string)[0] != "rehearsal" || rules[0]["eventSeries"] != true || rules[0]["groupWindowMinutes"] != 360 {
		t.Fatalf("expected first rule options to be preserved/clamped, got %+v", rules[0])
	}
	if rules[1]["eventSeries"] != false || rules[1]["groupWindowMinutes"] != 15 {
		t.Fatalf("expected legacy rule defaults and lower clamp, got %+v", rules[1])
	}
}

func TestNormalizeAdminSettingsJSONRoundTripsEventKeywordRuleOptions(t *testing.T) {
	t.Parallel()

	data, err := normalizeAdminSettingsJSON([]byte(`{"eventKeywords":[{"categoryId":"civic","categoryName":"Civic","keywords":["Debate"],"excludeKeywords":["rerun"],"eventSeries":true,"groupWindowMinutes":45}]}`))
	if err != nil {
		t.Fatalf("normalize settings: %v", err)
	}
	if got := string(data); got == "" {
		t.Fatal("expected normalized settings JSON")
	}
	if !containsJSON(data, `"excludeKeywords":["rerun"]`) || !containsJSON(data, `"eventSeries":true`) || !containsJSON(data, `"groupWindowMinutes":45`) {
		t.Fatalf("expected event keyword options to round-trip, got %s", data)
	}
}

func containsJSON(data []byte, fragment string) bool {
	return strings.Contains(string(data), fragment)
}

func TestNormalizeAdminSettingsClampsLiveRewindLimits(t *testing.T) {
	t.Parallel()
	normalized := normalizeAdminSettingsPayload(map[string]any{
		"liveRewindEnabled":       true,
		"liveRewindCacheGB":       float64(900),
		"liveRewindWindowMinutes": float64(45),
		"liveRewindMinFreeGB":     float64(0),
		"liveRewindMaxChannels":   float64(250),
	})
	if normalized["liveRewindEnabled"] != true {
		t.Fatal("expected rewind to remain enabled")
	}
	if normalized["liveRewindCacheGB"] != float64(500) {
		t.Fatalf("expected cache limit clamp, got %v", normalized["liveRewindCacheGB"])
	}
	if normalized["liveRewindWindowMinutes"] != 30 {
		t.Fatalf("expected invalid window to use default, got %v", normalized["liveRewindWindowMinutes"])
	}
	if normalized["liveRewindMinFreeGB"] != float64(1) {
		t.Fatalf("expected free-space clamp, got %v", normalized["liveRewindMinFreeGB"])
	}
	if normalized["liveRewindMaxChannels"] != 100 {
		t.Fatalf("expected channel limit clamp, got %v", normalized["liveRewindMaxChannels"])
	}
}

func TestNormalizeAdminSettingsSportsFirstPlayerEnabledDefaultsFalse(t *testing.T) {
	t.Parallel()

	if normalized := normalizeAdminSettingsPayload(map[string]any{})["sportsFirstPlayerEnabled"]; normalized != false {
		t.Fatalf("expected sports-first player setting to default to false, got %v", normalized)
	}
}

func TestNormalizeAdminSettingsPreservesSportsFirstPlayerEnabled(t *testing.T) {
	t.Parallel()

	if normalized := normalizeAdminSettingsPayload(map[string]any{"sportsFirstPlayerEnabled": true})["sportsFirstPlayerEnabled"]; normalized != true {
		t.Fatalf("expected sports-first player setting to remain enabled, got %v", normalized)
	}
}
