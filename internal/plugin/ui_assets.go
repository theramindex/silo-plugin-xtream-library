package plugin

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed ui/page.html ui/styles.css ui/*.js
var playerUIFiles embed.FS

// playerAppScriptParts is the source order of the browser "app.js" asset. The
// app is split into files for maintainability, but it is served (and tested)
// as ONE classic script, so every part shares a single top-level scope.
// core.js must stay first (it declares the route/state globals every other
// part reads), prefs.js second (the settings write queue and prefs sync state
// it declares are read by later top-level code), and app.js last (it ends with
// the load-time event wiring and route restore, which needs every const/let
// above it initialized). The middle parts only declare functions.
var playerAppScriptParts = []string{
	"ui/core.js",
	"ui/prefs.js",
	"ui/sports.js",
	"ui/admin.js",
	"ui/multiview.js",
	"ui/guide.js",
	"ui/player.js",
	"ui/app.js",
}

// playerScriptOrder is the order the page loads its scripts (see page.html);
// "ui/app.js" here is the composed playerAppScriptParts bundle.
var playerScriptOrder = []string{"ui/lineup.js", "ui/app.js"}

// composedUIAssets serves embedded UI files, composing ui/app.js from its parts.
type composedUIAssets struct {
	files embed.FS
}

func (assets composedUIAssets) ReadFile(name string) ([]byte, error) {
	if name != "ui/app.js" {
		return assets.files.ReadFile(name)
	}
	parts := make([]string, 0, len(playerAppScriptParts))
	for _, part := range playerAppScriptParts {
		payload, err := assets.files.ReadFile(part)
		if err != nil {
			return nil, fmt.Errorf("read app script part %s: %w", part, err)
		}
		parts = append(parts, strings.TrimRight(string(payload), "\n"))
	}
	return []byte(strings.Join(parts, "\n") + "\n"), nil
}

var playerUIAssets = composedUIAssets{files: playerUIFiles}

var playerPageHTMLTemplate string

func init() {
	playerPageHTMLTemplate = mustLoadPlayerPageHTMLTemplate()
}

func mustLoadPlayerPageHTMLTemplate() string {
	page, err := playerUIAssets.ReadFile("ui/page.html")
	if err != nil {
		panic(fmt.Errorf("read player page template: %w", err))
	}
	return string(page)
}

func playerAppJavaScript() string {
	parts := make([]string, 0, len(playerScriptOrder))
	for _, name := range playerScriptOrder {
		payload, err := playerUIAssets.ReadFile(name)
		if err != nil {
			return ""
		}
		parts = append(parts, string(payload))
	}
	return strings.Join(parts, "\n")
}

func playerStylesCSS() string {
	styles, err := playerUIAssets.ReadFile("ui/styles.css")
	if err != nil {
		return ""
	}
	return string(styles)
}
