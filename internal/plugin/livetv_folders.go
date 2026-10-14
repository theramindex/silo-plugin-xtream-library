package plugin

import (
	"regexp"
	"strings"

	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

// livetv/v1 channel folders: a Go port of the web app's virtual folder paths
// (virtualPathsForChannel and its helpers in ui/app.js, ui/prefs.js and
// ui/lineup.js), so Silo's Jellyfin bridge browses the same tree the web app
// shows. Keep the two in step. The contract lineup is the same for every
// caller, so the groupCategoriesByPipe preference is served as a second set of
// folders (pipeFolders) that Silo picks for a user who turned it on.

const livetvFolderSeparator = " / "

type livetvFolderSettings struct {
	delimiter      bool // categoryParsing().enabled
	source         string
	collapse       bool // collapseDuplicateVirtualGroups
	aliases        [][2]string
	profileNames   map[string]string
	activeProfile  string // source.channelProfile.id
	categoryLabels map[string]string
}

func (s *HTTPRoutesServer) livetvFolderSettings(source model.Source, categories []model.Category) livetvFolderSettings {
	settings := normalizeAdminSettingsPayload(s.adminSettingsPayload())
	text := func(key string) string { value, _ := settings[key].(string); return value }
	collapse, ok := settings["collapseDuplicateVirtualGroups"].(bool)
	folder := livetvFolderSettings{
		// The web app parses Xtream group names only for a user who turns on
		// groupCategoriesByPipe.
		delimiter:      source.Mode != model.SourceModeXtream && text("mode") == "delimiter",
		source:         text("virtualGroupSource"),
		collapse:       collapse || !ok,
		profileNames:   map[string]string{},
		categoryLabels: map[string]string{},
	}
	if aliases, ok := settings["categoryAliases"].([]map[string]string); ok {
		for _, alias := range aliases {
			folder.aliases = append(folder.aliases, [2]string{alias["sourcePath"], alias["aliasPath"]})
		}
	}
	for _, profile := range source.Profiles {
		if profile.ID != "" {
			folder.profileNames[profile.ID] = profile.Name
		}
	}
	if source.ChannelProfile != nil {
		folder.activeProfile = source.ChannelProfile.ID
	}
	for _, category := range categories {
		folder.categoryLabels[category.ID] = category.Name
	}
	return folder
}

// livetvChannelFolders returns each channel's folders by channel id. Only the
// deepest paths are kept: a folder's parents are implied. A channel with no
// virtual path is listed in its source group's path.
func livetvChannelFolders(settings livetvFolderSettings, channels []model.Channel) map[string][]string {
	tree := livetvFolderTree{livetvFolderSettings: settings}
	out := make(map[string][]string, len(channels))
	for _, channel := range channels {
		paths := tree.virtualPaths(channel)
		if len(paths) == 0 {
			if path := tree.sourceGroupPath(channel); path != "" {
				paths = []string{path}
			}
		}
		out[channel.ID] = livetvLeafFolders(paths)
	}
	return out
}

func livetvLeafFolders(paths []string) []string {
	var leaves []string
	for _, path := range paths {
		parent := false
		for _, other := range paths {
			if strings.HasPrefix(other, path+livetvFolderSeparator) {
				parent = true
				break
			}
		}
		if !parent {
			leaves = append(leaves, path)
		}
	}
	return leaves
}

type livetvFolderTree struct {
	livetvFolderSettings
}

func (t livetvFolderTree) useSourceGroups() bool { return t.source != "channel" }

func (t livetvFolderTree) useChannelNames() bool { return t.source != "group" }

func (t livetvFolderTree) useProfileGroups() bool { return t.source == "profile_group" }

// virtualPaths ports virtualPathsForChannel.
func (t livetvFolderTree) virtualPaths(channel model.Channel) []string {
	if t.useProfileGroups() {
		return livetvUnique(t.profileVirtualPaths(channel))
	}
	var paths []string
	sourcePath := t.sourceVirtualPath(channel)
	if sourcePath != "" && t.useSourceGroups() {
		paths = append(paths, sourcePath)
		paths = append(paths, t.aliasPaths(sourcePath)...)
		if t.source == "group_channel" {
			if combined := t.appendVirtualPathParts(sourcePath, livetvChannelNameFolderParts(channel)); combined != "" {
				paths = append(paths, combined)
				paths = append(paths, t.aliasPaths(combined)...)
			}
		}
	}
	if t.useChannelNames() {
		paths = append(paths, t.inferredChannelNamePaths(channel)...)
	}
	return livetvUnique(paths)
}

func (t livetvFolderTree) profileVirtualPaths(channel model.Channel) []string {
	sourcePath := t.sourceGroupPath(channel)
	if sourcePath == "" {
		return nil
	}
	sourceParts := livetvSplitPath(sourcePath)
	groupPaths := append([]string{sourcePath}, t.aliasPaths(sourcePath)...)
	marketParts := livetvLocalMarketParts(channel, sourceParts)
	var paths []string
	for _, profilePath := range t.profilePaths(channel) {
		paths = append(paths, profilePath)
		for _, groupPath := range groupPaths {
			combined := t.appendVirtualPathParts(profilePath, livetvSplitPath(groupPath))
			if combined != "" {
				paths = append(paths, combined)
			}
			base := combined
			if base == "" {
				base = profilePath + livetvFolderSeparator + groupPath
			}
			if market := t.appendVirtualPathParts(base, marketParts); market != "" {
				paths = append(paths, market)
			}
		}
	}
	return livetvUnique(paths)
}

func (t livetvFolderTree) profilePaths(channel model.Channel) []string {
	ids := channel.ProfileIDs
	if t.activeProfile != "" {
		ids = []string{t.activeProfile}
	}
	var paths []string
	for _, id := range ids {
		name := t.profileNames[id]
		if name == "" {
			name = id
		}
		if path := livetvProfilePath(name); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func livetvProfilePath(name string) string {
	var parts []string
	for _, part := range strings.Split(name, "|") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, livetvFolderSeparator)
}

// sourceLabel ports sourceCategoryLabel: the category's name with a leading
// "*" (featured) removed.
func (t livetvFolderTree) sourceLabel(channel model.Channel) string {
	raw := t.categoryLabels[channel.CategoryID]
	if raw == "" {
		raw = channel.CategoryName
	}
	return livetvCategoryDisplayName(raw)
}

func livetvCategoryDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "*") {
		return strings.TrimSpace(name[1:])
	}
	return name
}

