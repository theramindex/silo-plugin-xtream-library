package plugin

import (
	"path"
	"strings"
	"testing"
)

// Every ui/*.js file except lineup.js (a separate early script) must be a
// part of the composed app.js bundle, or its functions silently vanish.
func TestPlayerAppScriptPartsCoverEveryUIScript(t *testing.T) {
	t.Parallel()
	entries, err := playerUIFiles.ReadDir("ui")
	if err != nil {
		t.Fatalf("read ui dir: %v", err)
	}
	listed := map[string]bool{}
	for _, part := range playerAppScriptParts {
		listed[part] = true
	}
	for _, entry := range entries {
		name := "ui/" + entry.Name()
		if path.Ext(name) != ".js" || name == "ui/lineup.js" {
			continue
		}
		if !listed[name] {
			t.Errorf("%s is not in playerAppScriptParts", name)
		}
	}
	bundle, err := playerUIAssets.ReadFile("ui/app.js")
	if err != nil {
		t.Fatalf("compose app.js: %v", err)
	}
	for _, part := range playerAppScriptParts {
		payload, err := playerUIFiles.ReadFile(part)
		if err != nil {
			t.Fatalf("read %s: %v", part, err)
		}
		if !strings.Contains(string(bundle), strings.TrimRight(string(payload), "\n")) {
			t.Errorf("composed app.js is missing %s", part)
		}
	}
}
