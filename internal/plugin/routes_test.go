package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/cache"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestHTTPRoutesServerStatusRoute(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	source := model.LiveTVSource(model.SourceModeDirectLogin)
	source.ProfileAccess = &model.ProfileAccess{Status: "available", ProfileCount: 2, ChannelMembershipCount: 8}
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source:   source,
			Channels: []model.Channel{{ID: "xtream:1", Name: "News HD"}},
			Programs: []model.Program{{ID: "program:1", ChannelID: "xtream:1", Title: "Morning News", StartUnix: 1700000000}},
		},
		Health: model.SyncHealth{LastSuccessUnix: 123},
	})
	server := NewHTTPRoutesServer(store)

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/status"})
	if err != nil {
		t.Fatalf("handle route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}

	var payload HealthPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.SourceName != "Live TV" || payload.ChannelCount != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.ProfileAccess == nil || payload.ProfileAccess.ProfileCount != 2 || payload.ProfileAccess.ChannelMembershipCount != 8 {
		t.Fatalf("expected profile access summary, got %+v", payload.ProfileAccess)
	}
}

func TestHTTPRoutesServerChannelsAndGuideRoutes(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source: model.LiveTVSource(model.SourceModeXtream),
			Channels: []model.Channel{
				{ID: "xtream:1", Name: "News HD"},
			},
			Programs: []model.Program{
				{ID: "program:2", ChannelID: "xtream:1", Title: "Late News", StartUnix: 1700003600},
				{ID: "program:1", ChannelID: "xtream:1", Title: "Morning News", StartUnix: 1700000000},
			},
		},
	})
	server := NewHTTPRoutesServer(store)

	channelsResponse, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/channels"})
	if err != nil {
		t.Fatalf("channels route: %v", err)
	}
	if channelsResponse.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", channelsResponse.GetStatusCode())
	}
	var channelsPayload ChannelsPayload
	if err := json.Unmarshal(channelsResponse.GetBody(), &channelsPayload); err != nil {
		t.Fatalf("unmarshal channels payload: %v", err)
	}
	if len(channelsPayload.Channels) != 1 || channelsPayload.Channels[0].Name != "News HD" {
		t.Fatalf("unexpected channels payload: %+v", channelsPayload)
	}

	query, _ := structpb.NewStruct(map[string]any{"channel_id": "xtream:1"})
	guideResponse, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/guide", Query: query})
	if err != nil {
		t.Fatalf("guide route: %v", err)
	}
	if guideResponse.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", guideResponse.GetStatusCode())
	}
	var guidePayload GuidePayload
	if err := json.Unmarshal(guideResponse.GetBody(), &guidePayload); err != nil {
		t.Fatalf("unmarshal guide payload: %v", err)
	}
	if len(guidePayload.Programs) != 2 || guidePayload.Programs[0].Title != "Morning News" {
		t.Fatalf("unexpected guide payload: %+v", guidePayload)
	}
}

func TestHTTPRoutesServerAppRouteIncludesAppLayerPayload(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source: model.LiveTVSource(model.SourceModeXtream),
			Channels: []model.Channel{
				{ID: "xtream:1", Name: "News HD", CategoryID: "10", CategoryName: "News", StreamURL: "https://provider.example/live/demo/secret/1.ts"},
			},
			Programs: []model.Program{
				{ID: "program:1", ChannelID: "xtream:1", Title: "Morning News", StartUnix: 100, EndUnix: 200},
			},
			Content: model.ContentState{
				LiveCategories: []model.Category{{ID: "10", Name: "News", Kind: "live"}},
				VODCategories:  []model.Category{{ID: "movies", Name: "Movies", Kind: "vod"}},
				VODItems:       []model.VODItem{{ID: "vod:2001", Name: "Example Movie", Container: "mp4", StreamURL: "https://provider.example/movie/demo/secret/2001.mp4"}},
				SeriesItems:    []model.SeriesItem{{ID: "series:3001", Name: "Example Series"}},
			},
		},
	})
	server := NewHTTPRoutesServer(store)

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/api/app"})
	if err != nil {
		t.Fatalf("app route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	if !strings.Contains(string(response.GetBody()), `"id":"xtream:1"`) {
		t.Fatalf("expected lower-case JSON field names, got %s", string(response.GetBody()))
	}
	var payload AppPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal app payload: %v", err)
	}
	if !payload.Capabilities.LiveTV || payload.Capabilities.NativeLiveTVExport || payload.Capabilities.Recordings {
		t.Fatalf("unexpected capabilities: %+v", payload.Capabilities)
	}
	if len(payload.Categories) != 1 || len(payload.Channels) != 1 {
		t.Fatalf("unexpected app payload: %+v", payload)
	}
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(response.GetBody(), &topLevel); err != nil {
		t.Fatalf("unmarshal app payload fields: %v", err)
	}
	for _, field := range []string{"programs", "vod", "series"} {
		if _, exists := topLevel[field]; exists {
			t.Fatalf("app bootstrap payload must not include supplemental field %q", field)
		}
	}
	if payload.Channels[0].StreamFormat != "mpegts" {
		t.Fatalf("expected a non-secret playback format hint, got %+v", payload.Channels[0])
	}
	if strings.Contains(string(response.GetBody()), "provider.example") || strings.Contains(string(response.GetBody()), "secret") {
		t.Fatalf("app payload exposed provider credentials: %s", string(response.GetBody()))
	}
	if strings.Contains(string(response.GetBody()), `"sessions"`) || strings.Contains(string(response.GetBody()), `"preferences"`) {
		t.Fatalf("app payload must be user-neutral: %s", string(response.GetBody()))
	}
}

func TestCatalogSnapshotMatchesAPIKeyDirectAppMode(t *testing.T) {
	t.Parallel()

	settings := config.Settings{SourceMode: config.SourceModeAPIKey, DispatcharrURL: "https://dispatcharr.example", DispatcharrAPIKey: "secret"}
	snapshot := cache.Snapshot{
		Catalog:   model.CatalogState{Source: model.LiveTVSource(model.SourceModeDirectLogin), Channels: []model.Channel{{ID: "channel:1"}}},
		ConfigKey: config.CatalogCacheKey(settings),
	}
	if !catalogSnapshotMatchesSettings(snapshot, settings) {
		t.Fatal("API key catalog should match the shared Direct app source mode")
	}
}

func TestProfileCatalogNeedsRefreshForUnavailableDispatcharrSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := cache.Snapshot{Catalog: model.CatalogState{Source: model.LiveTVSource(model.SourceModeDirectLogin)}}
	snapshot.Catalog.Source.ProfileAccess = &model.ProfileAccess{Status: "unavailable"}
	apiKeySettings := config.Settings{SourceMode: config.SourceModeAPIKey}
	if !profileCatalogNeedsRefresh(snapshot, apiKeySettings) {
		t.Fatal("expected unavailable API key profile snapshot to need refresh")
	}
	directSettings := config.Settings{SourceMode: config.SourceModeDirectLogin}
	if !profileCatalogNeedsRefresh(snapshot, directSettings) {
		t.Fatal("expected direct-login profile snapshots to self-heal too")
	}
	snapshot.Catalog.Source.ProfileAccess.Status = "available"
	if profileCatalogNeedsRefresh(snapshot, apiKeySettings) {
		t.Fatal("expected available API key profile snapshot to remain warm")
	}
}

func TestPublicStreamFormatUsesUpstreamPathWithoutExposingIt(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"https://dispatcharr.example/proxy/ts/stream/channel-id":  "mpegts",
		"https://provider.example/live/channel.m3u8?token=secret": "hls",
		"https://provider.example/live/channel.ts":                "mpegts",
		"https://provider.example/live/channel":                   "",
	}
	for rawURL, expected := range tests {
		if actual := publicStreamFormat(rawURL); actual != expected {
			t.Fatalf("publicStreamFormat(%q) = %q, want %q", rawURL, actual, expected)
		}
	}
}

func TestHTTPRoutesServerAppPageIncludesVirtualFolderDrilldown(t *testing.T) {
	t.Parallel()

	response, err := NewHTTPRoutesServer(cache.NewStore()).Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: "GET",
		Path:   "/dispatcharr",
		Query:  &structpb.Struct{Fields: map[string]*structpb.Value{"theme": structpb.NewStringValue("midnight-cinema")}},
	})
	if err != nil {
		t.Fatalf("app route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	if response.GetHeaders()["cache-control"] != "no-store" {
		t.Fatalf("expected app shell to disable browser caching, got %q", response.GetHeaders()["cache-control"])
	}
	if !strings.Contains(string(response.GetBody()), `src="dispatcharr/assets/app.js?v=`) || strings.Contains(string(response.GetBody()), "__ASSET_VERSION__") {
		t.Fatalf("expected root app shell to reference versioned assets: %s", string(response.GetBody()))
	}
	body := string(response.GetBody()) + "\n" + playerAppJavaScript() + "\n" + playerStylesCSS()
	for _, want := range []string{
		`function sourceVirtualChildCategories(parentPath, includeChannel)`,
		`function featuredChildCategories(parentPath, includeChannel)`,
		`function virtualCategoriesFromPaths(parentPath, includeChannel, includeAllDescendants)`,
		`function featuredCategoriesFromPaths(parentPath, includeChannel, includeAllDescendants)`,
		`function guideFilterCategories()`,
		`featuredCategoriesFromPaths("", includeChannel, true)`,
		`virtualCategoriesFromPaths("", includeChannel, true)`,
		`const categories = guideFilterCategories();`,
		`if (state.category.indexOf("virtual:") === 0 || state.category.indexOf("featured:") === 0)`,
		`const children = (featured ? featuredChildCategories : virtualChildCategories)(path,`,
		`virtualFolderBreadcrumbs(path, featured)`,
		`const rootLabel = featured ? featuredGroupLabel() : virtualGroupLabel()`,
		`function featuredGroupLabel()`,
		`function allGroupLabel()`,
		`data-admin-category-field=\"virtualGroupLabel\"`,
		`const showSourceCategorySettings = !virtualCategoriesActive()`,
		`aria-label="Live TV sections"`,
		`<span>Guide</span>`,
		`<span>On Later</span>`,
		`Favorites <small id="favorite-count">0</small>`,
		`<span>Sports</span>`,
		`<span>Events</span>`,
		`id="settings-menu-button"`,
		`class="settings-dropdown"`,
		`data-view="settings" role="menuitem">Settings</button>`,
		`Refresh guide</button>`,
		`profileSelection: { mode: "all", profileIds: [] }`,
		`function renderProfileSettings()`,
		`function updateSelectedProfile(profileID, enabled)`,
		`function channelMatchesProfileSelection(channel)`,
		`data-profile-selection-id=`,
		`Use all profiles`,
		`.profile-selection-list`,
		`id="sports-topbar-tabs"`,
		`id="app-search-button"`,
		`data-view="search"`,
		`recentSearches: []`,
		`function renderSearchPage()`,
		`function renderSearchResults(query)`,
		`const SEARCH_RESULTS_DELAY_MS = 180;`,
		`function updateSearchPageResults()`,
		`function scheduleSearchResultsUpdate()`,
		`function refreshGuideRowsForQuery()`,
		`function updateLiveSearchFilter()`,
		`id=\"search-page-results\"`,
		`function renderOnLaterPage()`,
		`function groupedUpcomingAirings(programs, query)`,
		`function programIsGuidePlaceholder(program)`,
		`no games? today`,
		`function rememberSearch(value)`,
		`function folderFilterHTML(placeholder, actionsHTML)`,
		`id=\"folder-filter\"`,
		`onLaterType`,
		`data-search-recent=`,
		`data-search-type=`,
		`data-search-channel=`,
		`data-search-category=`,
		`data-search-program-channel=`,
		`data-keyword-pass-add=`,
		`keywordPasses`,
		`allowRecordingsByDefault`,
		`recordingCapability: null`,
		`function recordingSchedulingEnabled()`,
		`/dispatcharr/api/recordings/capability`,
		`Scheduling requires a Dispatcharr admin account or Admin API Key.`,
		`Search movies, tv shows, channels and more`,
		`function renderSportsPage()`,
		`function renderSportsTopbarTabs()`,
		`error.status = response.status`,
		`Your Silo session expired. Refresh the page or sign in again.`,
		`function compareSportsEventsForTab(left, right)`,
		`return rightRecent - leftRecent;`,
		`sports-channel-logo`,
		`function renderEventsPage()`,
		`/dispatcharr/api/events`,
		`data-event-tab=`,
		`function renderMultiviewPage()`,
		`function addChannelToMultiview(channel)`,
		`function syncMultiviewAudio()`,
		`multiviewQuery`,
		`function multiviewCandidateChannels(limit)`,
		`id=\"multiview-picker\"`,
		`Search channels or programs`,
		`picker.outerHTML = renderMultiviewPicker()`,
		`data-multiview-channel=`,
		`data-player-action=\"add-multiview\"`,
		`/dispatcharr/api/sports`,
		`data-sports-tab=`,
		`sportsFavoriteTeams`,
		`const isAdminRoute = path.endsWith("/dispatcharr/admin")`,
		`if (state.view === "admin" && !isAdminRoute) state.view = "home"`,
		`delimiter: "pipe"`,
		`if (!settings.delimiter) settings.delimiter = "pipe"`,
		`function renderVirtualCategoryGuide(channels)`,
		`function renderVirtualCategoryViewToggle()`,
		`function renderVirtualCategoryChannelList(channels)`,
		`function renderVirtualCategoryContent(channels)`,
		`function setVirtualCategoryView(view)`,
		`renderVirtualCategoryGuide(channels)`,
		`function categoryTileHTML(category)`,
		`.tile strong { display: -webkit-box;`,
		`-webkit-line-clamp: 2`,
		`data-virtual-category-view=\"guide\"`,
		`data-virtual-category-view=\"list\"`,
		`No channels in this virtual group yet.`,
		`function isRewindableChannel(channel)`,
		`video.controls = rewindable`,
		`isLive: !rewindable`,
		`data-silo-theme="midnight-cinema"`,
		`function applySiloTheme()`,
		`--silo-bg`,
		`const appCacheKey = "silo.ramindex.dispatcharr.appSnapshot.v1." + localCacheSuffix`,
		`function readLocalAppCache()`,
		`function writeLocalAppCache(payload)`,
		`await hydrateApp(cached, { localCache: true })`,
		`Showing saved guide. Refresh failed.`,
		`function rebuildProgramIndex()`,
		`.overflow-tooltip`,
		`data-overflow-description=\"true\"`,
		`data-overflow-tooltip=\"`,
		`function descriptionOverflows(target)`,
		`function showOverflowTooltip(target, event)`,
		`if (!descriptionOverflows(target)) return;`,
		`.logo-fallback`,
		`function channelLogoFallback(channel)`,
		`onerror=\"this.hidden = true; this.nextElementSibling.hidden = false;\"`,
		`<span class=\"epg-channel-title\">`,
		`title=\"" + escapeHTML(channelName) + "\"`,
		`data-channel-name=\"`,
		`content: attr(data-channel-name)`,
		`.epg-channel:hover::after`,
		`.epg-channel:focus-visible::after`,
		`function renderEPGGapCell(channel, startUnix, endUnix, windowInfo)`,
		`class=\"epg-cell program epg-gap\"`,
		`program" + (isLive ? " live" : "")`,
		`function epgProgramTitleParts(title)`,
		`class=\"epg-live-marker\" aria-hidden=\"true\"`,
		`.epg-live-marker { margin-left: 0.24rem;`,
		`data-program-detail-channel=`,
		`function renderProgramDetailsModal()`,
		`class=\"program-modal\"`,
		`aria-labelledby=\"program-modal-title\" aria-describedby=\"program-modal-description\"`,
		`.program-modal-art .logo { position: absolute; left: 50%; top: 50%;`,
		`.program-modal-tags span.is-live { background: #b42318;`,
		`tag === "Live now" ? " class=\"is-live\""`,
		`shell.setAttribute("inert", "")`,
		`function trapProgramModalFocus(event)`,
		`if (start > cursor) cells.push(renderEPGGapCell(channel, cursor, start, windowInfo));`,
		`customGroupChannelID`,
		`role=\"combobox\"`,
		`role=\"listbox\"`,
		`data-custom-group-channel-option=`,
		`function selectCustomGroupChannel(channelID)`,
		`function tickGuideAutoRefresh()`,
		`state.guideAutoTimer = setInterval(tickGuideAutoRefresh, 60000);`,
		`now - state.guideLastAutoFetchAt < 5 * 60 * 1000`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected app page to include virtual folder drilldown marker %q", want)
		}
	}
	if strings.Contains(body, `id=\"custom-group-channel\"><option`) {
		t.Fatalf("expected custom group channel picker not to render a native select")
	}
	if !strings.Contains(body, `data-sports-refresh`) {
		t.Fatalf("expected sports scores to have a dedicated refresh action")
	}
	if strings.Contains(body, `<span>Multiview</span>`) || strings.Contains(body, `sports-channel-multiview`) {
		t.Fatalf("expected multiview controls to be hidden from navigation and sports cards")
	}
	if strings.Contains(body, `postJSON("/dispatcharr/api/sports/favorites"`) {
		t.Fatalf("expected sports favorite teams to save through user profile preferences")
	}
	if strings.Contains(body, `colorClass(`) {
		t.Fatalf("expected guide colors to be semantic, not rotated by position")
	}
	if !strings.Contains(body, `const recent = recentChannels(5);`) {
		t.Fatalf("expected home guide to be based on up to 5 continue-watching channels")
	}
	if !strings.Contains(body, `return pool.filter(channelHasCurrentGuide).slice(0, 5);`) {
		t.Fatalf("expected home guide preview to be capped at 5 channels")
	}
	if !strings.Contains(body, `const watched = recent.length ? recent : visibleChannels(false).slice(0, 5);`) ||
		!strings.Contains(body, `+ (favorites.length ? sectionHeader("Favorites") + favoriteHomeCards(favorites) : "")`) ||
		!strings.Contains(body, `+ sectionHeaderWithActions("TV Guide", guideFreshnessHTML())`) ||
		!strings.Contains(body, `+ renderHomeGuide(homeGuideChannels(watched), "No current guide data for recently watched channels.", { hideFreshness: true })`) ||
		!strings.Contains(body, `+ categoryGrid();`) {
		t.Fatalf("expected home page order to be continue watching, favorites, guide grid, then group sections")
	}
	virtualHeaderIndex := strings.Index(body, `byId("view").innerHTML = virtualFolderHeader(path, featured)`)
	virtualFilterIndex := strings.Index(body, `+ folderFilterHTML("Filter this folder", "")`)
	virtualChildrenIndex := strings.Index(body, `+ (filteredChildren.length ? "<div class=\"category-grid\">`)
	virtualContentIndex := strings.Index(body, `+ renderVirtualCategoryContent(filteredChannels)`)
	if virtualHeaderIndex < 0 || virtualFilterIndex < 0 || virtualChildrenIndex < 0 || virtualContentIndex < 0 {
		t.Fatalf("expected virtual category drilldown to render breadcrumbs, filter, subfolders, and switchable channel content")
	}
	if !(virtualHeaderIndex < virtualFilterIndex && virtualFilterIndex < virtualChildrenIndex && virtualChildrenIndex < virtualContentIndex) {
		t.Fatalf("expected virtual category drilldown order to be breadcrumbs, filter, subfolders, then channel content")
	}
	if !strings.Contains(body, `virtual-folder-actions`) || !strings.Contains(body, `guideFreshnessHTML() + renderVirtualCategoryViewToggle()`) {
		t.Fatalf("expected virtual folder freshness and view toggle to align with breadcrumbs")
	}
	if strings.Contains(body, `+ (children.length ? sectionHeader("Virtual Groups")`) || strings.Contains(body, `+ (children.length ? sectionHeader("Virtual Categories")`) {
		t.Fatalf("expected virtual child groups to render without a duplicate section heading")
	}
	if strings.Contains(body, "Saved on this device") {
		t.Fatalf("expected profile preferences not to expose local-only save copy")
	}
	if strings.Contains(body, `postJSON("/dispatcharr/api/favorites"`) || strings.Contains(body, `postJSON("/dispatcharr/api/hidden-categories"`) {
		t.Fatalf("expected user preference changes to persist through Silo profile preferences")
	}
	if strings.Contains(body, `data-title=\"" + escapeHTML(programTitle) + "\"`) {
		t.Fatalf("expected guide program cells not to expose hover title popups")
	}
	if strings.Contains(body, `data-tooltip-always=\"true\"`) || strings.Contains(body, `epgProgramTooltip(`) {
		t.Fatalf("expected guide program details to use modal instead of large hover tooltip")
	}
}