func (t livetvFolderTree) sourceVirtualPath(channel model.Channel) string {
	return strings.Join(t.parsedCategoryPath(t.sourceLabel(channel)), livetvFolderSeparator)
}

func (t livetvFolderTree) sourceGroupPath(channel model.Channel) string {
	if path := t.sourceVirtualPath(channel); path != "" {
		return path
	}
	return livetvCategoryDisplayName(t.sourceLabel(channel))
}

var (
	livetvPipeSplit  = regexp.MustCompile(`\s*\|\s*`)
	livetvSlashSplit = regexp.MustCompile(`\s*/\s*`)
	livetvSpaces     = regexp.MustCompile(`\s+`)
)

func livetvNameToken(value string) string {
	return livetvSpaces.ReplaceAllString(strings.TrimSpace(value), " ")
}

func livetvNonEmptyTokens(parts []string) []string {
	var out []string
	for _, part := range parts {
		if part = livetvNameToken(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// parsedCategoryPath ports parsedCategoryPath: a name splits into a path only
// when delimiter parsing is on and it has more than one part.
func (t livetvFolderTree) parsedCategoryPath(name string) []string {
	name = livetvNameToken(name)
	if !t.delimiter || name == "" {
		return nil
	}
	parts := livetvNonEmptyTokens(livetvPipeSplit.Split(name, -1))
	if len(parts) > 1 {
		return parts
	}
	return nil
}

func (t livetvFolderTree) configuredPath(value string) string {
	display := livetvCategoryDisplayName(value)
	if parts := t.parsedCategoryPath(display); len(parts) > 1 {
		return strings.Join(parts, livetvFolderSeparator)
	}
	if parts := livetvNonEmptyTokens(livetvSlashSplit.Split(display, -1)); len(parts) > 1 {
		return strings.Join(parts, livetvFolderSeparator)
	}
	return ""
}

// aliasPaths ports aliasVirtualPathsForSourcePath.
func (t livetvFolderTree) aliasPaths(sourcePath string) []string {
	normalized := t.configuredPath(sourcePath)
	if normalized == "" {
		normalized = strings.TrimSpace(sourcePath)
	}
	if normalized == "" {
		return nil
	}
	var paths []string
	for _, alias := range t.aliases {
		from, to := t.configuredPath(alias[0]), t.configuredPath(alias[1])
		if from == "" || to == "" {
			continue
		}
		if normalized != from && !strings.HasPrefix(normalized, from+livetvFolderSeparator) {
			continue
		}
		remapped := to
		if normalized != from {
			remapped = to + livetvFolderSeparator + normalized[len(from)+len(livetvFolderSeparator):]
		}
		paths = append(paths, remapped)
	}
	return livetvUnique(paths)
}

func livetvChannelNameParts(channel model.Channel) []string {
	return livetvNonEmptyTokens(strings.Split(channel.Name, "|"))
}

func livetvChannelNameFolderParts(channel model.Channel) []string {
	parts := livetvChannelNameParts(channel)
	if len(parts) < 2 {
		return nil
	}
	return parts[:len(parts)-1]
}

func livetvSplitPath(path string) []string {
	return livetvNonEmptyTokens(strings.Split(path, livetvFolderSeparator))
}

// appendVirtualPathParts ports DispatcharrLineup.appendVirtualPathParts.
func (t livetvFolderTree) appendVirtualPathParts(basePath string, extra []string) string {
	baseParts := livetvSplitPath(basePath)
	additions := livetvNonEmptyTokens(extra)
	if len(baseParts) == 0 || len(additions) == 0 {
		return ""
	}
	if !t.collapse {
		return strings.Join(append(baseParts, additions...), livetvFolderSeparator)
	}
	return livetvAppendUnduplicated(baseParts, additions)
}

func livetvAppendUnduplicated(baseParts, additions []string) string {
	for len(additions) > 0 && len(baseParts) > 0 && strings.EqualFold(baseParts[len(baseParts)-1], additions[0]) {
		additions = additions[1:]
	}
	if len(additions) == 0 {
		return ""
	}
	return strings.Join(append(append([]string(nil), baseParts...), additions...), livetvFolderSeparator)
}

var livetvUSStates = map[string]bool{
	"AL": true, "AK": true, "AZ": true, "AR": true, "CA": true, "CO": true, "CT": true, "DE": true, "FL": true, "GA": true,
	"HI": true, "ID": true, "IL": true, "IN": true, "IA": true, "KS": true, "KY": true, "LA": true, "ME": true, "MD": true,
	"MA": true, "MI": true, "MN": true, "MS": true, "MO": true, "MT": true, "NE": true, "NV": true, "NH": true, "NJ": true,
	"NM": true, "NY": true, "NC": true, "ND": true, "OH": true, "OK": true, "OR": true, "PA": true, "RI": true, "SC": true,
	"SD": true, "TN": true, "TX": true, "UT": true, "VT": true, "VA": true, "WA": true, "WV": true, "WI": true, "WY": true,
	"DC": true,
}

var (
	livetvInternationalSports = regexp.MustCompile(`\b(sport|sports|team|teams|league|football|soccer|futbol|fútbol|mlb|nba|nfl|nhl|mls|f1)\b`)
	livetvInternationalNews   = regexp.MustCompile(`\b(news|noticias|nouvelles)\b`)
	livetvInternationalKids   = regexp.MustCompile(`\b(kids|children|cartoon|junior)\b`)
)

// inferredChannelNamePaths ports inferredChannelNameGroupPaths: "NY | WABC"
// files under US TV / Locals / NY, anything else under an International root.
func (t livetvFolderTree) inferredChannelNamePaths(channel model.Channel) []string {
	parts := livetvChannelNameParts(channel)
	if len(parts) < 2 {
		return nil
	}
	first, second := parts[0], parts[1]
	if livetvUSStates[strings.ToUpper(first)] {
		state := strings.ToUpper(first)
		return livetvUnique([]string{"US TV / Locals / " + state, "US TV / Locals / " + state + " / " + second})
	}
	label := t.sourceLabel(channel)
	text := strings.ToLower(strings.Join(parts, " ") + " " + label + " " + label)
	root := "International TV"
	switch {
	case livetvInternationalSports.MatchString(text):
		root = "International Sports"
	case livetvInternationalNews.MatchString(text):
		root = "International News"
	case livetvInternationalKids.MatchString(text):
		root = "International Kids"
	}
	return livetvUnique([]string{root + " / " + first, root + " / " + first + " / " + second})
}

func livetvLocalMarketParts(channel model.Channel, sourceParts []string) []string {
	if len(sourceParts) == 0 {
		return nil
	}
	tail := strings.ToLower(sourceParts[len(sourceParts)-1])
	if tail != "local" && tail != "locals" {
		return nil
	}
	parts := livetvChannelNameParts(channel)
	if len(parts) < 2 {
		return nil
	}
	if livetvUSStates[strings.ToUpper(parts[0])] {
		return parts[1:2]
	}
	return parts[:1]
}

func livetvUnique(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
