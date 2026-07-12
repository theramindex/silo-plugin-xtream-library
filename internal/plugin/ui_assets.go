package plugin

import (
	"embed"
	"fmt"
)

//go:embed ui/page.html ui/styles.css ui/lineup.js ui/app.js
var playerUIAssets embed.FS

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
	lineup, err := playerUIAssets.ReadFile("ui/lineup.js")
	if err != nil {
		return ""
	}
	script, err := playerUIAssets.ReadFile("ui/app.js")
	if err != nil {
		return ""
	}
	return string(lineup) + "\n" + string(script)
}

func playerStylesCSS() string {
	styles, err := playerUIAssets.ReadFile("ui/styles.css")
	if err != nil {
		return ""
	}
	return string(styles)
}