func TestManifestDeclaresPublicApplicationRoutesOnly(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("..", "..", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest struct {
		HTTPRoutes []struct {
			Method string `json:"method"`
			Path   string `json:"path"`
		} `json:"http_routes"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	seen := map[string]bool{}
	for _, route := range manifest.HTTPRoutes {
		seen[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /dispatcharr/api/sports",
		"GET /dispatcharr/api/events",
		"GET /dispatcharr/api/recordings/capability",
		"GET /dispatcharr/assets/app.js",
		"GET /dispatcharr/assets/lineup.js",
		"GET /dispatcharr/assets/app.css",
	} {
		if !seen[route] {
			t.Fatalf("manifest does not declare %s", route)
		}
	}
	for _, route := range []string{
		"POST /dispatcharr/api/sports/favorites",
		"GET /dispatcharr/api/preferences",
		"POST /dispatcharr/api/preferences",
		"POST /dispatcharr/api/favorites",
		"POST /dispatcharr/api/hidden-categories",
		"GET /dispatcharr/api/playback",
		"POST /dispatcharr/api/playback",
	} {
		if seen[route] {
			t.Fatalf("manifest must not advertise process-global user state route %s", route)
		}
	}
}

func TestHTTPRoutesServerAdminPageIncludesCategoryMapping(t *testing.T) {
	t.Parallel()

	response, err := NewHTTPRoutesServer(cache.NewStore()).Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/admin"})
	if err != nil {
		t.Fatalf("admin route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	body := string(response.GetBody()) + "\n" + playerAppJavaScript() + "\n" + playerStylesCSS()
	for _, want := range []string{
		`<title>Live TV Admin</title>`,
		`<h1>Live TV Admin</h1>`,
		`<div class="shell is-admin">`,
		`.shell.is-admin .rail { display: none; }`,
		`.shell.is-admin .main { display: grid; grid-template-rows: auto minmax(0, 1fr); min-height: 0; padding: 0; }`,
		`.admin-topbar`,
		`justify-content: flex-start`,
		`<div class="admin-topbar">`,
		`<nav id="admin-tabs" class="admin-tabs" aria-label="Live TV admin sections"></nav>`,
		`<div id="admin-actions" class="admin-actions"></div>`,
		`const adminSettingsKey = "adminCategorySettings"`,
		`adminTab: "settings"`,
		`function defaultAdminCategorySettings()`,
		`function renderAdminPage()`,
		`function renderAdminTopbarTabs()`,
		`function renderAdminTopbarActions()`,
		`function renderAdminSettingsTab()`,
		`function renderAdminIntegrationsTab()`,
		`Connection Status`,
		`function adminStatusPanel()`,
		`function refreshAdminStatus()`,
		`adminStatusItem("Profiles", profileValue, profileDetail)`,
		`Dispatcharr account. Assign profiles in Dispatcharr, then refresh Live TV.`,
		`admin-status-strip`,
		`admin-status-refresh`,
		`data-admin-status-refresh`,
		`Refresh connection status`,
		`Connection status refreshed.`,
		`Presentation Overrides`,
		`function renderAdminCategoryAliasSettings()`,
		`function renderAdminECMSettings()`,
		`root.innerHTML = adminSaveStatusHTML() + "<div class=\"settings-row ecm-url-row compact-row\"`,
		`function adminECMURL()`,
		`virtualGroupSource: "group"`,
		`ecmEnabled: false`,
		`collapseDuplicateVirtualGroups: true`,
		`inferChannelNameGroups: false`,
		`state.adminCategorySettings.virtualGroupSource = normalizeVirtualGroupSource(state.adminCategorySettings.virtualGroupSource, state.adminCategorySettings.inferChannelNameGroups === true)`,
		`state.adminCategorySettings.ecmEnabled = !!state.adminCategorySettings.ecmURL`,
		`state.adminCategorySettings.inferChannelNameGroups = state.adminCategorySettings.virtualGroupSource !== "group"`,
		`function virtualGroupSourceMode()`,
		`function inferredChannelNameGroupPaths(channel)`,
		`data-admin-category-field=\"virtualGroupSource\"`,
		`Group pipe + channel pipe`,
		`Profile pipe + group pipe`,
		`Channel pipe`,
		`Collapse duplicate virtual groups`,
		`return !!adminECMURL();`,
		`Group method`,
		`virtual-label-control`,
		`placeholder=\"Groups\"`,
		`Alternative group name`,
		`Also show as`,
		`alias-builder`,
		`alias-table`,
		`alias-table-row`,
		`Normal`,
		`By delimiter`,
		`ECM URL`,
		`ecm-url-row`,
		`.settings-row.ecm-url-row input`,
		`data-admin-tab=\"settings\"`,
		`data-admin-tab=\"integrations\"`,
		`data-admin-tab=\"manager\"`,
		`data-admin-ecm-field=\"url\"`,
		`byId("view").innerHTML = state.adminTab === "manager" ? renderExternalChannelManager()`,
		`data-admin-category-field=\"mode\"`,
		`data-admin-alias-action=\"add\"`,
		`data-admin-alias-action=\"remove\"`,
		`data-admin-settings-action=\"save\"`,
		`data-admin-settings-action=\"discard\"`,
		`Saved plugin settings.`,
		`renderAdminTopbarActions();`,
		`function renderExternalChannelManager()`,
		`classList.toggle("is-admin-manager"`,
		`class=\"external-manager-surface\"`,
		`class=\"external-manager-frame\"`,
		`Unsaved changes.`,
		`Save`,
		`Discard`,
		`function effectiveChannel(channel)`,
		`/dispatcharr/api/admin-settings`,
		`/dispatcharr/api/refresh-channels`,
		`Retry profiles`,
		`data-admin-profile-refresh`,
		`/api/v1/admin/plugins/installations/`,
		`key: "category_settings"`,
		`state.adminCategorySettings = await loadAdminCategorySettings().catch(function()`,
		`row.keywords.join("\n")`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected admin page to include category mapping marker %q", want)
		}
	}
	if strings.Contains(body, `row.keywords.join("\\n")`) {
		t.Fatal("expected event keyword textareas to render real line breaks, not escaped newline text")
	}
	if strings.Contains(body, "dispatcharr-admin-token") {
		t.Fatal("expected admin page to rely on Silo route authorization, not a custom browser token")
	}
	if strings.Contains(body, `class="nav admin-nav"`) || strings.Contains(body, `function renderAdminSidebarTabs()`) {
		t.Fatal("expected admin tabs to render in the topbar, not the sidebar")
	}
	if strings.Contains(body, `<div class=\"settings-card\"><div class=\"external-manager-head\"`) {
		t.Fatal("expected ECM iframe to render as a full action-area surface, not inside a settings card")
	}
	if strings.Contains(body, `external-manager-toolbar`) || strings.Contains(body, `Open in new window`) {
		t.Fatal("expected ECM iframe to render without a floating open-in-new-window overlay")
	}
	if strings.Contains(body, `https://`+`ecm.ramindex.org`) {
		t.Fatal("expected admin page not to include a hardcoded ECM URL")
	}
	for _, removed := range []string{
		`Admin-only status panel. No usernames, passwords, or API keys are shown.`,
		`<div class=\"settings-card\"><h2>Preview</h2>`,
		`function adminCategoryPreview()`,
		`Group Renames`,
		`data-admin-rename-action`,
		`data-admin-rename-field`,
		`function renderAdminCategoryRenameSettings()`,
	} {
		if strings.Contains(body, removed) {
			t.Fatalf("expected admin page to omit removed settings clutter %q", removed)
		}
	}
	for _, hidden := range []string{`<span>Home</span>`, `<span>Favorites</span>`, `<span>Guide</span>`, `aria-label="Live TV sections"`} {
		if strings.Contains(body, hidden) {
			t.Fatalf("expected admin page shell to hide user nav marker %q", hidden)
		}
	}
}

func TestDelimiterVirtualFoldersApplyToSourceGroups(t *testing.T) {
	t.Parallel()

	script := extractPlayerScript(t)
	context := map[string]any{
		"state": map[string]any{
			"app": map[string]any{
				"channels": []map[string]any{
					{"id": "channel:world-cup", "name": "World Cup Feed", "categoryId": "cat:world-cup", "categoryName": "* World Cup"},
					{"id": "channel:admin-favorites", "name": "Admin Favorite", "categoryId": "cat:admin-favorites", "categoryName": "* Admin Favorites"},
					{"id": "channel:argentina-sports", "name": "Argentina Sports", "categoryId": "cat:argentina-sports", "categoryName": "* International | Argentina | Sports"},
					{"id": "channel:world-cup-replay", "name": "World Cup Replay", "categoryId": "cat:world-cup-replays", "categoryName": "World Cup Replays"},
					{"id": "channel:ny-local", "name": "NY | New York City | FOX 5 WNYW", "categoryId": "cat:locals", "categoryName": "Locals", "profileIds": []string{"profile-ny", "profile-us-tv"}},
					{"id": "channel:profile-us-tv-dup", "name": "Demo Channel", "categoryId": "cat:us-tv", "categoryName": "US TV", "profileIds": []string{"profile-us-tv"}},
					{"id": "channel:us-tv-dup", "name": "TV | Demo Channel", "categoryId": "cat:us-tv-pipe", "categoryName": "US | TV"},
					{"id": "channel:argentina-city", "name": "Argentina | Buenos Aires | Sports 1", "categoryId": "cat:intl-sports", "categoryName": "International Sports"},
					{"id": "channel:us-sports-mlb", "name": "MLB Teams Network", "categoryId": "cat:us-sports-mlb", "categoryName": "US | Sports | MLB Teams", "profileIds": []string{"profile-ny"}},
					{"id": "channel:ny-news-sports", "name": "NY Sports News", "categoryId": "cat:news-sports", "categoryName": "News | Sports | Regional", "profileIds": []string{"profile-ny"}},
				},
				"categories": []map[string]any{
					{"id": "cat:world-cup", "name": "* World Cup"},
					{"id": "cat:admin-favorites", "name": "* Admin Favorites"},
					{"id": "cat:argentina-sports", "name": "* International | Argentina | Sports"},
					{"id": "cat:world-cup-replays", "name": "World Cup Replays"},
					{"id": "cat:locals", "name": "Locals"},
					{"id": "cat:us-tv", "name": "US TV"},
					{"id": "cat:us-tv-pipe", "name": "US | TV"},
					{"id": "cat:intl-sports", "name": "International Sports"},
					{"id": "cat:us-sports-mlb", "name": "US | Sports | MLB Teams"},
					{"id": "cat:news-sports", "name": "News | Sports | Regional"},
				},
				"source": map[string]any{
					"profiles": []map[string]any{
						{"id": "profile-ny", "name": "US TV | NY", "channelCount": 3},
						{"id": "profile-us-tv", "name": "US TV", "channelCount": 1},
					},
				},
			},
			"adminCategorySettings": map[string]any{
				"mode":                   "delimiter",
				"delimiter":              "pipe",
				"virtualGroupSource":     "group_channel",
				"inferChannelNameGroups": true,
				"categoryAliases": []map[string]any{
					{"sourcePath": "International | Argentina | Sports", "aliasPath": "Sports | Argentina"},
					{"sourcePath": "International | Argentina | Sports", "aliasPath": "World Cup | Argentina"},
					{"sourcePath": "US | Sports", "aliasPath": "Sports | US"},
					{"sourcePath": "News | Sports", "aliasPath": "Information | Athletics"},
				},
			},
		},
	}

	result := runVirtualAliasScript(t, script, context)
	if !result.SourcePath {
		t.Fatalf("expected source path to remain visible: %+v", result)
	}
	if !result.ProfileGroupPath || !result.ProfileGroupRoot {
		t.Fatalf("expected profile plus channel group virtual paths to be present: %+v", result)
	}
	if !result.ProfileNestedGroupPath {
		t.Fatalf("expected every nested channel group segment beneath the profile path: %+v", result)
	}
	if !result.ProfileOverridePath {
		t.Fatalf("expected presentation overrides to remain scoped beneath each profile path: %+v", result)
	}
	if !result.ProfileSelectionDefaultsAll || !result.ProfileSelectionFiltersChannels || !result.ProfileSelectionFiltersPaths || !result.ProfileSelectionFiltersPrograms || !result.ProfileSelectionFiltersEventChannels || !result.ProfileSelectionDropsStaleIDs {
		t.Fatalf("expected per-user profile selection to filter every Live TV discovery surface: %+v", result)
	}
	if result.ProfileOrganizationMode != "delimiter" {
		t.Fatalf("expected profile organization to require delimiter mode: %+v", result)
	}
	if !result.ProfileLocalMarketPath {
		t.Fatalf("expected profile locals to include inferred market path: %+v", result)
	}
	if !result.SelectedProfileScoped {
		t.Fatalf("expected an explicitly selected profile to hide other profile memberships: %+v", result)
	}
	if !result.DuplicateProfileCollapsed || !result.DuplicateProfileExpanded || !result.DuplicateGroupCollapsed || !result.DuplicateGroupExpanded {
		t.Fatalf("expected duplicate virtual group labels to collapse by default and expand when disabled: %+v", result)
	}
	if !result.AliasPath || !result.SecondAliasPath {
		t.Fatalf("expected Silo admin alias paths to be present: %+v", result)
	}
	if result.SourceCount != 1 || result.AliasCount != 1 || result.SecondAliasCount != 1 {
		t.Fatalf("expected source and alias counts to point at the same channel: %+v", result)
	}
	if !result.PrefixAliasPath || result.PrefixAliasCount != 1 {
		t.Fatalf("expected prefix alias subtree to include one channel: %+v", result)
	}
	if !result.InferredLocalGroup || !result.InferredLocalCityGroup || !result.InferredCountryGroup || !result.InferredCountryCityGroup {
		t.Fatalf("expected channel-name inference to add local and international city/country virtual groups: %+v", result)
	}
	if !result.ChannelOnlySourceHidden || !result.ChannelOnlyInferredShown {
		t.Fatalf("expected channel-pipe source mode to hide source group paths while preserving channel-name groups: %+v", result)
	}
	if result.ObjectParsedMode != "delimiter" {
		t.Fatalf("expected admin settings JSON object to preserve mode: %+v", result)
	}
	if result.StringParsedMode != "delimiter" {
		t.Fatalf("expected admin settings JSON string to preserve mode: %+v", result)
	}
	if !result.FeaturedSection || !result.FeaturedCategory {
		t.Fatalf("expected starred source category to render in featured section: %+v", result)
	}
	if !result.FeaturedRenamedSection {
		t.Fatalf("expected featured section label to use the configured group label: %+v", result)
	}
	if !result.ListingRenamedSection {
		t.Fatalf("expected channel listing section label to use the configured group label: %+v", result)
	}
	if !result.GuideRenamedAllOption {
		t.Fatalf("expected guide filter all option to use the configured group label: %+v", result)
	}
	if !result.VirtualRenamedBreadcrumb {
		t.Fatalf("expected virtual folder breadcrumbs to use the configured group label: %+v", result)
	}
	if !result.FeaturedAlphabetical {
		t.Fatalf("expected featured categories to render alphabetically by display name: %+v", result)
	}
	if result.FeaturedMarkerVisible {
		t.Fatalf("expected starred source category marker to be hidden: %+v", result)
	}
	if !result.FeaturedVirtualCategory {
		t.Fatalf("expected starred delimiter category to open the featured breadcrumb view: %+v", result)
	}
	if result.FeaturedSourceCategory {
		t.Fatalf("expected starred delimiter category to stop linking to the source-card view: %+v", result)
	}
	if !result.FeaturedBreadcrumbRoot || !result.FeaturedBreadcrumbPath || !result.FeaturedGuide {
		t.Fatalf("expected starred delimiter category to render featured breadcrumbs and guide: %+v", result)
	}
	if result.FeaturedGuideHeading || result.VirtualGuideHeading {
		t.Fatalf("expected virtual drilldown guide views to omit the redundant TV Guide heading: %+v", result)
	}
	if !result.FeaturedViewToggle || !result.FeaturedListView {
		t.Fatalf("expected featured virtual category to toggle between guide and channel list views: %+v", result)
	}
	if !result.SimpleFeaturedCategory || !result.SimpleFeaturedGuide || !result.SimpleFeaturedViewToggle || result.SimpleFeaturedSourcePage {
		t.Fatalf("expected simple starred groups to use the featured drilldown guide/list view: %+v", result)
	}
	if !result.VirtualBreadcrumbRoot {
		t.Fatalf("expected normal virtual group breadcrumb root to use the default configured label: %+v", result)
	}
	if result.FeaturedBackButton || result.VirtualBackButton {
		t.Fatalf("expected virtual drilldowns to omit the redundant Back button: %+v", result)
	}
	if result.ChannelCategoryName != "International | Argentina | Sports" {
		t.Fatalf("expected channel category display name to hide marker: %+v", result)
	}
	if !result.ReplayRewindable || result.NormalRewindable {
		t.Fatalf("expected only World Cup Replays channels to be rewindable: %+v", result)
	}
	if !result.ReplayPlayerClass || !result.ReplayPlayerControls || !result.ReplayPlayerTag {
		t.Fatalf("expected World Cup Replays player to expose replay controls: %+v", result)
	}
	if !result.EPGOverlapResolved {
		t.Fatalf("expected overlapping EPG programs to render without overlapping cells: %+v", result)
	}
	if !result.EPGLiveTitleMarker {
		t.Fatalf("expected EPG live title suffixes to render as dedicated accessible status markers: %+v", result)
	}
	if !result.GuideStartsAtCurrentSlot {
		t.Fatalf("expected guide window to start at the current half-hour slot: %+v", result)
	}
	if !result.ProgramSearchMatchesEPG {
		t.Fatalf("expected global search to match channels by EPG program title: %+v", result)
	}
	if !result.GuideWindowBounded {
		t.Fatalf("expected guide windowing to stay within the 60-row DOM bound: %+v", result)
	}
	if !result.DetailsFirstProgramClick {
		t.Fatalf("expected Search and On Later program clicks to open explicit Watch Now details: %+v", result)
	}
	if !result.DetailsLiveTag {
		t.Fatalf("expected live program details to render a dedicated live-status tag: %+v", result)
	}
	if !result.RecordingDeniedHidden || !result.RecordingAdminShown {
		t.Fatalf("expected recording controls to follow verified Dispatcharr permissions: %+v", result)
	}
	if !result.PlayerReturnContextRestored {
		t.Fatalf("expected player exit to restore browse and scroll context: %+v", result)
	}
}

func TestHTTPRoutesServerAppPageIncludesOrderedFavorites(t *testing.T) {
	t.Parallel()

	response, err := NewHTTPRoutesServer(cache.NewStore()).Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr"})
	if err != nil {
		t.Fatalf("app route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	body := string(response.GetBody()) + "\n" + playerAppJavaScript() + "\n" + playerStylesCSS()
	for _, want := range []string{
		`favoriteOrder: []`,
		`function orderedFavoriteChannels(`,
		`function moveFavorite(channelID, direction)`,
		`function setChannelFavorite(channelID, enabled)`,
		`const isFavorite = setChannelFavorite(id, !favoriteMap()[id]);`,
		`data-favorite-move=\"up\"`,
		`data-favorite-move=\"down\"`,
		`clip-path: inset(0);`,
		`.epg-cell .epg-play { position: absolute; inset: 0;`,
		`max-width: 100%; overflow: hidden; white-space: nowrap;`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected app page to include UI marker %q", want)
		}
	}
	helperIndex := strings.Index(body, `function setChannelFavorite(channelID, enabled)`)
	if helperIndex == -1 {
		t.Fatalf("expected app page to include channel favorite helper")
	}
	helperBody := body[helperIndex:]
	saveIndex := strings.Index(helperBody, `savePrefs();`)
	if saveIndex == -1 {
		t.Fatalf("expected channel favorite helper to save Silo profile preferences")
	}
	if strings.Contains(helperBody[:strings.Index(helperBody, `function channelMatchesQuery(channel)`)], `postJSON("/dispatcharr/api/favorites"`) {
		t.Fatalf("expected channel favorite helper not to use the local plugin favorite cache endpoint")
	}
}

type virtualAliasResult struct {
	SourcePath                           bool   `json:"sourcePath"`
	ProfileGroupPath                     bool   `json:"profileGroupPath"`
	ProfileGroupRoot                     bool   `json:"profileGroupRoot"`
	ProfileNestedGroupPath               bool   `json:"profileNestedGroupPath"`
	ProfileOverridePath                  bool   `json:"profileOverridePath"`
	ProfileSelectionDefaultsAll          bool   `json:"profileSelectionDefaultsAll"`
	ProfileSelectionFiltersChannels      bool   `json:"profileSelectionFiltersChannels"`
	ProfileSelectionFiltersPaths         bool   `json:"profileSelectionFiltersPaths"`
	ProfileSelectionFiltersPrograms      bool   `json:"profileSelectionFiltersPrograms"`
	ProfileSelectionFiltersEventChannels bool   `json:"profileSelectionFiltersEventChannels"`
	ProfileSelectionDropsStaleIDs        bool   `json:"profileSelectionDropsStaleIds"`
	ProfileOrganizationMode              string `json:"profileOrganizationMode"`
	ProfileLocalMarketPath               bool   `json:"profileLocalMarketPath"`
	SelectedProfileScoped                bool   `json:"selectedProfileScoped"`
	DuplicateProfileCollapsed            bool   `json:"duplicateProfileCollapsed"`
	DuplicateProfileExpanded             bool   `json:"duplicateProfileExpanded"`
	DuplicateGroupCollapsed              bool   `json:"duplicateGroupCollapsed"`
	DuplicateGroupExpanded               bool   `json:"duplicateGroupExpanded"`
	AliasPath                            bool   `json:"aliasPath"`
	SecondAliasPath                      bool   `json:"secondAliasPath"`
	PrefixAliasPath                      bool   `json:"prefixAliasPath"`
	SourceCount                          int    `json:"sourceCount"`
	AliasCount                           int    `json:"aliasCount"`
	SecondAliasCount                     int    `json:"secondAliasCount"`
	PrefixAliasCount                     int    `json:"prefixAliasCount"`
	InferredLocalGroup                   bool   `json:"inferredLocalGroup"`
	InferredLocalCityGroup               bool   `json:"inferredLocalCityGroup"`
	InferredCountryGroup                 bool   `json:"inferredCountryGroup"`
	InferredCountryCityGroup             bool   `json:"inferredCountryCityGroup"`
	ChannelOnlySourceHidden              bool   `json:"channelOnlySourceHidden"`
	ChannelOnlyInferredShown             bool   `json:"channelOnlyInferredShown"`
	ObjectParsedMode                     string `json:"objectParsedMode"`
	StringParsedMode                     string `json:"stringParsedMode"`
	FeaturedSection                      bool   `json:"featuredSection"`
	FeaturedRenamedSection               bool   `json:"featuredRenamedSection"`
	ListingRenamedSection                bool   `json:"listingRenamedSection"`
	GuideRenamedAllOption                bool   `json:"guideRenamedAllOption"`
	VirtualRenamedBreadcrumb             bool   `json:"virtualRenamedBreadcrumb"`
	FeaturedCategory                     bool   `json:"featuredCategory"`
	FeaturedAlphabetical                 bool   `json:"featuredAlphabetical"`
	FeaturedVirtualCategory              bool   `json:"featuredVirtualCategory"`
	FeaturedSourceCategory               bool   `json:"featuredSourceCategory"`
	FeaturedMarkerVisible                bool   `json:"featuredMarkerVisible"`
	FeaturedBreadcrumbRoot               bool   `json:"featuredBreadcrumbRoot"`
	FeaturedBreadcrumbPath               bool   `json:"featuredBreadcrumbPath"`
	FeaturedGuide                        bool   `json:"featuredGuide"`
	FeaturedGuideHeading                 bool   `json:"featuredGuideHeading"`
	FeaturedViewToggle                   bool   `json:"featuredViewToggle"`
	FeaturedListView                     bool   `json:"featuredListView"`
	FeaturedBackButton                   bool   `json:"featuredBackButton"`
	SimpleFeaturedCategory               bool   `json:"simpleFeaturedCategory"`
	SimpleFeaturedGuide                  bool   `json:"simpleFeaturedGuide"`
	SimpleFeaturedViewToggle             bool   `json:"simpleFeaturedViewToggle"`
	SimpleFeaturedSourcePage             bool   `json:"simpleFeaturedSourcePage"`
	VirtualBreadcrumbRoot                bool   `json:"virtualBreadcrumbRoot"`
	VirtualGuideHeading                  bool   `json:"virtualGuideHeading"`
	VirtualBackButton                    bool   `json:"virtualBackButton"`
	ChannelCategoryName                  string `json:"channelCategoryName"`
	ReplayRewindable                     bool   `json:"replayRewindable"`
	NormalRewindable                     bool   `json:"normalRewindable"`
	ReplayPlayerClass                    bool   `json:"replayPlayerClass"`
	ReplayPlayerControls                 bool   `json:"replayPlayerControls"`
	ReplayPlayerTag                      bool   `json:"replayPlayerTag"`
	EPGOverlapResolved                   bool   `json:"epgOverlapResolved"`
	EPGLiveTitleMarker                   bool   `json:"epgLiveTitleMarker"`
	GuideStartsAtCurrentSlot             bool   `json:"guideStartsAtCurrentSlot"`
	ProgramSearchMatchesEPG              bool   `json:"programSearchMatchesEpg"`
	GuideWindowBounded                   bool   `json:"guideWindowBounded"`
	DetailsFirstProgramClick             bool   `json:"detailsFirstProgramClick"`
	DetailsLiveTag                       bool   `json:"detailsLiveTag"`
	RecordingDeniedHidden                bool   `json:"recordingDeniedHidden"`
	RecordingAdminShown                  bool   `json:"recordingAdminShown"`
	PlayerReturnContextRestored          bool   `json:"playerReturnContextRestored"`
}

func extractPlayerScript(t *testing.T) string {
	t.Helper()

	script := playerAppJavaScript()
	if script == "" {
		t.Fatal("expected embedded app script")
	}
	return script
}

func runVirtualAliasScript(t *testing.T, script string, context map[string]any) virtualAliasResult {
	t.Helper()

	payload, err := json.Marshal(context)
	if err != nil {
		t.Fatalf("marshal context: %v", err)
	}
	dir := t.TempDir()
	appScriptPath := filepath.Join(dir, "app.js")
	runnerPath := filepath.Join(dir, "runner.js")
	if err := os.WriteFile(appScriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write app script: %v", err)
	}
	nodeScript := fmt.Sprintf(`
const fs = require("fs");
const vm = require("vm");
const input = %s;
const script = fs.readFileSync(%q, "utf8");
function makeElement() {
  const attributes = {};
  return {
    innerHTML: "",
    textContent: "",
    value: "",
    style: {},
    classList: { add: () => {}, remove: () => {}, toggle: () => {} },
    setAttribute: (name, value) => { attributes[name] = String(value); },
    getAttribute: (name) => attributes[name] || null,
    removeAttribute: (name) => { delete attributes[name]; },
    focus: () => {},
    querySelector: () => null,
    querySelectorAll: () => [],
    closest: () => null,
    addEventListener: () => {},
    play: () => Promise.resolve(),
    pause: () => {},
    load: () => {},
  };
}
const mainElement = makeElement();
const documentListeners = {};
const sandbox = {
  window: { location: { pathname: "/api/v1/plugins/14/dispatcharr/admin", search: "" }, innerHeight: 800, scrollY: 0, scrollTo: function(x, y) { this.lastScroll = [x, y]; }, addEventListener: () => {} },
  document: { documentElement: { dataset: {} }, body: makeElement(), elements: {}, fullscreenElement: null, activeElement: null, querySelectorAll: () => [], querySelector: (selector) => selector === ".main" ? mainElement : makeElement(), getElementById: function(id) { this.elements[id] = this.elements[id] || makeElement(); return this.elements[id]; }, addEventListener: function(name, handler) { this.listeners[name] = this.listeners[name] || []; this.listeners[name].push(handler); }, listeners: documentListeners, contains: () => true },
  localStorage: { getItem: () => null, setItem: () => {} },
  navigator: { sendBeacon: () => true },
  console: { log: () => {}, warn: () => {}, error: () => {} },
  URLSearchParams,
  getComputedStyle: () => ({ getPropertyValue: () => "", fontSize: "16px" }),
  requestAnimationFrame: (callback) => { callback(); return 1; },
  setTimeout,
  clearTimeout,
};
sandbox.input = input;
vm.createContext(sandbox);
vm.runInContext(script, sandbox);
const result = vm.runInContext(`+"`"+`
Object.assign(state, input.state);
const epgWindow = guideWindow();
state.app.programs = [
  { id: "overlap-a", channelId: "channel:argentina-sports", title: "First overlapping program with a very long title \u1d38\u1da6\u1d5b\u1d49", startUnix: epgWindow.start, endUnix: epgWindow.start + 3600 },
  { id: "overlap-b", channelId: "channel:argentina-sports", title: "Second overlapping program", startUnix: epgWindow.start + 1800, endUnix: epgWindow.start + 5400 }
];
rebuildProgramIndex();
JSON.stringify((function() {
  const all = virtualCategoriesFromPaths("", function() { return true; }, true);
  state.adminCategorySettings.virtualGroupSource = "profile_group";
  normalizeAdminCategorySettings();
  const profileOrganizationMode = state.adminCategorySettings.mode;
  const profileAll = virtualCategoriesFromPaths("", function() { return true; }, true);
  const nyLocalProfilePaths = virtualPathsForChannel(channelByID("channel:ny-local"));
  const nestedProfileGroupPaths = virtualPathsForChannel(channelByID("channel:ny-news-sports"));
  const profileOverride = profileAll.find(function(item) { return item.name === "US TV / NY / Information / Athletics / Regional"; });
  state.app.source.channelProfile = { id: "profile-ny", name: "US TV | NY" };
  const selectedProfilePaths = profilePathsForChannel(channelByID("channel:ny-local"));
  delete state.app.source.channelProfile;
  const duplicateProfilePaths = virtualPathsForChannel(channelByID("channel:profile-us-tv-dup"));
  state.adminCategorySettings.collapseDuplicateVirtualGroups = false;
  normalizeAdminCategorySettings();
  const duplicateProfileExpandedPaths = virtualPathsForChannel(channelByID("channel:profile-us-tv-dup"));
  state.adminCategorySettings.collapseDuplicateVirtualGroups = true;
  normalizeAdminCategorySettings();
  state.adminCategorySettings.virtualGroupSource = "group_channel";
  normalizeAdminCategorySettings();
  const usTVDuplicateGroupPaths = virtualPathsForChannel(channelByID("channel:us-tv-dup"));
  state.adminCategorySettings.collapseDuplicateVirtualGroups = false;
  normalizeAdminCategorySettings();
  const usTVDuplicateGroupExpandedPaths = virtualPathsForChannel(channelByID("channel:us-tv-dup"));
  state.adminCategorySettings.collapseDuplicateVirtualGroups = true;
  normalizeAdminCategorySettings();
  const source = all.find(function(item) { return item.name === "International / Argentina / Sports"; });
  const profileGroup = profileAll.find(function(item) { return item.name === "US TV / NY / Locals"; });
  const profileRoot = profileAll.find(function(item) { return item.name === "US TV"; });
  const alias = all.find(function(item) { return item.name === "Sports / Argentina"; });
  const secondAlias = all.find(function(item) { return item.name === "World Cup / Argentina"; });
  const prefixAlias = all.find(function(item) { return item.name === "Sports / US / MLB Teams"; });
  const channelsInSource = effectiveChannels(false).filter(function(channel) {
    return virtualPathsForChannel(channel).indexOf("International / Argentina / Sports") !== -1;
  });
  const channelsInAlias = effectiveChannels(false).filter(function(channel) {
    return virtualPathsForChannel(channel).indexOf("Sports / Argentina") !== -1;
  });
  const channelsInSecondAlias = effectiveChannels(false).filter(function(channel) {
    return virtualPathsForChannel(channel).indexOf("World Cup / Argentina") !== -1;
  });
  const channelsInPrefixAlias = effectiveChannels(false).filter(function(channel) {
    return virtualPathsForChannel(channel).indexOf("Sports / US / MLB Teams") !== -1;
  });
  const nyLocal = channelByID("channel:ny-local");
  const argentinaCity = channelByID("channel:argentina-city");
  const nyPaths = virtualPathsForChannel(nyLocal);
  const argentinaPaths = virtualPathsForChannel(argentinaCity);
  state.adminCategorySettings.virtualGroupSource = "channel";
  normalizeAdminCategorySettings();
  const channelOnlyArgentinaPaths = virtualPathsForChannel(argentinaCity);
  const channelOnlySourceHidden = channelOnlyArgentinaPaths.indexOf("International Sports") === -1;
  const channelOnlyInferredShown = channelOnlyArgentinaPaths.indexOf("International Sports / Argentina") !== -1;
  state.adminCategorySettings.virtualGroupSource = "group_channel";
  normalizeAdminCategorySettings();
  const grid = categoryGrid();
  state.adminCategorySettings.virtualGroupLabel = "Things";
  const renamedGrid = categoryGrid();
  renderGuidePage();
  const renamedGuideView = document.elements.view ? document.elements.view.innerHTML : "";
  state.category = "virtual:International / Argentina / Sports";
  renderLivePage();
  const renamedVirtualView = document.elements.view ? document.elements.view.innerHTML : "";
  state.category = "";
  state.adminCategorySettings.virtualGroupLabel = "Groups";
  const channel = channelByID("channel:argentina-sports");
  state.category = "featured:International / Argentina / Sports";
  renderLivePage();
  const featuredView = document.elements.view ? document.elements.view.innerHTML : "";
  state.category = "featured:Admin Favorites";
  renderLivePage();
  const simpleFeaturedView = document.elements.view ? document.elements.view.innerHTML : "";
  state.category = "featured:International / Argentina / Sports";
  state.virtualCategoryView = "list";
  renderLivePage();
  const featuredListView = document.elements.view ? document.elements.view.innerHTML : "";
  state.virtualCategoryView = "guide";
  state.category = "virtual:International / Argentina / Sports";
  renderLivePage();
  const virtualView = document.elements.view ? document.elements.view.innerHTML : "";
  const replayChannel = channelByID("channel:world-cup-replay");
  state.currentChannel = replayChannel;
  renderPlayerPage();
	const replayPlayerView = document.elements.view ? document.elements.view.innerHTML : "";
	const epgHTML = renderEPGCells(channel, 0);
	const epgProgramCells = epgHTML.split('style="left: calc(').slice(1).map(function(part) {
		const pieces = part.split(' * var(--epg-slot)); width: calc(');
		const widthPart = pieces[1] ? pieces[1].split(' * var(--epg-slot) - 0.0625rem);')[0] : "";
		return { left: Number(pieces[0]), width: Number(widthPart) };
	});
	const epgOverlapResolved = epgProgramCells.length >= 2 && epgProgramCells[1].left + 0.001 >= epgProgramCells[0].left + epgProgramCells[0].width;
	const epgLiveTitleMarker = epgHTML.indexOf('class="epg-live-marker" aria-hidden="true"') !== -1 && epgHTML.indexOf('First overlapping program with a very long title Live"') !== -1;
const guideStartsAtCurrentSlot = guideWindow().start === Math.floor(Math.floor(Date.now() / 1000) / 1800) * 1800;
	state.view = "home";
	state.category = "";
	state.query = "Second overlapping";
	const programSearchMatchesEPG = visibleChannels(false).some(function(item) { return item.id === "channel:argentina-sports"; });
	state.currentChannel = null;
	state.view = "search";
	state.searchQuery = "overlap";
	const searchProgramTarget = {
		closest: function(selector) { return selector === "[data-search-program-channel]" ? this : null; },
		getAttribute: function(name) { return name === "data-search-program-channel" ? "channel:argentina-sports" : (name === "data-search-program" ? "overlap-a" : ""); }
	};
	(document.listeners.click || []).forEach(function(handler) {
		handler({ target: searchProgramTarget, preventDefault: function() {} });
	});
	const programModal = document.getElementById("program-details-root");
	const detailsFirstProgramClick = !!state.programDetails && state.programDetails.programID === "overlap-a" && programModal.innerHTML.indexOf("Watch Now") !== -1 && state.currentChannel === null && state.view === "search";
	const detailsLiveTag = programModal.innerHTML.indexOf('<span class="is-live">Live now</span>') !== -1;
	const originalSource = state.app.source;
	const originalCapabilities = state.app.capabilities;
	const originalRecordingCapability = state.recordingCapability;
	state.app.source = { mode: "direct_login" };
	state.app.capabilities = { recordings: true };
	state.recordingCapability = { available: true, canSchedule: false, reason: "Scheduling requires a Dispatcharr admin account or Admin API Key." };
	renderProgramDetailsModal();
	const recordingDeniedControlsHidden = programModal.innerHTML.indexOf("data-program-detail-schedule") === -1 && !recordingSchedulingEnabled();
	state.recordingCapability = { available: true, canSchedule: true };
	renderProgramDetailsModal();
	const recordingAdminControlsShown = programModal.innerHTML.indexOf("data-program-detail-schedule") !== -1 && recordingSchedulingEnabled();
	state.app.source = originalSource;
	state.app.capabilities = originalCapabilities;
	state.recordingCapability = originalRecordingCapability;
	state.programDetails = null;
	renderProgramDetailsModal();

	state.view = "player";
	state.playerReturnContext = { view: "guide", category: "source:cat:argentina-sports", query: "sports", folderQuery: "Argentina", scrollY: 47, mainScrollTop: 63, guideScrollLeft: 91, guideScrollTop: 117 };
	returnFromPlayer();
	const restoredGuideScroll = document.getElementById("guide-scroll");
	const playerReturnContextRestored = state.view === "guide" && state.category === "source:cat:argentina-sports" && state.query === "sports" && state.folderQuery === "Argentina" && state.playerReturnContext === null && window.lastScroll[1] === 47 && document.querySelector(".main").scrollTop === 63 && restoredGuideScroll.scrollLeft === 91 && restoredGuideScroll.scrollTop === 117;

	state.guideChannels = Array.from({ length: 2521 }, function(_, index) { return { id: "window-channel-" + index, name: "Channel " + index, categoryId: "" }; });
	state.view = "guide";
	state.guideLoading = false;
	state.guideWindowStart = -1;
	state.guideWindowEnd = -1;
	restoredGuideScroll.scrollTop = 90000;
	restoredGuideScroll.clientHeight = 700;
	restoredGuideScroll.querySelector = function(selector) { return selector === ".time-head" ? { offsetHeight: 32 } : null; };
	renderGuideWindow(true);
	const renderedGuideRows = (document.getElementById("epg").innerHTML.match(/class="epg-row"/g) || []).length;
	const guideWindowBounded = state.guideWindowStart > 0 && renderedGuideRows > 0 && renderedGuideRows <= 60 && renderedGuideRows === state.guideWindowEnd - state.guideWindowStart;
	const originalPreferences = state.app.preferences;
	const originalPrograms = state.app.programs;
	state.app.preferences = defaultPrefs();
	normalizePreferences();
	const defaultProfileChannelIDs = effectiveChannels(false).map(function(channel) { return channel.id; });
	const profileSelectionDefaultsAll = defaultProfileChannelIDs.indexOf("channel:ny-local") !== -1 && defaultProfileChannelIDs.indexOf("channel:profile-us-tv-dup") !== -1;
	state.app.preferences.profileSelection = { mode: "selected", profileIds: ["profile-ny", "profile-stale"] };
	normalizePreferences();
	state.app.programs = [
	  { id: "program-profile-ny", channelId: "channel:ny-local", title: "NY News", startUnix: epgWindow.start, endUnix: epgWindow.end },
	  { id: "program-profile-us", channelId: "channel:profile-us-tv-dup", title: "US News", startUnix: epgWindow.start, endUnix: epgWindow.end }
	];
	rebuildProgramIndex();
	const selectedProfileChannelIDs = effectiveChannels(false).map(function(channel) { return channel.id; });
	const userSelectedProfilePaths = profilePathsForChannel(channelByID("channel:ny-local"));
	const selectedProgramIDs = programsFor("").map(function(program) { return program.id; });
	const selectedEventChannels = uniqueEventChannels([channelByID("channel:ny-local"), channelByID("channel:profile-us-tv-dup")]);
	const profileSelectionFiltersChannels = selectedProfileChannelIDs.indexOf("channel:ny-local") !== -1 && selectedProfileChannelIDs.indexOf("channel:profile-us-tv-dup") === -1;
	const profileSelectionFiltersPaths = userSelectedProfilePaths.length === 1 && userSelectedProfilePaths[0] === "US TV / NY";
	const profileSelectionFiltersPrograms = selectedProgramIDs.length === 1 && selectedProgramIDs[0] === "program-profile-ny";
	const profileSelectionFiltersEventChannels = selectedEventChannels.length === 1 && selectedEventChannels[0].id === "channel:ny-local";
	const profileSelectionDropsStaleIDs = profileSelection().profileIds.length === 1 && profileSelection().profileIds[0] === "profile-ny";
	state.app.preferences = originalPreferences;
	state.app.programs = originalPrograms;
	normalizePreferences();
	rebuildProgramIndex();
	return {
    sourcePath: !!source,
    profileGroupPath: !!profileGroup,
    profileGroupRoot: !!profileRoot,
    profileNestedGroupPath: nestedProfileGroupPaths.indexOf("US TV / NY / News / Sports / Regional") !== -1,
    profileOverridePath: !!profileOverride && nestedProfileGroupPaths.indexOf("US TV / NY / Information / Athletics / Regional") !== -1,
    profileSelectionDefaultsAll: profileSelectionDefaultsAll,
    profileSelectionFiltersChannels: profileSelectionFiltersChannels,
    profileSelectionFiltersPaths: profileSelectionFiltersPaths,
    profileSelectionFiltersPrograms: profileSelectionFiltersPrograms,
    profileSelectionFiltersEventChannels: profileSelectionFiltersEventChannels,
    profileSelectionDropsStaleIds: profileSelectionDropsStaleIDs,
    profileOrganizationMode: profileOrganizationMode,
    profileLocalMarketPath: nyLocalProfilePaths.indexOf("US TV / NY / Locals / New York City") !== -1,
    selectedProfileScoped: selectedProfilePaths.length === 1 && selectedProfilePaths[0] === "US TV / NY",
    duplicateProfileCollapsed: duplicateProfilePaths.indexOf("US TV") !== -1 && duplicateProfilePaths.indexOf("US TV / US TV") === -1,
    duplicateProfileExpanded: duplicateProfileExpandedPaths.indexOf("US TV / US TV") !== -1,
    duplicateGroupCollapsed: usTVDuplicateGroupPaths.indexOf("US / TV") !== -1 && usTVDuplicateGroupPaths.indexOf("US / TV / TV") === -1,
    duplicateGroupExpanded: usTVDuplicateGroupExpandedPaths.indexOf("US / TV / TV") !== -1,
    aliasPath: !!alias,
    secondAliasPath: !!secondAlias,
    prefixAliasPath: !!prefixAlias,
    sourceCount: channelsInSource.length,
    aliasCount: channelsInAlias.length,
    secondAliasCount: channelsInSecondAlias.length,
    prefixAliasCount: channelsInPrefixAlias.length,
    inferredLocalGroup: nyPaths.indexOf("US TV / Locals / NY") !== -1,
    inferredLocalCityGroup: nyPaths.indexOf("US TV / Locals / NY / New York City") !== -1,
    inferredCountryGroup: argentinaPaths.indexOf("International Sports / Argentina") !== -1,
    inferredCountryCityGroup: argentinaPaths.indexOf("International Sports / Argentina / Buenos Aires") !== -1,
    channelOnlySourceHidden: channelOnlySourceHidden,
    channelOnlyInferredShown: channelOnlyInferredShown,
    objectParsedMode: readAdminSettingsValue({ mode: "delimiter", delimiter: "pipe" }).mode,
    stringParsedMode: readAdminSettingsValue(JSON.stringify({ mode: "delimiter", delimiter: "pipe" })).mode,
    featuredSection: grid.indexOf(">Featured Groups<") !== -1,
    featuredRenamedSection: renamedGrid.indexOf(">Featured Things<") !== -1 && renamedGrid.indexOf(">Featured Groups<") === -1,
    listingRenamedSection: renamedGrid.indexOf(">Things<") !== -1 && renamedGrid.indexOf(">Channel Groups<") === -1,
    guideRenamedAllOption: renamedGuideView.indexOf('value="All things"') !== -1 && renamedGuideView.indexOf('value="All channel groups"') === -1,
    virtualRenamedBreadcrumb: renamedVirtualView.indexOf(">Things</button>") !== -1 && renamedVirtualView.indexOf(">Channel Groups</button>") === -1,
    featuredCategory: grid.indexOf("International | Argentina | Sports") !== -1,
    featuredAlphabetical: grid.indexOf(">Admin Favorites</strong>") !== -1 && grid.indexOf(">World Cup</strong>") !== -1 && grid.indexOf(">Admin Favorites</strong>") < grid.indexOf(">World Cup</strong>"),
    featuredVirtualCategory: grid.indexOf('data-category="featured:International / Argentina / Sports"') !== -1,
    featuredSourceCategory: grid.indexOf('data-category="source:cat:argentina-sports"') !== -1,
    featuredMarkerVisible: grid.indexOf("* International") !== -1,
    featuredBreadcrumbRoot: featuredView.indexOf(">Featured Groups</button>") !== -1,
    featuredBreadcrumbPath: featuredView.indexOf(">International</button>") !== -1 && featuredView.indexOf(">Argentina</button>") !== -1 && featuredView.indexOf(">Sports</button>") !== -1,
    featuredGuide: featuredView.indexOf('data-channel="channel:argentina-sports"') !== -1,
    featuredGuideHeading: featuredView.indexOf(">TV Guide<") !== -1,
    featuredViewToggle: featuredView.indexOf('data-virtual-category-view="guide"') !== -1 && featuredView.indexOf('data-virtual-category-view="list"') !== -1,
    featuredListView: featuredListView.indexOf(">Channels<") !== -1 && featuredListView.indexOf('class="virtual-channel-button" data-channel="channel:argentina-sports"') !== -1 && featuredListView.indexOf(">TV Guide<") === -1,
    featuredBackButton: featuredView.indexOf(">Back</button>") !== -1,
    simpleFeaturedCategory: grid.indexOf('data-category="featured:Admin Favorites"') !== -1,
    simpleFeaturedGuide: simpleFeaturedView.indexOf(">Featured Groups</button>") !== -1 && simpleFeaturedView.indexOf(">Admin Favorites</button>") !== -1 && simpleFeaturedView.indexOf('data-channel="channel:admin-favorites"') !== -1,
    simpleFeaturedViewToggle: simpleFeaturedView.indexOf('data-virtual-category-view="guide"') !== -1 && simpleFeaturedView.indexOf('data-virtual-category-view="list"') !== -1,
    simpleFeaturedSourcePage: simpleFeaturedView.indexOf(">Featured Groups<") !== -1 && simpleFeaturedView.indexOf(">Groups<") !== -1 && simpleFeaturedView.indexOf(">Admin Favorites<") !== -1,
    virtualBreadcrumbRoot: virtualView.indexOf(">Groups</button>") !== -1,
    virtualGuideHeading: virtualView.indexOf(">TV Guide<") !== -1,
    virtualBackButton: virtualView.indexOf(">Back</button>") !== -1,
    channelCategoryName: channel ? channel.categoryName : "",
    replayRewindable: isRewindableChannel(replayChannel),
    normalRewindable: isRewindableChannel(channel),
		replayPlayerClass: replayPlayerView.indexOf('class="playback-shell is-replay"') !== -1,
		replayPlayerControls: replayPlayerView.indexOf('controls></video>') !== -1,
		replayPlayerTag: replayPlayerView.indexOf(">Replay</span>") !== -1,
		epgOverlapResolved: epgOverlapResolved,
		epgLiveTitleMarker: epgLiveTitleMarker,
		guideStartsAtCurrentSlot: guideStartsAtCurrentSlot,
		programSearchMatchesEpg: programSearchMatchesEPG,
		guideWindowBounded: guideWindowBounded,
		detailsFirstProgramClick: detailsFirstProgramClick,
		detailsLiveTag: detailsLiveTag,
		recordingDeniedHidden: recordingDeniedControlsHidden,
		recordingAdminShown: recordingAdminControlsShown,
		playerReturnContextRestored: playerReturnContextRestored
	};
})())
`+"`"+`, sandbox);
process.stdout.write(result);
`, string(payload), appScriptPath)
	if err := os.WriteFile(runnerPath, []byte(nodeScript), 0o600); err != nil {
		t.Fatalf("write runner script: %v", err)
	}
	cmd := exec.Command("node", runnerPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run node script: %v\n%s", err, output)
	}
	var result virtualAliasResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode node result: %v\n%s", err, output)
	}
	return result
}

func TestHTTPRoutesServerRecordingsDisabledForXtream(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source:   model.LiveTVSource(model.SourceModeXtream),
			Channels: []model.Channel{{ID: "xtream:1", Name: "News HD"}},
		},
	})
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings {
		return config.Settings{
			SourceMode:      config.SourceModeXtream,
			XtreamBaseURL:   "https://dispatcharr.example.com",
			XtreamUsername:  "demo",
			XtreamPassword:  "secret",
			ChannelRefreshH: config.DefaultChannelRefreshHours,
			EPGRefreshH:     config.DefaultEPGRefreshHours,
		}
	})

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/api/recordings"})
	if err != nil {
		t.Fatalf("recordings route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	var payload RecordingsPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal recordings payload: %v", err)
	}
	if payload.Available || !strings.Contains(payload.Reason, "Dispatcharr Direct") {
		t.Fatalf("expected recordings disabled for xtream, got %+v", payload)
	}

	response, err = server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: "POST",
		Path:   "/dispatcharr/api/recordings",
		Body:   []byte(`{"channelId":"xtream:1","title":"News","startUnix":1700000000,"endUnix":1700003600}`),
	})
	if err != nil {
		t.Fatalf("recordings schedule route: %v", err)
	}
	if response.GetStatusCode() != 409 {
		t.Fatalf("expected 409, got %d", response.GetStatusCode())
	}
}

func TestHTTPRoutesServerRecordingCapabilityRequiresDispatcharrAdmin(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name        string
		userLevel   int
		canSchedule bool
	}{
		{name: "standard user", userLevel: 1, canSchedule: false},
		{name: "admin user", userLevel: 10, canSchedule: true},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/accounts/token/":
					_, _ = w.Write([]byte(`{"access":"access-token","refresh":"refresh-token"}`))
				case "/api/accounts/users/me/":
					if r.Header.Get("Authorization") != "Bearer access-token" {
						http.Error(w, "missing auth", http.StatusUnauthorized)
						return
					}
					_, _ = fmt.Fprintf(w, `{"id":7,"username":"viewer","user_level":%d}`, tt.userLevel)
				default:
					http.NotFound(w, r)
				}
			}))
			defer upstream.Close()

			store := cache.NewStore()
			store.Replace(cache.Snapshot{Catalog: model.CatalogState{Source: model.LiveTVSource(model.SourceModeDirectLogin)}})
			server := NewHTTPRoutesServerWithSettings(store, func() config.Settings {
				return config.Settings{
					SourceMode:      config.SourceModeDirectLogin,
					DispatcharrURL:  upstream.URL,
					DispatcharrUser: "viewer",
					DispatcharrPass: "secret",
				}
			})

			response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/dispatcharr/api/recordings/capability"})
			if err != nil {
				t.Fatalf("recording capability route: %v", err)
			}
			if response.GetStatusCode() != http.StatusOK {
				t.Fatalf("expected 200, got %d", response.GetStatusCode())
			}
			var payload RecordingCapabilityPayload
			if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
				t.Fatalf("decode recording capability: %v", err)
			}
			if !payload.Available || payload.CanSchedule != tt.canSchedule {
				t.Fatalf("unexpected recording capability: %+v", payload)
			}
			if !tt.canSchedule && !strings.Contains(payload.Reason, "admin account or Admin API Key") {
				t.Fatalf("expected actionable permission reason, got %+v", payload)
			}
		})
	}
}

func TestHTTPRoutesServerRecordingCapabilityAllowsAdminAPIKeyMode(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/accounts/users/me/" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-API-Key") != "secret" || r.Header.Get("Authorization") != "ApiKey secret" {
			http.Error(w, "missing API key", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"admin","user_level":10}`))
	}))
	defer upstream.Close()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{Source: model.LiveTVSource(model.SourceModeDirectLogin)}})
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings {
		return config.Settings{
			SourceMode:        config.SourceModeAPIKey,
			DispatcharrURL:    upstream.URL,
			DispatcharrAPIKey: "secret",
			ChannelRefreshH:   config.DefaultChannelRefreshHours,
			EPGRefreshH:       config.DefaultEPGRefreshHours,
		}
	})

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/dispatcharr/api/recordings/capability"})
	if err != nil {
		t.Fatalf("recording capability route: %v", err)
	}
	var payload RecordingCapabilityPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("decode recording capability: %v", err)
	}
	if !payload.Available || !payload.CanSchedule {
		t.Fatalf("expected Admin API Key mode to allow scheduling, got %+v", payload)
	}
}

func TestDvrEnabledForSourceAllowsDispatcharrDirectModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sourceMode model.SourceMode
		want       bool
	}{
		{name: "direct login", sourceMode: model.SourceModeDirectLogin, want: true},
		{name: "api key", sourceMode: model.SourceModeAPIKey, want: true},
		{name: "xtream", sourceMode: model.SourceModeXtream, want: false},
		{name: "m3u xmltv", sourceMode: model.SourceModeM3UXMLTV, want: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := dvrEnabledForSource(tt.sourceMode); got != tt.want {
				t.Fatalf("dvrEnabledForSource(%q) = %t, want %t", tt.sourceMode, got, tt.want)
			}
		})
	}
}

func TestHTTPRoutesServerScheduleRecordingReportsDispatcharrPermission(t *testing.T) {
	t.Parallel()

	const channelUUID = "dispatcharr-channel-1"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/accounts/token/":
			_, _ = w.Write([]byte(`{"access":"token","refresh":"refresh"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/channels/channels/":
			_, _ = w.Write([]byte(`[{"id":4131,"uuid":"` + channelUUID + `","name":"News HD","effective_name":"News HD","effective_tvg_id":"news.hd"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/channels/recordings/":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"detail":"You do not have permission to perform this action."}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"not found"}`))
		}
	}))
	defer upstream.Close()

	channel := model.Channel{
		ID:        model.StableChannelID(model.SourceModeDirectLogin, model.ChannelIdentity{UpstreamID: channelUUID, GuideID: "news.hd", Name: "News HD", StreamURL: upstream.URL + "/proxy/ts/stream/" + channelUUID}),
		Name:      "News HD",
		GuideID:   "news.hd",
		StreamURL: upstream.URL + "/proxy/ts/stream/" + channelUUID,
	}
	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source:   model.LiveTVSource(model.SourceModeDirectLogin),
			Channels: []model.Channel{channel},
		},
	})
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings {
		return config.Settings{
			SourceMode:      config.SourceModeDirectLogin,
			DispatcharrURL:  upstream.URL,
			DispatcharrUser: "demo",
			DispatcharrPass: "secret",
			ChannelRefreshH: config.DefaultChannelRefreshHours,
			EPGRefreshH:     config.DefaultEPGRefreshHours,
		}
	})

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: http.MethodPost,
		Path:   "/dispatcharr/api/recordings",
		Body:   []byte(fmt.Sprintf(`{"channelId":%q,"title":"News","startUnix":1900000000,"endUnix":1900003600}`, channel.ID)),
	})
	if err != nil {
		t.Fatalf("schedule route: %v", err)
	}
	if response.GetStatusCode() != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", response.GetStatusCode(), response.GetBody())
	}
	if !strings.Contains(string(response.GetBody()), "admin account or API key") {
		t.Fatalf("expected actionable permission message, got %q", response.GetBody())
	}
}

func TestScheduleRecordingErrorResponseMapsDispatcharrAuthFailures(t *testing.T) {
	t.Parallel()

	for _, message := range []string{
		"unexpected status 401: {\"detail\":\"Authentication credentials were not provided.\"}",
		"unexpected status 403: {\"detail\":\"You do not have permission to perform this action.\"}",
		"unauthorized",
		"permission denied",
	} {
		response := scheduleRecordingErrorResponse(errors.New(message))
		if response.GetStatusCode() != http.StatusForbidden {
			t.Fatalf("expected auth failure %q to map to 403, got %d", message, response.GetStatusCode())
		}
		if !strings.Contains(string(response.GetBody()), "admin account or API key") {
			t.Fatalf("expected actionable auth message for %q, got %q", message, response.GetBody())
		}
	}
}

func TestHTTPRoutesServerAppRouteHydratesColdCatalog(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	syncer := &stubCatalogSyncer{store: store}
	server := NewHTTPRoutesServerWithSyncer(store, func() config.Settings {
		return config.Settings{
			SourceMode:      config.SourceModeDirectLogin,
			DispatcharrURL:  "https://dispatcharr.example.com",
			DispatcharrUser: "demo",
			DispatcharrPass: "secret",
			ChannelRefreshH: 24,
			EPGRefreshH:     24,
		}
	}, syncer)

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/api/app"})
	if err != nil {
		t.Fatalf("app route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	if syncer.calls != 1 {
		t.Fatalf("expected cold catalog sync once, got %d", syncer.calls)
	}

	var payload AppPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal app payload: %v", err)
	}
	if len(payload.Channels) != 1 || payload.Channels[0].ID != "dispatcharr:news" {
		t.Fatalf("expected hydrated channel payload, got %+v", payload.Channels)
	}

	_, err = server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/api/app"})
	if err != nil {
		t.Fatalf("second app route: %v", err)
	}
	if syncer.calls != 1 {
		t.Fatalf("expected warm catalog to skip sync, got %d calls", syncer.calls)
	}
}

func TestHTTPRoutesServerAppRouteWarmsUnavailableAPIKeyProfilesInBackground(t *testing.T) {
	t.Parallel()

	settings := config.Settings{
		SourceMode:        config.SourceModeAPIKey,
		DispatcharrURL:    "https://dispatcharr.example.com",
		DispatcharrAPIKey: "secret",
		ChannelRefreshH:   24,
		EPGRefreshH:       24,
	}
	store := cache.NewStore()
	source := model.LiveTVSource(model.SourceModeDirectLogin)
	source.ProfileAccess = &model.ProfileAccess{Status: "unavailable", Message: "context canceled"}
	store.Replace(cache.Snapshot{
		ConfigKey: config.CatalogCacheKey(settings),
		Catalog: model.CatalogState{
			Source:   source,
			Channels: []model.Channel{{ID: "dispatcharr:old", Name: "Old Channel"}},
		},
	})
	done := make(chan struct{}, 1)
	syncer := &stubCatalogSyncer{store: store, done: done}
	server := NewHTTPRoutesServerWithSyncer(store, func() config.Settings { return settings }, syncer)

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/dispatcharr/api/app"})
	if err != nil {
		t.Fatalf("app route: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected app route to return immediately, got %d", response.GetStatusCode())
	}
	waitForStubSync(t, done)
	if syncer.callCount() != 1 {
		t.Fatalf("expected one background profile refresh, got %d", syncer.callCount())
	}
}

func TestHTTPRoutesServerAppRouteRefreshesStalePersistedSnapshotForCurrentSettings(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		ConfigKey: config.CatalogCacheKey(config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "https://old.example.com", XtreamUsername: "demo"}),
		Catalog: model.CatalogState{
			Source:   model.LiveTVSource(model.SourceModeXtream),
			Channels: []model.Channel{{ID: "xtream:old", Name: "Old Channel"}},
		},
	})
	syncer := &stubCatalogSyncer{store: store}
	server := NewHTTPRoutesServerWithSyncer(store, func() config.Settings {
		return config.Settings{
			SourceMode:      config.SourceModeDirectLogin,
			DispatcharrURL:  "https://dispatcharr.example.com",
			DispatcharrUser: "demo",
			DispatcharrPass: "secret",
			ChannelRefreshH: 24,
			EPGRefreshH:     24,
		}
	}, syncer)

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/api/app"})
	if err != nil {
		t.Fatalf("app route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	if syncer.calls != 1 {
		t.Fatalf("expected stale persisted snapshot to refresh, got %d calls", syncer.calls)
	}
	var payload AppPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal app payload: %v", err)
	}
	if len(payload.Channels) != 1 || payload.Channels[0].ID != "dispatcharr:news" {
		t.Fatalf("expected current settings payload, got %+v", payload.Channels)
	}
}

func TestHTTPRoutesServerAppRouteClearsStalePersistedSnapshotWhenCurrentSettingsInvalid(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		ConfigKey: config.CatalogCacheKey(config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "https://old.example.com", XtreamUsername: "demo"}),
		Catalog: model.CatalogState{
			Source:   model.LiveTVSource(model.SourceModeXtream),
			Channels: []model.Channel{{ID: "xtream:old", Name: "Old Channel"}},
		},
	})
	syncer := &stubCatalogSyncer{store: store}
	server := NewHTTPRoutesServerWithSyncer(store, func() config.Settings {
		return config.Settings{
			SourceMode:      config.SourceModeDirectLogin,
			DispatcharrURL:  "https://dispatcharr.example.com",
			DispatcharrUser: "demo",
			ChannelRefreshH: 24,
			EPGRefreshH:     24,
		}
	}, syncer)

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/api/app"})
	if err != nil {
		t.Fatalf("app route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	if syncer.calls != 0 {
		t.Fatalf("expected invalid settings to skip sync, got %d calls", syncer.calls)
	}
	var payload AppPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal app payload: %v", err)
	}
	if len(payload.Channels) != 0 {
		t.Fatalf("expected stale channels to be cleared, got %+v", payload.Channels)
	}
}

func TestHTTPRoutesServerRefreshRouteStartsBackgroundCatalogSync(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source:   model.LiveTVSource(model.SourceModeDirectLogin),
			Channels: []model.Channel{{ID: "dispatcharr:old", Name: "Old Channel"}},
		},
	})
	store.ReplacePrograms([]model.Program{
		{ID: "program:old-1", ChannelID: "dispatcharr:old", Title: "Old Morning"},
		{ID: "program:old-2", ChannelID: "dispatcharr:old", Title: "Old Evening"},
	}, 100)
	block := make(chan struct{})
	done := make(chan struct{}, 1)
	syncer := &stubCatalogSyncer{store: store, block: block, done: done}
	server := NewHTTPRoutesServerWithSyncer(store, func() config.Settings {
		return config.Settings{
			SourceMode:      config.SourceModeDirectLogin,
			DispatcharrURL:  "https://dispatcharr.example.com",
			DispatcharrUser: "demo",
			DispatcharrPass: "secret",
			ChannelRefreshH: 24,
			EPGRefreshH:     24,
		}
	}, syncer)

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "POST", Path: "/dispatcharr/api/refresh"})
	if err != nil {
		t.Fatalf("refresh route: %v", err)
	}
	if response.GetStatusCode() != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", response.GetStatusCode())
	}
	var payload AppPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal app payload: %v", err)
	}
	if len(payload.Channels) != 1 || payload.Channels[0].ID != "dispatcharr:old" {
		t.Fatalf("expected current channel payload while sync runs, got %+v", payload.Channels)
	}
	if payload.Status.EPGStatus != "loading" {
		t.Fatalf("expected loading EPG status while sync runs, got %+v", payload.Status)
	}
	close(block)
	waitForStubSync(t, done)
	if syncer.forceCallCount() != 1 {
		t.Fatalf("expected refresh route to force guide purge sync, got %d force calls", syncer.forceCallCount())
	}
	if syncer.callCount() != 1 {
		t.Fatalf("expected refresh to force one sync, got %d calls", syncer.callCount())
	}
	current := store.Current()
	if len(current.Catalog.Channels) != 1 || current.Catalog.Channels[0].ID != "dispatcharr:news" {
		t.Fatalf("expected refreshed channel payload, got %+v", current.Catalog.Channels)
	}
	if current.Health.EPGStatus != "ok" || current.Health.EPGProgramCount != 1 {
		t.Fatalf("expected refreshed guide health, got %+v", current.Health)
	}
	if len(current.Catalog.Programs) != 1 || current.Catalog.Programs[0].ID != "program:1" {
		t.Fatalf("expected refreshed guide programs, got %+v", current.Catalog.Programs)
	}
}

func TestHTTPRoutesServerChannelRefreshRouteStartsChannelOnlySync(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	settings := config.Settings{
		SourceMode:        config.SourceModeAPIKey,
		DispatcharrURL:    "https://dispatcharr.example.com",
		DispatcharrAPIKey: "secret",
		ChannelRefreshH:   24,
		EPGRefreshH:       24,
	}
	store.Replace(cache.Snapshot{
		ConfigKey: config.CatalogCacheKey(settings),
		Catalog: model.CatalogState{
			Source:   model.LiveTVSource(model.SourceModeDirectLogin),
			Channels: []model.Channel{{ID: "dispatcharr:old", Name: "Old Channel"}},
		},
	})
	done := make(chan struct{}, 1)
	syncer := &stubCatalogSyncer{store: store, done: done}
	server := NewHTTPRoutesServerWithSyncer(store, func() config.Settings { return settings }, syncer)

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodPost, Path: "/dispatcharr/api/refresh-channels"})
	if err != nil {
		t.Fatalf("channel refresh route: %v", err)
	}
	if response.GetStatusCode() != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", response.GetStatusCode())
	}
	waitForStubSync(t, done)
	if syncer.channelCallCount() != 1 || syncer.forceCallCount() != 0 {
		t.Fatalf("expected channel-only refresh, got channels=%d force=%d", syncer.channelCallCount(), syncer.forceCallCount())
	}
}

func TestHTTPRoutesServerGuidePingRefreshesWhenAnyCheckedChannelIsMissingGuide(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	now := time.Now().Unix()
	settings := config.Settings{
		SourceMode:      config.SourceModeDirectLogin,
		DispatcharrURL:  "https://dispatcharr.example.com",
		DispatcharrUser: "demo",
		DispatcharrPass: "secret",
		ChannelRefreshH: 24,
		EPGRefreshH:     24,
	}
	source := model.LiveTVSource(model.SourceModeDirectLogin)
	source.ProfileAccess = &model.ProfileAccess{Status: "available", ProfileCount: 1}
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{
		Source: source,
		Channels: []model.Channel{
			{ID: "dispatcharr:news", Name: "News HD"},
			{ID: "dispatcharr:sports", Name: "Sports HD"},
		},
	}, ConfigKey: config.CatalogCacheKey(settings)})
	store.ReplacePrograms([]model.Program{{
		ID:        "program:news",
		ChannelID: "dispatcharr:news",
		Title:     "Current News",
		StartUnix: now - 60,
		EndUnix:   now + 1800,
	}}, now)
	done := make(chan struct{}, 1)
	syncer := &stubCatalogSyncer{store: store, done: done}
	server := NewHTTPRoutesServerWithSyncer(store, func() config.Settings { return settings }, syncer)

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: http.MethodPost,
		Path:   "/dispatcharr/api/guide/ping",
		Body:   []byte(`{"channelIds":["dispatcharr:news","dispatcharr:sports"]}`),
	})
	if err != nil {
		t.Fatalf("guide ping: %v", err)
	}
	if response.GetStatusCode() != http.StatusAccepted {
		t.Fatalf("expected partial guide to start refresh, got %d", response.GetStatusCode())
	}
	var payload GuidePingPayload
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal guide ping payload: %v", err)
	}
	if payload.Status != "refreshing" || payload.CurrentPrograms != 1 {
		t.Fatalf("expected one covered channel and refreshing status, got %+v", payload)
	}
	waitForStubSync(t, done)
}

func TestHTTPRoutesServerLegacyFavoriteRouteRejectsProcessGlobalState(t *testing.T) {
	t.Parallel()

	server := NewHTTPRoutesServer(cache.NewStore())
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: "POST",
		Path:   "/dispatcharr/api/favorites",
		Body:   []byte(`{"id":"xtream:1","enabled":true}`),
	})
	if err != nil {
		t.Fatalf("favorite route: %v", err)
	}
	if response.GetStatusCode() != http.StatusGone {
		t.Fatalf("expected 410, got %d", response.GetStatusCode())
	}
}

type stubCatalogSyncer struct {
	store        *cache.Store
	calls        int
	forceCalls   int
	channelCalls int
	mu           sync.Mutex
	block        <-chan struct{}
	done         chan<- struct{}
}

func (s *stubCatalogSyncer) ForceSyncNow(ctx context.Context, settings config.Settings, nowUnix int64) error {
	s.mu.Lock()
	s.forceCalls++
	s.mu.Unlock()
	return s.SyncNow(ctx, settings, nowUnix)
}

func (s *stubCatalogSyncer) RefreshGuideOnlyNow(ctx context.Context, settings config.Settings, nowUnix int64) error {
	return s.SyncNow(ctx, settings, nowUnix)
}

func (s *stubCatalogSyncer) RefreshChannelsNow(ctx context.Context, settings config.Settings, nowUnix int64) error {
	s.mu.Lock()
	s.channelCalls++
	s.mu.Unlock()
	return s.SyncNow(ctx, settings, nowUnix)
}

func (s *stubCatalogSyncer) SyncNow(_ context.Context, settings config.Settings, nowUnix int64) error {
	if s.block != nil {
		<-s.block
	}
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	source := model.LiveTVSource(model.SourceModeDirectLogin)
	source.ProfileAccess = &model.ProfileAccess{Status: "available", ProfileCount: 1}
	s.store.Replace(cache.Snapshot{
		ConfigKey: config.CatalogCacheKey(settings),
		Catalog: model.CatalogState{
			Source:   source,
			Channels: []model.Channel{{ID: "dispatcharr:news", Name: "News HD"}},
			Programs: []model.Program{{ID: "program:1", ChannelID: "dispatcharr:news", Title: "Morning News", StartUnix: 100, EndUnix: 200}},
		},
		Health: model.SyncHealth{LastSuccessUnix: nowUnix},
	})
	if s.done != nil {
		select {
		case s.done <- struct{}{}:
		default:
		}
	}
	return nil
}

func (s *stubCatalogSyncer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *stubCatalogSyncer) forceCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.forceCalls
}

func (s *stubCatalogSyncer) channelCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.channelCalls
}

func waitForStubSync(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for background refresh")
	}
}

func TestHTTPRoutesServerLegacyPreferencesRouteRejectsProcessGlobalState(t *testing.T) {
	t.Parallel()

	server := NewHTTPRoutesServer(cache.NewStore())
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: "POST",
		Path:   "/dispatcharr/api/preferences",
		Body:   []byte(`{"favorites":{"channel:1":true},"favoriteOrder":["channel:1","channel:3"],"autoFavorites":{"channel:2":true},"hiddenCategories":{"sports":true},"sportsFavoriteTeams":{"mlb:cin":true},"keywordPasses":[{"id":"keyword:world-cup","keyword":"World Cup","createdAt":1234}],"recentChannels":["channel:1"],"continueWatching":{"channel:1":{"plays":3}},"playback":{"streamMode":"redirect","outputFormat":"hls"},"categoryParsing":{"enabled":true,"mode":"delimiter","delimiter":"pipe","regex":"","output":""},"customGroups":[{"id":"group:spanish","name":"Spanish","order":10}],"customGroupMemberships":{"group:spanish":["channel:1","channel:2"]}}`),
	})
	if err != nil {
		t.Fatalf("preferences route: %v", err)
	}
	if response.GetStatusCode() != http.StatusGone {
		t.Fatalf("expected 410, got %d", response.GetStatusCode())
	}
}

func TestHTTPRoutesServerAdminSettingsRoutePersistsPayload(t *testing.T) {
	t.Parallel()

	server := NewHTTPRoutesServer(cache.NewStore())
	var persisted map[string]any
	server.adminPersister = func(_ context.Context, payload map[string]any) error {
		persisted = payload
		return nil
	}
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method:  "POST",
		Path:    "/dispatcharr/api/admin-settings",
		Headers: map[string]string{"x-silo-user-role": "admin"},
		Body:    []byte(`{"mode":"normal","delimiter":"pipe","virtualGroupLabel":" Virtual Categories ","virtualGroupSource":"profile_group","ecmURL":" https://ecm.example.test/manage ","allowRecordingsByDefault":false,"collapseDuplicateVirtualGroups":false,"inferChannelNameGroups":true,"categoryRenames":[{"sourcePath":" International | Arabic | Sports ","displayName":" International Sports "},{"sourcePath":"International | Arabic | Sports","displayName":"Duplicate Ignored"},{"sourcePath":"","displayName":"Nowhere"},{"sourcePath":"International | TV","displayName":""}],"categoryAliases":[{"sourcePath":" International | Arabic | Sports ","aliasPath":" Sports | Arabic "},{"sourcePath":"International | Arabic | Sports","aliasPath":"Sports | Arabic"},{"sourcePath":"International | Arabic | Sports","aliasPath":"World Cup | Arabic"},{"sourcePath":"","aliasPath":"Nowhere"},{"sourcePath":"International | Arabic | Sports","aliasPath":""}]}`),
	})
	if err != nil {
		t.Fatalf("admin settings route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}

	response, err = server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method:  "GET",
		Path:    "/dispatcharr/api/admin-settings",
		Headers: map[string]string{"x-silo-user-role": "admin"},
	})
	if err != nil {
		t.Fatalf("admin settings route: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal admin settings: %v", err)
	}
	if payload["mode"] != "delimiter" || payload["delimiter"] != "pipe" {
		t.Fatalf("expected admin settings to persist: %+v", payload)
	}
	if payload["virtualGroupLabel"] != "Virtual Categories" {
		t.Fatalf("expected virtual group label to persist: %+v", payload)
	}
	if payload["virtualGroupSource"] != "profile_group" {
		t.Fatalf("expected virtual group source to persist: %+v", payload)
	}
	if payload["ecmEnabled"] != true || payload["ecmURL"] != "https://ecm.example.test/manage" {
		t.Fatalf("expected ECM URL to persist: %+v", payload)
	}
	if payload["allowRecordingsByDefault"] != false {
		t.Fatalf("expected admin recording default to persist: %+v", payload)
	}
	if payload["collapseDuplicateVirtualGroups"] != false {
		t.Fatalf("expected duplicate virtual group collapse setting to persist: %+v", payload)
	}
	if payload["inferChannelNameGroups"] != true {
		t.Fatalf("expected channel-name group inference flag to persist: %+v", payload)
	}
	renames, ok := payload["categoryRenames"].([]any)
	if !ok || len(renames) != 1 {
		t.Fatalf("expected one normalized category rename, got %+v", payload["categoryRenames"])
	}
	firstRename, _ := renames[0].(map[string]any)
	if firstRename["sourcePath"] != "International | Arabic | Sports" || firstRename["displayName"] != "International Sports" {
		t.Fatalf("expected category rename to be trimmed and preserved, got %+v", firstRename)
	}
	aliases, ok := payload["categoryAliases"].([]any)
	if !ok || len(aliases) != 2 {
		t.Fatalf("expected two normalized category aliases, got %+v", payload["categoryAliases"])
	}
	firstAlias, _ := aliases[0].(map[string]any)
	secondAlias, _ := aliases[1].(map[string]any)
	if firstAlias["sourcePath"] != "International | Arabic | Sports" || firstAlias["aliasPath"] != "Sports | Arabic" {
		t.Fatalf("expected first category alias to be trimmed and preserved, got %+v", firstAlias)
	}
	if secondAlias["sourcePath"] != "International | Arabic | Sports" || secondAlias["aliasPath"] != "World Cup | Arabic" {
		t.Fatalf("expected second category alias to preserve another display path, got %+v", secondAlias)
	}
	if persisted["mode"] != "delimiter" || persisted["delimiter"] != "pipe" {
		t.Fatalf("expected admin settings to write through to host config: %+v", persisted)
	}
	if persisted["virtualGroupLabel"] != "Virtual Categories" {
		t.Fatalf("expected virtual group label to write through to host config: %+v", persisted)
	}
	if persisted["virtualGroupSource"] != "profile_group" {
		t.Fatalf("expected virtual group source to write through to host config: %+v", persisted)
	}
	if persisted["ecmEnabled"] != true || persisted["ecmURL"] != "https://ecm.example.test/manage" {
		t.Fatalf("expected ECM URL to write through to host config: %+v", persisted)
	}
	if persisted["allowRecordingsByDefault"] != false {
		t.Fatalf("expected admin recording default to write through to host config: %+v", persisted)
	}
	if persisted["collapseDuplicateVirtualGroups"] != false {
		t.Fatalf("expected duplicate virtual group collapse setting to write through to host config: %+v", persisted)
	}
	if persisted["inferChannelNameGroups"] != true {
		t.Fatalf("expected channel-name group inference flag to write through to host config: %+v", persisted)
	}
	persistedRenames, ok := persisted["categoryRenames"].([]map[string]string)
	if !ok || len(persistedRenames) != 1 {
		t.Fatalf("expected category renames to write through to host config: %+v", persisted)
	}
	persistedAliases, ok := persisted["categoryAliases"].([]map[string]string)
	if !ok || len(persistedAliases) != 2 {
		t.Fatalf("expected category aliases to write through to host config: %+v", persisted)
	}
}

func TestHTTPRoutesServerAdminSettingsRouteReportsHostPersistFailure(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "category-settings.json")
	server := NewHTTPRoutesServerWithSyncerAndAdminSettingsFile(cache.NewStore(), nil, nil, path)
	server.adminPersister = func(context.Context, map[string]any) error {
		return fmt.Errorf("host timeout")
	}
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method:  "POST",
		Path:    "/dispatcharr/api/admin-settings",
		Headers: map[string]string{"x-silo-user-role": "admin"},
		Body:    []byte(`{"mode":"delimiter","delimiter":"pipe","ecmURL":"https://ecm.example.test/manage"}`),
	})
	if err != nil {
		t.Fatalf("admin settings route: %v", err)
	}
	if response.GetStatusCode() != http.StatusBadGateway {
		t.Fatalf("expected 502 when host persistence fails, got %d", response.GetStatusCode())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected private file fallback to be saved: %v", err)
	}
	if !strings.Contains(string(data), `"ecmURL":"https://ecm.example.test/manage"`) {
		t.Fatalf("expected ECM URL in private file fallback: %s", data)
	}
	if server.store.HasAdminSettings() {
		t.Fatal("expected failed host persistence not to update the in-memory settings")
	}
}

func TestHTTPRoutesServerAdminSettingsRouteUsesDurableFileWithoutRuntimeHost(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "category-settings.json")
	server := NewHTTPRoutesServerWithSyncerAndAdminSettingsFile(cache.NewStore(), nil, nil, path)
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method:  http.MethodPost,
		Path:    "/dispatcharr/api/admin-settings",
		Headers: map[string]string{"x-silo-user-role": "admin"},
		Body:    []byte(`{"mode":"delimiter","delimiter":"pipe","ecmURL":"https://ecm.example.test/manage"}`),
	})
	if err != nil {
		t.Fatalf("admin settings route: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected durable local save without a runtime host, got %d: %s", response.GetStatusCode(), response.GetBody())
	}
	if !server.store.HasAdminSettings() {
		t.Fatal("expected durable local save to update in-memory settings")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read durable settings file: %v", err)
	}
	if !strings.Contains(string(data), `"ecmURL":"https://ecm.example.test/manage"`) {
		t.Fatalf("expected ECM URL in durable settings file: %s", data)
	}
}

func TestHTTPRoutesServerAdminSettingsRoutePersistsPayloadToFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "category-settings.json")
	server := NewHTTPRoutesServerWithSyncerAndAdminSettingsFile(cache.NewStore(), nil, nil, path)
	server.adminPersister = func(context.Context, map[string]any) error {
		return nil
	}
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method:  "POST",
		Path:    "/dispatcharr/api/admin-settings",
		Headers: map[string]string{"x-silo-user-role": "admin"},
		Body:    []byte(`{"mode":"admin_delimiter","delimiter":"dash","ecmEnabled":false,"ecmURL":" https://ecm.example.test/manage ","categoryAliases":[{"sourcePath":"International | Argentina | Sports","aliasPath":"Sports | Argentina"}],"groupAliases":[{"from":"International | Argentina | Sports"}]}`),
	})
	if err != nil {
		t.Fatalf("admin settings route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read admin settings file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat admin settings file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected owner-only admin settings file, got %o", info.Mode().Perm())
	}
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("decode admin settings file: %v", err)
	}
	if saved["mode"] != "delimiter" || saved["delimiter"] != "dash" {
		t.Fatalf("expected normalized admin settings file, got %+v", saved)
	}
	if saved["ecmEnabled"] != true || saved["ecmURL"] != "https://ecm.example.test/manage" {
		t.Fatalf("expected normalized ECM settings file, got %+v", saved)
	}
	if aliases, ok := saved["categoryAliases"].([]any); !ok || len(aliases) != 1 {
		t.Fatalf("expected normalized category aliases in settings file, got %+v", saved["categoryAliases"])
	}
	if _, ok := saved["groupAliases"]; ok {
		t.Fatalf("expected stale remapping keys to be stripped: %+v", saved)
	}

	nextServer := NewHTTPRoutesServerWithSyncerAndAdminSettingsFile(cache.NewStore(), nil, nil, path)
	response, err = nextServer.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method:  "GET",
		Path:    "/dispatcharr/api/admin-settings",
		Headers: map[string]string{"x-silo-user-role": "admin"},
	})
	if err != nil {
		t.Fatalf("admin settings route: %v", err)
	}
	var loaded map[string]any
	if err := json.Unmarshal(response.GetBody(), &loaded); err != nil {
		t.Fatalf("decode loaded admin settings: %v", err)
	}
	if loaded["mode"] != "delimiter" || loaded["delimiter"] != "dash" {
		t.Fatalf("expected admin settings to load from file: %+v", loaded)
	}
	if loaded["ecmEnabled"] != true || loaded["ecmURL"] != "https://ecm.example.test/manage" {
		t.Fatalf("expected ECM settings to load from file: %+v", loaded)
	}
	if aliases, ok := loaded["categoryAliases"].([]any); !ok || len(aliases) != 1 {
		t.Fatalf("expected category aliases to load from file: %+v", loaded["categoryAliases"])
	}
}

func TestHTTPRoutesServerAdminSettingsRouteReadsConfiguredPayload(t *testing.T) {
	t.Parallel()

	server := NewHTTPRoutesServerWithSettings(cache.NewStore(), func() config.Settings {
		return config.Settings{AdminSettings: json.RawMessage(`{"mode":"delimiter","delimiter":"pipe"}`)}
	})
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method:  "GET",
		Path:    "/dispatcharr/api/admin-settings",
		Headers: map[string]string{"x-silo-user-role": "admin"},
	})
	if err != nil {
		t.Fatalf("admin settings route: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal admin settings: %v", err)
	}
	if payload["mode"] != "delimiter" || payload["delimiter"] != "pipe" {
		t.Fatalf("expected configured admin settings: %+v", payload)
	}
}

func TestHTTPRoutesServerAdminSettingsRouteAllowsUserRead(t *testing.T) {
	t.Parallel()

	server := NewHTTPRoutesServerWithSettings(cache.NewStore(), func() config.Settings {
		return config.Settings{AdminSettings: json.RawMessage(`{"mode":"delimiter","delimiter":"pipe"}`)}
	})
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: "GET",
		Path:   "/dispatcharr/api/admin-settings",
	})
	if err != nil {
		t.Fatalf("admin settings route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200 for user admin settings read, got %d", response.GetStatusCode())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal admin settings: %v", err)
	}
	if payload["mode"] != "delimiter" || payload["delimiter"] != "pipe" {
		t.Fatalf("expected configured admin settings: %+v", payload)
	}
}

func TestHTTPRoutesServerAdminSettingsRouteRequiresSiloAdminRoleForPost(t *testing.T) {
	t.Parallel()

	response, err := NewHTTPRoutesServer(cache.NewStore()).Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: http.MethodPost,
		Path:   "/dispatcharr/api/admin-settings",
		Body:   []byte(`{"mode":"delimiter","delimiter":"pipe"}`),
	})
	if err != nil {
		t.Fatalf("admin settings route: %v", err)
	}
	if response.GetStatusCode() != 403 {
		t.Fatalf("expected 403 without Silo admin role, got %d", response.GetStatusCode())
	}
}

func TestHTTPRoutesServerAdminSettingsPublicReadHidesManagerURL(t *testing.T) {
	t.Parallel()

	server := NewHTTPRoutesServer(cache.NewStore())
	server.store.SetAdminSettings(json.RawMessage(`{"mode":"delimiter","delimiter":"pipe","ecmEnabled":true,"ecmURL":"https://manager.example/private"}`))
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: "/dispatcharr/api/admin-settings"})
	if err != nil {
		t.Fatalf("admin settings route: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.GetBody(), &payload); err != nil {
		t.Fatalf("unmarshal public admin settings: %v", err)
	}
	if payload["ecmURL"] != "" || payload["ecmEnabled"] != false {
		t.Fatalf("public admin settings exposed manager configuration: %+v", payload)
	}
}

func TestHTTPRoutesServerAdminSettingsRouteRejectsLegacyQueryTokenForWrite(t *testing.T) {
	t.Parallel()

	server := NewHTTPRoutesServer(cache.NewStore())
	query, _ := structpb.NewStruct(map[string]any{"admin_token": "legacy-token"})
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: http.MethodPost,
		Path:   "/dispatcharr/api/admin-settings",
		Query:  query,
		Body:   []byte(`{"mode":"delimiter","delimiter":"pipe"}`),
	})
	if err != nil {
		t.Fatalf("admin settings route: %v", err)
	}
	if response.GetStatusCode() != http.StatusForbidden {
		t.Fatalf("expected query token to be rejected, got %d", response.GetStatusCode())
	}
}

func TestHTTPRoutesServerWatchLifecycleUpdatesSessionState(t *testing.T) {
	t.Parallel()

	server := NewHTTPRoutesServer(cache.NewStore())
	startResponse, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: "POST",
		Path:   "/dispatcharr/api/watch/start",
		Body:   []byte(`{"itemKind":"channel","itemId":"xtream:1","itemName":"News HD"}`),
	})
	if err != nil {
		t.Fatalf("watch start route: %v", err)
	}
	if startResponse.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", startResponse.GetStatusCode())
	}
	var startPayload struct {
		Session cache.WatchSession `json:"session"`
	}
	if err := json.Unmarshal(startResponse.GetBody(), &startPayload); err != nil {
		t.Fatalf("unmarshal watch start payload: %v", err)
	}
	if startPayload.Session.ID == "" || startPayload.Session.ItemID != "xtream:1" {
		t.Fatalf("unexpected watch session: %+v", startPayload.Session)
	}

	heartbeatResponse, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: "POST",
		Path:   "/dispatcharr/api/watch/heartbeat",
		Body:   []byte(`{"sessionId":"` + startPayload.Session.ID + `"}`),
	})
	if err != nil {
		t.Fatalf("watch heartbeat route: %v", err)
	}
	if heartbeatResponse.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", heartbeatResponse.GetStatusCode())
	}

	stopResponse, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{
		Method: "POST",
		Path:   "/dispatcharr/api/watch/stop",
		Body:   []byte(`{"sessionId":"` + startPayload.Session.ID + `","reason":"test"}`),
	})
	if err != nil {
		t.Fatalf("watch stop route: %v", err)
	}
	if stopResponse.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", stopResponse.GetStatusCode())
	}
}

func TestHTTPRoutesServerStreamM3URoute(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source: model.LiveTVSource(model.SourceModeM3UXMLTV),
			Channels: []model.Channel{
				{ID: "m3u:news.hd", Name: "News HD", StreamURL: "https://dispatcharr.example.com/live/news.m3u8"},
			},
		},
	})
	server := NewHTTPRoutesServer(store)
	query, _ := structpb.NewStruct(map[string]any{"channel_id": "m3u:news.hd"})

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/stream", Query: query})
	if err != nil {
		t.Fatalf("stream route: %v", err)
	}
	if response.GetStatusCode() != 302 {
		t.Fatalf("expected 302, got %d", response.GetStatusCode())
	}
	if response.GetHeaders()["location"] != "https://dispatcharr.example.com/live/news.m3u8" {
		t.Fatalf("unexpected location header: %q", response.GetHeaders()["location"])
	}
}

func TestHTTPRoutesServerStreamXtreamRoute(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source: model.LiveTVSource(model.SourceModeXtream),
			Channels: []model.Channel{
				{ID: "xtream:1001", Name: "News HD"},
			},
		},
	})
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings {
		return config.Settings{
			SourceMode:      config.SourceModeXtream,
			XtreamBaseURL:   "https://dispatcharr.example.com",
			XtreamUsername:  "demo",
			XtreamPassword:  "secret",
			ChannelRefreshH: config.DefaultChannelRefreshHours,
			EPGRefreshH:     config.DefaultEPGRefreshHours,
		}
	})
	query, _ := structpb.NewStruct(map[string]any{"channel_id": "xtream:1001"})

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/stream", Query: query})
	if err != nil {
		t.Fatalf("stream route: %v", err)
	}
	if response.GetStatusCode() != 302 {
		t.Fatalf("expected 302, got %d", response.GetStatusCode())
	}
	if !strings.Contains(response.GetHeaders()["location"], "/live/demo/secret/1001") {
		t.Fatalf("unexpected location header: %q", response.GetHeaders()["location"])
	}
}

func TestHTTPRoutesServerStreamPreservesBrowserPlaybackQuery(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source: model.LiveTVSource(model.SourceModeXtream),
			Channels: []model.Channel{
				{ID: "xtream:1001", Name: "News HD"},
			},
		},
	})
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings {
		return config.Settings{
			SourceMode:      config.SourceModeXtream,
			XtreamBaseURL:   "https://dispatcharr.example.com",
			XtreamUsername:  "demo",
			XtreamPassword:  "secret",
			ChannelRefreshH: config.DefaultChannelRefreshHours,
			EPGRefreshH:     config.DefaultEPGRefreshHours,
		}
	})
	query, _ := structpb.NewStruct(map[string]any{
		"channel_id":     "xtream:1001",
		"output_profile": "2",
	})

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/stream", Query: query})
	if err != nil {
		t.Fatalf("stream route: %v", err)
	}
	location := response.GetHeaders()["location"]
	if !strings.Contains(location, "output_profile=2") {
		t.Fatalf("expected browser playback query in location header: %q", location)
	}
}

func TestHTTPRoutesServerVODStreamXtreamRoute(t *testing.T) {
	t.Parallel()

	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{
			Source: model.LiveTVSource(model.SourceModeXtream),
			Content: model.ContentState{
				VODItems: []model.VODItem{{ID: "vod:2001", Name: "Movie", Container: "mp4"}},
			},
		},
	})
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings {
		return config.Settings{
			SourceMode:      config.SourceModeXtream,
			XtreamBaseURL:   "https://dispatcharr.example.com",
			XtreamUsername:  "demo",
			XtreamPassword:  "secret",
			ChannelRefreshH: config.DefaultChannelRefreshHours,
			EPGRefreshH:     config.DefaultEPGRefreshHours,
		}
	})
	query, _ := structpb.NewStruct(map[string]any{"item_id": "vod:2001"})

	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/vod/stream", Query: query})
	if err != nil {
		t.Fatalf("vod stream route: %v", err)
	}
	if response.GetStatusCode() != 302 {
		t.Fatalf("expected 302, got %d", response.GetStatusCode())
	}
	if !strings.Contains(response.GetHeaders()["location"], "/movie/demo/secret/2001.mp4") {
		t.Fatalf("unexpected location header: %q", response.GetHeaders()["location"])
	}
}

func TestHTTPRoutesServerPlayerRoute(t *testing.T) {
	t.Parallel()

	server := NewHTTPRoutesServer(cache.NewStore())
	response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/dispatcharr/player"})
	if err != nil {
		t.Fatalf("player route: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("expected 200, got %d", response.GetStatusCode())
	}
	if !strings.Contains(string(response.GetBody()), `aria-label="Live TV sections"`) {
		t.Fatalf("expected app shell html body")
	}
	if !strings.Contains(string(response.GetBody()), `href="/" aria-label="Back to Silo"`) {
		t.Fatalf("expected back to Silo link")
	}
	body := string(response.GetBody())
	if strings.Contains(body, "cdn.jsdelivr.net") {
		t.Fatalf("expected player libraries to be served locally")
	}
	if strings.Contains(body, "__ASSET_PREFIX__") || strings.Contains(body, "__PLAYER_LIBRARIES__") {
		t.Fatalf("expected asset placeholders to be resolved")
	}
	if !strings.Contains(body, `src="assets/app.js?v=`) || !strings.Contains(body, `href="assets/app.css?v=`) || strings.Contains(body, "mpegts.js") {
		t.Fatalf("expected external application assets and on-demand player libraries")
	}
	if !strings.Contains(playerAppJavaScript(), "output_profile=2") {
		t.Fatalf("expected browser playback to request AAC Xtream profile")
	}
}

func TestPlayerAppApprovedUXPassContracts(t *testing.T) {
	// Keep these assertions at the embedded-asset boundary: they protect the
	// user-facing behavior without making formatting an API.
	t.Parallel()

	script := playerAppJavaScript()
	styles := playerStylesCSS()
	compactStyles := strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "").Replace(styles)
	requireScript := func(want string) {
		t.Helper()
		if !strings.Contains(script, want) {
			t.Fatalf("expected approved UX script contract %q", want)
		}
	}
	requireStyle := func(want string) {
		t.Helper()
		if !strings.Contains(styles, want) {
			t.Fatalf("expected approved UX style contract %q", want)
		}
	}
	functionBody := func(name string) string {
		t.Helper()
		start := strings.Index(script, "function "+name+"(")
		if start < 0 {
			t.Fatalf("expected %s function", name)
		}
		end := strings.Index(script[start:], "\nfunction ")
		if end < 0 {
			return script[start:]
		}
		return script[start : start+end]
	}
	// The guide should render only a bounded, overscanned channel window.
	requireScript(`class="guide-window-spacer"`)
	requireScript(`class="guide-window"`)
	requireScript(`overscan`)
	if strings.Contains(script, "function appendGuideRows(") || strings.Contains(script, "function isNearGuideEnd(") {
		t.Fatal("guide must not retain recursive append-at-end rendering")
	}

	// Details are the first action, including when upstream guide data is absent.
	openDetails := functionBody("openProgramDetails")
	if !strings.Contains(openDetails, "programIsGuidePlaceholder(program)") || !strings.Contains(openDetails, "state.programDetails") {
		t.Fatal("guide placeholders must open program details fallback state")
	}
	if strings.Contains(openDetails, "playChannel(channel)") {
		t.Fatal("opening program details must not directly play a fallback channel")
	}
	requireScript(`Program details unavailable`)
	recentCards := functionBody("rowCards")
	if !strings.Contains(recentCards, "currentProgram(channel)") || !strings.Contains(recentCards, `class=\"continue-card recent-channel-card\"`) || strings.Contains(recentCards, "channel.categoryName") {
		t.Fatal("recently watched cards must show current programming instead of internal channel groups")
	}

	// The VM integration test exercises details-first clicks, guide windowing,
	// and exact player return state. These checks keep their public hooks stable.
	requireScript(`playerReturnContext`)
	requireScript(`view: state.view`)
	requireScript(`scrollY: window.scrollY`)
	requireScript(`window.scrollTo(0, context.scrollY || 0)`)
	recoveryPanel := functionBody("recoveryPanelHTML")
	if strings.Contains(recoveryPanel, `class="recovery-panel" role="status"`) || !strings.Contains(recoveryPanel, `<span role="status" aria-live="polite">`) {
		t.Fatal("recovery controls must sit outside the live status region")
	}

	for _, want := range []string{
		`class="organization-preview"`,
		`Profile`,
		`Group`,
		`data-on-later-filter-group=`,
		`[data-onlater-time]`,
		`onLaterTime`,
		`class="on-later-filter-group`,
		`class="event-card no-art`,
		`class="recovery-panel`,
		`>Retry<`,
		`>Reload<`,
		`setAttribute("aria-current", "page")`,
	} {
		requireScript(want)
	}

	sportsCard := functionBody("renderSportsEventCard")
	if strings.Count(sportsCard, "event.leagueName") > 1 {
		t.Fatal("sports card must not render the league label more than once")
	}
	playerSports := functionBody("renderPlayerSportsDrawer")
	for _, want := range []string{`player-sports-drawer`, `Live &amp; upcoming`, `Sports channels`} {
		if !strings.Contains(playerSports, want) {
			t.Fatalf("sports-first player drawer must include %q", want)
		}
	}
	playerSportsEvents := functionBody("playerSportsEvents")
	if !strings.Contains(playerSportsEvents, `Number(channel.score || 0) >= 60`) {
		t.Fatal("sports-first player must hide low-confidence channel matches")
	}
	startSportsRefresh := functionBody("startPlayerSportsRefresh")
	if !strings.Contains(startSportsRefresh, `30000`) || !strings.Contains(startSportsRefresh, `state.playerSportsOpen`) {
		t.Fatal("sports-first player score refresh must be scoped to the open drawer")
	}
	eventWindows := functionBody("renderEventBroadcastWindows")
	for _, want := range []string{`event.windows`, `event-broadcast-window`, `Broadcast coverage windows`} {
		if !strings.Contains(eventWindows, want) {
			t.Fatalf("event cards must expose grouped coverage marker %q", want)
		}
	}
	normalizeEventRules := functionBody("normalizeEventKeywordRows")
	for _, want := range []string{`excludeKeywords`, `eventSeries`, `groupWindowMinutes`} {
		if !strings.Contains(normalizeEventRules, want) {
			t.Fatalf("event-series admin rules must preserve %q", want)
		}
	}

	for _, want := range []string{
		`.guide-scroll {`,
		`.shell.is-guide .main { display: grid; grid-template-rows: auto minmax(0, 1fr); min-height: 0; overflow: hidden; padding-bottom: 0; }`,
		`.shell.is-guide #view { min-height: 0; overflow: hidden; }`,
		`.shell.is-guide .guide-page { min-height: 0; height: 100%; max-height: none; }`,
		`.shell.is-guide .guide-scroll { min-height: 0; }`,
		`.guide-window-spacer`,
		`.guide-window`,
		`.recovery-panel`,
		`.filter-sections`,
		`.filter-section`,
		`.organization-preview`,
		`.event-card-body.no-art`,
		`.recent-channel-card`,
		`.search-result-card-art { width: 5.1rem; height: 5.1rem;`,
		`.search-result-card-art img, .search-result-card-art img.logo {`,
		`object-fit: contain; object-position: center;`,
		`.search-result-card-art img:not(.logo) { width: 100%; height: 100%; max-width: none; max-height: none; object-fit: cover; }`,
		`@media (max-width: 700px)`,
		`.topbar-primary`,
		`.topbar-actions`,
		`.player-sports-drawer`,
		`.player-sports-event.live`,
		`.event-broadcast-windows`,
		`.event-keyword-options`,
		`@media (prefers-reduced-motion: reduce)`,
	} {
		requireStyle(want)
	}
	if !strings.Contains(compactStyles, `.sports-card{`) || !strings.Contains(compactStyles, `.admin-status-strip{`) || !strings.Contains(compactStyles, `.custom-group-browser,.custom-group-members{`) || !strings.Contains(compactStyles, `border-radius:0.5rem;`) {
		t.Fatal("non-pill sports cards must keep an 8px-or-smaller radius")
	}
	if !strings.Contains(styles, `.time-head span:not(:first-child) { position: sticky; left: var(--epg-logo-col);`) {
		t.Fatal("guide time panes must remain frozen while scrolling horizontally")
	}
	if strings.Contains(styles, `letter-spacing: 0.04em`) {
		t.Fatal("interface labels must use neutral letter spacing")
	}
}

func TestHTTPRoutesServerPlayerAssetRoutes(t *testing.T) {
	t.Parallel()

	server := NewHTTPRoutesServer(cache.NewStore())
	for _, path := range []string{"/dispatcharr/assets/hls.min.js", "/dispatcharr/assets/mpegts.min.js", "/assets/hls.min.js", "/assets/mpegts.min.js"} {
		response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: path})
		if err != nil {
			t.Fatalf("asset route %s: %v", path, err)
		}
		if response.GetStatusCode() != 200 {
			t.Fatalf("expected 200 for %s, got %d", path, response.GetStatusCode())
		}
		if response.GetHeaders()["content-type"] != "application/javascript; charset=utf-8" {
			t.Fatalf("unexpected content type for %s: %q", path, response.GetHeaders()["content-type"])
		}
		if len(response.GetBody()) < 1024 {
			t.Fatalf("expected embedded player asset body for %s", path)
		}
	}
}

func TestHTTPRoutesServerApplicationAssetRoutes(t *testing.T) {
	t.Parallel()

	server := NewHTTPRoutesServer(cache.NewStore())
	tests := map[string]string{
		"/dispatcharr/assets/app.js":    "application/javascript; charset=utf-8",
		"/dispatcharr/assets/lineup.js": "application/javascript; charset=utf-8",
		"/dispatcharr/assets/app.css":   "text/css; charset=utf-8",
	}
	for path, contentType := range tests {
		response, err := server.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: http.MethodGet, Path: path})
		if err != nil {
			t.Fatalf("asset route %s: %v", path, err)
		}
		if response.GetStatusCode() != http.StatusOK || response.GetHeaders()["content-type"] != contentType {
			t.Fatalf("unexpected response for %s: status=%d content-type=%q", path, response.GetStatusCode(), response.GetHeaders()["content-type"])
		}
		if response.GetHeaders()["cache-control"] != "public, max-age=31536000, immutable" || len(response.GetBody()) == 0 {
			t.Fatalf("expected cacheable embedded asset for %s", path)
		}
	}
}

func TestPlayerAppUsesLightweightRefreshPolling(t *testing.T) {
	t.Parallel()

	script := playerAppJavaScript()
	if strings.Count(script, `getJSON("/dispatcharr/api/app")`) != 1 {
		t.Fatalf("app bootstrap endpoint should only be used for initial load")
	}
	for _, marker := range []string{
		`getJSON("/dispatcharr/api/status")`,
		`getJSON("/dispatcharr/api/guide")`,
		`getJSON("/dispatcharr/api/vod")`,
		`getJSON("/dispatcharr/api/series")`,
		`attempt < 300`,
	} {
		if !strings.Contains(script, marker) {
			t.Fatalf("expected lightweight refresh marker %q", marker)
		}
	}
}
