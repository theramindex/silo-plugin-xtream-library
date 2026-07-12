package plugin

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/cache"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/timeshift"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/dispatcharr"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xtream"
)

//go:embed assets/hls.min.js assets/mpegts.min.js
var playerAssets embed.FS

var (
	assetVersionOnce  sync.Once
	assetVersionValue string
)

type HTTPRoutesServer struct {
	pluginv1.UnimplementedHttpRoutesServer
	store               *cache.Store
	settingsProvider    func() config.Settings
	adminPersister      func(context.Context, map[string]any) error
	adminStorage        adminSettingsStorage
	coordinator         *RefreshCoordinator
	hydrateMu           sync.Mutex
	refreshMu           sync.Mutex
	guideWarmLastUnix   int64
	profileWarmLastUnix int64
	sportsProvider      sportsProvider
	sportsCache         sportsEventCache
	sportsMu            sync.Mutex
	timeShift           *timeshift.Manager
}

type catalogSyncer interface {
	SyncNow(ctx context.Context, settings config.Settings, nowUnix int64) error
}

type forceCatalogSyncer interface {
	ForceSyncNow(ctx context.Context, settings config.Settings, nowUnix int64) error
}

type guideOnlySyncer interface {
	RefreshGuideOnlyNow(ctx context.Context, settings config.Settings, nowUnix int64) error
}

func NewHTTPRoutesServer(store *cache.Store) *HTTPRoutesServer {
	return newHTTPRoutesServer(store, nil, nil)
}

func NewHTTPRoutesServerWithSettings(store *cache.Store, settingsProvider func() config.Settings) *HTTPRoutesServer {
	return newHTTPRoutesServer(store, settingsProvider, nil)
}

func NewHTTPRoutesServerWithSyncer(store *cache.Store, settingsProvider func() config.Settings, syncer catalogSyncer) *HTTPRoutesServer {
	return newHTTPRoutesServer(store, settingsProvider, syncer)
}

func NewHTTPRoutesServerWithSyncerAndAdminSettingsFile(store *cache.Store, settingsProvider func() config.Settings, syncer catalogSyncer, path string) *HTTPRoutesServer {
	server := newHTTPRoutesServer(store, settingsProvider, syncer)
	server.adminStorage = NewFileAdminSettingsStorage(path)
	return server
}

func NewHTTPRoutesServerWithCoordinatorAndAdminSettingsFile(store *cache.Store, settingsProvider func() config.Settings, coordinator *RefreshCoordinator, path string) *HTTPRoutesServer {
	server := newHTTPRoutesServer(store, settingsProvider, nil)
	server.coordinator = coordinator
	server.adminStorage = NewFileAdminSettingsStorage(path)
	return server
}

func newHTTPRoutesServer(store *cache.Store, settingsProvider func() config.Settings, syncer catalogSyncer) *HTTPRoutesServer {
	server := &HTTPRoutesServer{store: store, settingsProvider: settingsProvider, sportsProvider: newESPNSportsProvider(&http.Client{Timeout: 8 * time.Second}), timeShift: timeshift.NewManager("")}
	if syncer != nil {
		server.coordinator = NewRefreshCoordinator(syncer)
	}
	return server
}

type ChannelsPayload struct {
	SourceName string           `json:"sourceName"`
	Channels   []PublicChannel  `json:"channels"`
	Categories []model.Category `json:"categories"`
}

type PublicChannel struct {
	ID           string   `json:"id"`
	SourceID     string   `json:"sourceId"`
	Name         string   `json:"name"`
	Number       string   `json:"number,omitempty"`
	GuideID      string   `json:"guideId,omitempty"`
	LogoURL      string   `json:"logoUrl,omitempty"`
	CategoryID   string   `json:"categoryId,omitempty"`
	CategoryName string   `json:"categoryName,omitempty"`
	ProfileIDs   []string `json:"profileIds,omitempty"`
	StreamFormat string   `json:"streamFormat,omitempty"`
}

type PublicVODItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CategoryID  string `json:"categoryId,omitempty"`
	PosterURL   string `json:"posterUrl,omitempty"`
	Rating      string `json:"rating,omitempty"`
	Added       string `json:"added,omitempty"`
	Container   string `json:"container,omitempty"`
	Description string `json:"description,omitempty"`
}

type GuidePayload struct {
	Programs []model.Program `json:"programs"`
}

type guidePingRequest struct {
	ChannelIDs []string `json:"channelIds"`
}

type GuidePingPayload struct {
	Status          string `json:"status"`
	CheckedChannels int    `json:"checkedChannels"`
	CurrentPrograms int    `json:"currentPrograms"`
	Refreshing      bool   `json:"refreshing"`
}

type ContentPayload struct {
	Available  bool             `json:"available"`
	Reason     string           `json:"reason,omitempty"`
	Categories []model.Category `json:"categories"`
	Items      any              `json:"items"`
}

type RecordingsPayload struct {
	Available bool              `json:"available"`
	Reason    string            `json:"reason,omitempty"`
	Items     []json.RawMessage `json:"items"`
}

type RecordingCapabilityPayload struct {
	Available   bool   `json:"available"`
	CanSchedule bool   `json:"canSchedule"`
	Reason      string `json:"reason,omitempty"`
}

type scheduleRecordingRequest struct {
	ChannelID   string `json:"channelId"`
	ProgramID   string `json:"programId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	StartUnix   int64  `json:"startUnix"`
	EndUnix     int64  `json:"endUnix"`
}

type AppCapabilities struct {
	LiveTV                bool   `json:"liveTv"`
	Guide                 bool   `json:"guide"`
	VOD                   bool   `json:"vod"`
	Series                bool   `json:"series"`
	Recordings            bool   `json:"recordings"`
	Favorites             bool   `json:"favorites"`
	HiddenCategories      bool   `json:"hiddenCategories"`
	BackendProxySupported bool   `json:"backendProxySupported"`
	StreamMode            string `json:"streamMode"`
	NativeLiveTVExport    bool   `json:"nativeLiveTvExport"`
}

type AppPayload struct {
	Status       HealthPayload    `json:"status"`
	Source       model.Source     `json:"source"`
	Channels     []PublicChannel  `json:"channels"`
	Categories   []model.Category `json:"categories"`
	Capabilities AppCapabilities  `json:"capabilities"`
}

type watchRequest struct {
	SessionID string `json:"sessionId"`
	ItemKind  string `json:"itemKind"`
	ItemID    string `json:"itemId"`
	ItemName  string `json:"itemName"`
	Reason    string `json:"reason"`
}

func (s *HTTPRoutesServer) Handle(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if strings.HasPrefix(request.GetPath(), "/dispatcharr/timeshift/") {
		return s.handleTimeShiftMedia(request), nil
	}
	switch request.GetPath() {
	case "/dispatcharr", "/dispatcharr/player", "/dispatcharr/admin":
		return htmlResponse(http.StatusOK, s.playerPageHTML(request)), nil
	case "/dispatcharr/assets/hls.min.js", "/assets/hls.min.js":
		return playerLibraryAssetResponse("assets/hls.min.js")
	case "/dispatcharr/assets/mpegts.min.js", "/assets/mpegts.min.js":
		return playerLibraryAssetResponse("assets/mpegts.min.js")
	case "/dispatcharr/assets/app.js", "/assets/app.js":
		return playerUIAssetResponse("ui/app.js", "application/javascript; charset=utf-8")
	case "/dispatcharr/assets/lineup.js", "/assets/lineup.js":
		return playerUIAssetResponse("ui/lineup.js", "application/javascript; charset=utf-8")
	case "/dispatcharr/assets/app.css", "/assets/app.css":
		return playerUIAssetResponse("ui/styles.css", "text/css; charset=utf-8")
	case "/dispatcharr/status", "/dispatcharr/api/status":
		return s.respondJSON(http.StatusOK, s.healthPayload())
	case "/dispatcharr/api/refresh":
		return s.handleRefresh(ctx, request)
	case "/dispatcharr/api/refresh-channels":
		return s.handleChannelRefresh(ctx, request)
	case "/dispatcharr/api/app":
		s.ensureCatalogHydrated(ctx)
		return s.respondJSON(http.StatusOK, s.buildAppPayload())
	case "/dispatcharr/channels", "/dispatcharr/api/channels":
		s.ensureCatalogHydrated(ctx)
		return s.respondJSON(http.StatusOK, s.channelsPayload())
	case "/dispatcharr/guide", "/dispatcharr/api/guide":
		s.ensureCatalogHydrated(ctx)
		channelID := queryValue(request, "channel_id")
		programs := programsForChannel(s.store.Current().Catalog.Programs, channelID)
		sort.Slice(programs, func(i, j int) bool {
			return programs[i].StartUnix < programs[j].StartUnix
		})
		return s.respondJSON(http.StatusOK, GuidePayload{Programs: programs})
	case "/dispatcharr/api/guide/ping":
		return s.handleGuidePing(ctx, request)
	case "/dispatcharr/api/categories":
		s.ensureCatalogHydrated(ctx)
		return s.respondJSON(http.StatusOK, s.categoriesPayload())
	case "/dispatcharr/api/vod":
		s.ensureCatalogHydrated(ctx)
		return s.respondJSON(http.StatusOK, s.vodPayload())
	case "/dispatcharr/api/series":
		s.ensureCatalogHydrated(ctx)
		return s.respondJSON(http.StatusOK, s.seriesPayload())
	case "/dispatcharr/api/recordings/capability":
		return s.handleRecordingCapability(ctx)
	case "/dispatcharr/api/recordings":
		s.ensureCatalogHydrated(ctx)
		if request.GetMethod() == http.MethodPost {
			return s.handleScheduleRecording(ctx, request)
		}
		return s.handleRecordings(ctx)
	case "/dispatcharr/api/timeshift/start":
		return s.handleTimeShiftStart(request), nil
	case "/dispatcharr/api/timeshift/status":
		return s.handleTimeShiftStatus(request), nil
	case "/dispatcharr/api/timeshift/heartbeat":
		return s.handleTimeShiftHeartbeat(request), nil
	case "/dispatcharr/api/timeshift/stop":
		return s.handleTimeShiftStop(request), nil
	case "/dispatcharr/api/timeshift/admin-status":
		return s.handleTimeShiftAdminStatus(request), nil
	case "/dispatcharr/api/timeshift/clear":
		return s.handleTimeShiftClear(request), nil
	case "/dispatcharr/api/sports":
		s.ensureCatalogHydrated(ctx)
		return s.handleSports(ctx, request)
	case "/dispatcharr/api/sports/favorites":
		return s.handleSportsFavorite(request)
	case "/dispatcharr/api/events":
		s.ensureCatalogHydrated(ctx)
		return s.handleEvents(ctx, request)
	case "/dispatcharr/api/preferences":
		return s.handlePreferences(request)
	case "/dispatcharr/api/admin-settings":
		return s.handleAdminSettings(ctx, request)
	case "/dispatcharr/api/favorites":
		return s.handleFavorite(request)
	case "/dispatcharr/api/hidden-categories":
		return s.handleHiddenCategory(request)
	case "/dispatcharr/api/playback":
		return s.handlePlaybackSettings(request)
	case "/dispatcharr/api/watch/start":
		return s.handleWatchStart(request)
	case "/dispatcharr/api/watch/heartbeat":
		return s.handleWatchHeartbeat(request)
	case "/dispatcharr/api/watch/stop":
		return s.handleWatchStop(request)
	case "/dispatcharr/stream":
		s.ensureCatalogHydrated(ctx)
		channelID := queryValue(request, "channel_id")
		if strings.TrimSpace(channelID) == "" {
			return textResponse(http.StatusBadRequest, "missing channel_id query parameter"), nil
		}
		streamURL, err := s.resolveStreamURL(channelID)
		if err != nil {
			return textResponse(http.StatusNotFound, err.Error()), nil
		}
		streamURL = appendPlaybackQuery(streamURL, request)
		return redirectResponse(streamURL), nil
	case "/dispatcharr/vod/stream":
		s.ensureCatalogHydrated(ctx)
		itemID := queryValue(request, "item_id")
		if strings.TrimSpace(itemID) == "" {
			return textResponse(http.StatusBadRequest, "missing item_id query parameter"), nil
		}
		streamURL, err := s.resolveVODStreamURL(ctx, itemID)
		if err != nil {
			return textResponse(http.StatusNotFound, err.Error()), nil
		}
		return redirectResponse(streamURL), nil
	default:
		return textResponse(http.StatusNotFound, "route not found"), nil
	}
}

func (s *HTTPRoutesServer) ensureCatalogHydrated(ctx context.Context) {
	if s.coordinator == nil || s.settingsProvider == nil {
		return
	}

	settings := s.settingsProvider()
	current := s.store.Current()
	if len(current.Catalog.Channels) > 0 && catalogSnapshotMatchesSettings(current, settings) {
		s.startBackgroundProfileWarm(current, settings)
		return
	}

	s.hydrateMu.Lock()
	defer s.hydrateMu.Unlock()

	current = s.store.Current()
	if len(current.Catalog.Channels) > 0 && catalogSnapshotMatchesSettings(current, settings) {
		s.startBackgroundProfileWarm(current, settings)
		return
	}
	if len(current.Catalog.Channels) > 0 && !catalogSnapshotMatchesSettings(current, settings) {
		s.store.Replace(cache.Snapshot{
			Catalog:   model.CatalogState{Source: model.LiveTVSource(modelSourceModeForSettings(settings))},
			ConfigKey: config.CatalogCacheKey(settings),
		})
	}
	if err := settings.Validate(); err != nil {
		s.store.RecordFailure(time.Now().Unix(), err.Error())
		return
	}
	if err := s.coordinator.Run(ctx, RefreshCatalog, settings, time.Now().Unix()); err != nil {
		s.store.RecordFailure(time.Now().Unix(), err.Error())
		return
	}
	if len(s.store.Current().Catalog.Programs) == 0 {
		_, _ = s.startBackgroundGuideWarm(settings)
	}
}

func catalogSnapshotMatchesSettings(snapshot cache.Snapshot, settings config.Settings) bool {
	if len(snapshot.Catalog.Channels) == 0 {
		return false
	}
	return snapshot.Catalog.Source.Mode == modelSourceModeForSettings(settings) && snapshot.ConfigKey == config.CatalogCacheKey(settings)
}

func profileCatalogNeedsRefresh(snapshot cache.Snapshot, settings config.Settings) bool {
	if modelSourceModeForSettings(settings) != model.SourceModeDirectLogin {
		return false
	}
	access := snapshot.Catalog.Source.ProfileAccess
	return access == nil || access.Status == "unavailable"
}

func (s *HTTPRoutesServer) handleChannelRefresh(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if request.GetMethod() != http.MethodPost {
		return textResponse(http.StatusMethodNotAllowed, "channel refresh requires POST"), nil
	}
	if s.coordinator == nil || s.settingsProvider == nil {
		return textResponse(http.StatusServiceUnavailable, "catalog sync is not available"), nil
	}

	settings := s.settingsProvider()
	if err := settings.Validate(); err != nil {
		s.store.RecordFailure(time.Now().Unix(), err.Error())
		return textResponse(http.StatusBadRequest, err.Error()), nil
	}
	_, started := s.coordinator.Start(RefreshChannels, settings)
	status := http.StatusAccepted
	if !started {
		status = http.StatusOK
	}
	return s.respondJSON(status, s.buildAppPayload())
}

func (s *HTTPRoutesServer) startBackgroundProfileWarm(snapshot cache.Snapshot, settings config.Settings) bool {
	if s.coordinator == nil || !profileCatalogNeedsRefresh(snapshot, settings) {
		return false
	}

	now := time.Now().Unix()
	s.refreshMu.Lock()
	if s.profileWarmLastUnix > 0 && now-s.profileWarmLastUnix < 300 {
		s.refreshMu.Unlock()
		return false
	}
	s.profileWarmLastUnix = now
	s.refreshMu.Unlock()

	_, started := s.coordinator.Start(RefreshChannels, settings)
	return started
}

func modelSourceModeForSettings(settings config.Settings) model.SourceMode {
	switch settings.EffectiveSourceMode() {
	case config.SourceModeAPIKey:
		return model.SourceModeDirectLogin
	case config.SourceModeXtream:
		return model.SourceModeXtream
	case config.SourceModeM3UXMLTV:
		return model.SourceModeM3UXMLTV
	default:
		return model.SourceModeDirectLogin
	}
}

func (s *HTTPRoutesServer) handleRefresh(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if request.GetMethod() != http.MethodPost {
		return textResponse(http.StatusMethodNotAllowed, "refresh requires POST"), nil
	}
	if s.coordinator == nil || s.settingsProvider == nil {
		return textResponse(http.StatusServiceUnavailable, "catalog sync is not available"), nil
	}

	s.hydrateMu.Lock()
	defer s.hydrateMu.Unlock()

	settings := s.settingsProvider()
	if err := settings.Validate(); err != nil {
		s.store.RecordFailure(time.Now().Unix(), err.Error())
		return textResponse(http.StatusBadRequest, err.Error()), nil
	}
	started := s.startBackgroundRefresh(settings)
	status := http.StatusAccepted
	if !started {
		status = http.StatusOK
	}
	return s.respondJSON(status, s.buildAppPayload())
}

func (s *HTTPRoutesServer) handleGuidePing(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if request.GetMethod() != http.MethodPost {
		return textResponse(http.StatusMethodNotAllowed, "guide ping requires POST"), nil
	}
	if s.coordinator == nil || s.settingsProvider == nil {
		return textResponse(http.StatusServiceUnavailable, "catalog sync is not available"), nil
	}

	var payload guidePingRequest
	if len(request.GetBody()) > 0 {
		if err := json.Unmarshal(request.GetBody(), &payload); err != nil {
			return textResponse(http.StatusBadRequest, "invalid guide ping payload"), nil
		}
	}

	s.ensureCatalogHydrated(ctx)
	channelIDs := normalizeChannelIDs(payload.ChannelIDs)
	currentPrograms := currentProgramCountForChannels(s.store.Current().Catalog.Programs, channelIDs, time.Now().Unix())
	if len(channelIDs) == 0 || currentPrograms >= len(channelIDs) {
		return s.respondJSON(http.StatusOK, GuidePingPayload{Status: "fresh", CheckedChannels: len(channelIDs), CurrentPrograms: currentPrograms})
	}

	settings := s.settingsProvider()
	if err := settings.Validate(); err != nil {
		s.store.RecordFailure(time.Now().Unix(), err.Error())
		return textResponse(http.StatusBadRequest, err.Error()), nil
	}
	started, status := s.startBackgroundGuideWarm(settings)
	responseStatus := http.StatusOK
	if started {
		responseStatus = http.StatusAccepted
	}
	return s.respondJSON(responseStatus, GuidePingPayload{
		Status:          status,
		CheckedChannels: len(channelIDs),
		CurrentPrograms: currentPrograms,
		Refreshing:      started,
	})
}

func (s *HTTPRoutesServer) startBackgroundRefresh(settings config.Settings) bool {
	if s.coordinator == nil {
		return false
	}
	s.store.MarkEPGLoading()
	_, started := s.coordinator.Start(RefreshForce, settings)
	return started
}

func (s *HTTPRoutesServer) startBackgroundGuideWarm(settings config.Settings) (bool, string) {
	if s.coordinator == nil {
		return false, "unavailable"
	}

	now := time.Now().Unix()
	s.refreshMu.Lock()
	if s.guideWarmLastUnix > 0 && now-s.guideWarmLastUnix < 300 {
		s.refreshMu.Unlock()
		return false, "cooldown"
	}
	s.guideWarmLastUnix = now
	s.refreshMu.Unlock()

	s.store.MarkEPGLoading()
	_, started := s.coordinator.Start(RefreshGuide, settings)
	if !started {
		return false, "refreshing"
	}
	return true, "refreshing"
}

func (s *HTTPRoutesServer) buildAppPayload() AppPayload {
	snapshot := s.store.Current()
	return AppPayload{
		Status:       s.healthPayload(),
		Source:       snapshot.Catalog.Source,
		Channels:     publicChannels(snapshot.Catalog.Channels),
		Categories:   liveCategories(snapshot),
		Capabilities: appCapabilities(snapshot.Catalog.Source.Mode),
	}
}

func (s *HTTPRoutesServer) healthPayload() HealthPayload {
	payload := BuildHealthPayload(s.store.Current())
	if s.coordinator != nil {
		payload.Refresh = s.coordinator.Status()
	} else {
		payload.Refresh = RefreshJob{State: RefreshIdle}
	}
	return payload
}

func (s *HTTPRoutesServer) channelsPayload() ChannelsPayload {
	snapshot := s.store.Current()
	return ChannelsPayload{
		SourceName: snapshot.Catalog.Source.Name,
		Channels:   publicChannels(snapshot.Catalog.Channels),
		Categories: liveCategories(snapshot),
	}
}

func (s *HTTPRoutesServer) categoriesPayload() map[string][]model.Category {
	snapshot := s.store.Current()
	return map[string][]model.Category{
		"live":   liveCategories(snapshot),
		"vod":    snapshot.Catalog.Content.VODCategories,
		"series": snapshot.Catalog.Content.SeriesCategories,
	}
}

func (s *HTTPRoutesServer) vodPayload() ContentPayload {
	snapshot := s.store.Current()
	if snapshot.Catalog.Source.Mode == model.SourceModeM3UXMLTV {
		return ContentPayload{Available: false, Reason: "M3U/XMLTV mode only exposes Live TV and guide data.", Items: []model.VODItem{}}
	}
	return ContentPayload{
		Available:  len(snapshot.Catalog.Content.VODItems) > 0,
		Categories: snapshot.Catalog.Content.VODCategories,
		Items:      publicVODItems(snapshot.Catalog.Content.VODItems),
	}
}

func publicChannels(channels []model.Channel) []PublicChannel {
	result := make([]PublicChannel, 0, len(channels))
	for _, channel := range channels {
		result = append(result, PublicChannel{
			ID:           channel.ID,
			SourceID:     channel.SourceID,
			Name:         channel.Name,
			Number:       channel.Number,
			GuideID:      channel.GuideID,
			LogoURL:      channel.LogoURL,
			CategoryID:   channel.CategoryID,
			CategoryName: channel.CategoryName,
			ProfileIDs:   append([]string(nil), channel.ProfileIDs...),
			StreamFormat: publicStreamFormat(channel.StreamURL),
		})
	}
	return result
}

func publicStreamFormat(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	path := strings.ToLower(parsed.Path)
	if strings.HasSuffix(path, ".m3u8") || strings.Contains(path, "/proxy/hls/") {
		return "hls"
	}
	if strings.HasSuffix(path, ".ts") || strings.Contains(path, "/proxy/ts/") {
		return "mpegts"
	}
	return ""
}

func publicVODItems(items []model.VODItem) []PublicVODItem {
	result := make([]PublicVODItem, 0, len(items))
	for _, item := range items {
		result = append(result, PublicVODItem{
			ID:          item.ID,
			Name:        item.Name,
			CategoryID:  item.CategoryID,
			PosterURL:   item.PosterURL,
			Rating:      item.Rating,
			Added:       item.Added,
			Container:   item.Container,
			Description: item.Description,
		})
	}
	return result
}

func (s *HTTPRoutesServer) seriesPayload() ContentPayload {
	snapshot := s.store.Current()
	if snapshot.Catalog.Source.Mode == model.SourceModeM3UXMLTV {
		return ContentPayload{Available: false, Reason: "M3U/XMLTV mode only exposes Live TV and guide data.", Items: []model.SeriesItem{}}
	}
	return ContentPayload{
		Available:  len(snapshot.Catalog.Content.SeriesItems) > 0,
		Categories: snapshot.Catalog.Content.SeriesCategories,
		Items:      snapshot.Catalog.Content.SeriesItems,
	}
}

func (s *HTTPRoutesServer) handleRecordings(ctx context.Context) (*pluginv1.HandleHTTPResponse, error) {
	if !dvrEnabledForSource(s.store.Current().Catalog.Source.Mode) {
		return s.respondJSON(http.StatusOK, RecordingsPayload{Available: false, Reason: "Recordings require Dispatcharr Direct Connect.", Items: []json.RawMessage{}})
	}
	client, err := s.dispatcharrClient()
	if err != nil {
		return s.respondJSON(http.StatusOK, RecordingsPayload{Available: false, Reason: err.Error(), Items: []json.RawMessage{}})
	}
	recordings, err := client.Recordings(ctx)
	if err != nil {
		return s.respondJSON(http.StatusBadGateway, RecordingsPayload{Available: false, Reason: err.Error(), Items: []json.RawMessage{}})
	}
	return s.respondJSON(http.StatusOK, RecordingsPayload{Available: true, Items: enrichRecordings(client, recordings)})
}

func (s *HTTPRoutesServer) handleRecordingCapability(ctx context.Context) (*pluginv1.HandleHTTPResponse, error) {
	snapshot := s.store.Current()
	if !dvrEnabledForSource(snapshot.Catalog.Source.Mode) {
		return s.respondJSON(http.StatusOK, RecordingCapabilityPayload{
			Reason: "Recordings require Dispatcharr Direct Connect.",
		})
	}
	if s.settingsProvider == nil {
		return s.respondJSON(http.StatusOK, RecordingCapabilityPayload{
			Available: true,
			Reason:    "Unable to verify Dispatcharr recording permissions.",
		})
	}
	client, err := s.dispatcharrClient()
	if err != nil {
		return s.respondJSON(http.StatusOK, RecordingCapabilityPayload{
			Available: true,
			Reason:    "Unable to verify Dispatcharr recording permissions.",
		})
	}
	user, err := client.CurrentUser(ctx)
	if err != nil {
		return s.respondJSON(http.StatusOK, RecordingCapabilityPayload{
			Available: true,
			Reason:    "Unable to verify Dispatcharr recording permissions.",
		})
	}
	if user.UserLevel < 10 {
		return s.respondJSON(http.StatusOK, RecordingCapabilityPayload{
			Available: true,
			Reason:    "Scheduling requires a Dispatcharr admin account or Admin API Key.",
		})
	}
	return s.respondJSON(http.StatusOK, RecordingCapabilityPayload{Available: true, CanSchedule: true})
}

func (s *HTTPRoutesServer) handleScheduleRecording(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if !dvrEnabledForSource(s.store.Current().Catalog.Source.Mode) {
		return textResponse(http.StatusConflict, "recordings require Dispatcharr Direct Connect"), nil
	}
	var payload scheduleRecordingRequest
	if err := json.Unmarshal(request.GetBody(), &payload); err != nil {
		return textResponse(http.StatusBadRequest, "invalid recording payload"), nil
	}
	if strings.TrimSpace(payload.ChannelID) == "" {
		return textResponse(http.StatusBadRequest, "missing channel id"), nil
	}
	if payload.EndUnix <= payload.StartUnix || payload.EndUnix <= 0 {
		return textResponse(http.StatusBadRequest, "invalid recording window"), nil
	}
	if payload.StartUnix <= 0 {
		return textResponse(http.StatusBadRequest, "missing recording start"), nil
	}
	channel, ok := s.channelByID(payload.ChannelID)
	if !ok {
		return textResponse(http.StatusNotFound, "channel not found"), nil
	}
	client, err := s.dispatcharrClient()
	if err != nil {
		return s.respondJSON(http.StatusOK, RecordingsPayload{Available: false, Reason: err.Error(), Items: []json.RawMessage{}})
	}
	dispatcharrChannelID, err := s.dispatcharrChannelID(ctx, client, channel)
	if err != nil {
		return textResponse(http.StatusBadGateway, err.Error()), nil
	}
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = channel.Name
	}
	start := time.Unix(payload.StartUnix, 0).UTC()
	end := time.Unix(payload.EndUnix, 0).UTC()
	recording, err := client.CreateRecording(ctx, map[string]any{
		"channel":    dispatcharrChannelID,
		"start_time": start.Format(time.RFC3339),
		"end_time":   end.Format(time.RFC3339),
		"custom_properties": map[string]any{
			"program": map[string]any{
				"id":          strings.TrimSpace(payload.ProgramID),
				"title":       title,
				"description": strings.TrimSpace(payload.Description),
				"start_time":  start.Format(time.RFC3339),
				"end_time":    end.Format(time.RFC3339),
				"tvg_id":      strings.TrimSpace(channel.GuideID),
			},
			"channel_name": channel.Name,
			"source":       "silo.ramindex.dispatcharr",
		},
	})
	if err != nil {
		return scheduleRecordingErrorResponse(err), nil
	}
	return s.respondJSON(http.StatusOK, map[string]any{"ok": true, "recording": enrichRecording(client, recording)})
}

func scheduleRecordingErrorResponse(err error) *pluginv1.HandleHTTPResponse {
	message := err.Error()
	lower := strings.ToLower(message)
	if strings.Contains(lower, "unexpected status 401") ||
		strings.Contains(lower, "unexpected status 403") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "permission") {
		return textResponse(http.StatusForbidden, "Dispatcharr requires an admin account or API key to schedule recordings.")
	}
	return textResponse(http.StatusBadGateway, message)
}

func (s *HTTPRoutesServer) handlePreferences(request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	return userStateUnavailableResponse(), nil
}

func (s *HTTPRoutesServer) handleAdminSettings(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if request.GetMethod() == http.MethodPost && !s.adminSettingsAuthorized(request) {
		return textResponse(http.StatusForbidden, "Silo administrator access is required"), nil
	}
	if request.GetMethod() != http.MethodPost {
		if s.adminStorage != nil {
			saved, ok, err := s.adminStorage.Load()
			if err != nil {
				log.Printf("dispatcharr: load admin settings file failed: %v", err)
				return textResponse(http.StatusInternalServerError, "could not load admin settings"), nil
			}
			if ok {
				s.store.SetAdminSettings(saved)
				return s.respondAdminSettings(request, saved)
			}
		}
		if s.store.HasAdminSettings() {
			return s.respondAdminSettings(request, s.store.AdminSettings())
		}
		if s.settingsProvider != nil {
			if configured := s.settingsProvider().AdminSettings; len(configured) > 0 {
				return s.respondAdminSettings(request, configured)
			}
		}
		return s.respondAdminSettings(request, s.store.AdminSettings())
	}
	var payload map[string]any
	if err := json.Unmarshal(request.GetBody(), &payload); err != nil {
		return textResponse(http.StatusBadRequest, "invalid admin settings payload"), nil
	}
	normalized := normalizeAdminSettingsPayload(payload)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return textResponse(http.StatusBadRequest, "invalid admin settings payload"), nil
	}
	if s.adminStorage != nil {
		if err := s.adminStorage.Save(encoded); err != nil {
			log.Printf("dispatcharr: save admin settings file failed: %v", err)
			return textResponse(http.StatusInternalServerError, "could not save admin settings"), nil
		}
	}
	persistCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := s.persistAdminSettings(persistCtx, normalized); err != nil {
		log.Printf("dispatcharr: persist admin plugin settings failed: %v", err)
		return textResponse(http.StatusBadGateway, "could not save plugin settings"), nil
	}
	saved := s.store.SetAdminSettings(encoded)
	s.timeShiftConfig()
	return s.respondJSON(http.StatusOK, json.RawMessage(saved))
}

func (s *HTTPRoutesServer) respondAdminSettings(request *pluginv1.HandleHTTPRequest, settings json.RawMessage) (*pluginv1.HandleHTTPResponse, error) {
	if s.adminSettingsAuthorized(request) {
		return s.respondJSON(http.StatusOK, settings)
	}
	var public map[string]any
	if err := json.Unmarshal(settings, &public); err != nil {
		return textResponse(http.StatusInternalServerError, "invalid saved admin settings"), nil
	}
	public["ecmEnabled"] = false
	public["ecmURL"] = ""
	return s.respondJSON(http.StatusOK, public)
}

func (s *HTTPRoutesServer) adminSettingsAuthorized(request *pluginv1.HandleHTTPRequest) bool {
	return strings.EqualFold(strings.TrimSpace(headerValue(request.GetHeaders(), "x-silo-user-role")), "admin")
}

func headerValue(headers map[string]string, key string) string {
	for name, value := range headers {
		if strings.EqualFold(name, key) {
			return value
		}
	}
	return ""
}

func (s *HTTPRoutesServer) persistAdminSettings(ctx context.Context, payload map[string]any) error {
	if s.adminPersister == nil {
		return nil
	}
	return s.adminPersister(ctx, payload)
}

func (s *HTTPRoutesServer) handleFavorite(request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	return userStateUnavailableResponse(), nil
}

func (s *HTTPRoutesServer) handleHiddenCategory(request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	return userStateUnavailableResponse(), nil
}

func (s *HTTPRoutesServer) handlePlaybackSettings(request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	return userStateUnavailableResponse(), nil
}

func userStateUnavailableResponse() *pluginv1.HandleHTTPResponse {
	return textResponse(http.StatusGone, "user state is stored in the Silo user profile")
}

func (s *HTTPRoutesServer) handleWatchStart(request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if request.GetMethod() != http.MethodPost {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed"), nil
	}
	var payload watchRequest
	if err := json.Unmarshal(request.GetBody(), &payload); err != nil {
		return textResponse(http.StatusBadRequest, "invalid watch payload"), nil
	}
	if strings.TrimSpace(payload.ItemID) == "" {
		return textResponse(http.StatusBadRequest, "missing itemId"), nil
	}
	if strings.TrimSpace(payload.ItemKind) == "" {
		payload.ItemKind = "channel"
	}
	session := s.store.StartWatch(payload.ItemKind, payload.ItemID, payload.ItemName)
	return s.respondJSON(http.StatusOK, map[string]any{"session": session})
}

func (s *HTTPRoutesServer) handleWatchHeartbeat(request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if request.GetMethod() != http.MethodPost {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed"), nil
	}
	var payload watchRequest
	if err := json.Unmarshal(request.GetBody(), &payload); err != nil {
		return textResponse(http.StatusBadRequest, "invalid watch payload"), nil
	}
	session, ok := s.store.HeartbeatWatch(payload.SessionID)
	if !ok {
		return textResponse(http.StatusNotFound, "watch session not found"), nil
	}
	return s.respondJSON(http.StatusOK, map[string]any{"session": session})
}

func (s *HTTPRoutesServer) handleWatchStop(request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if request.GetMethod() != http.MethodPost {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed"), nil
	}
	var payload watchRequest
	if err := json.Unmarshal(request.GetBody(), &payload); err != nil {
		return textResponse(http.StatusBadRequest, "invalid watch payload"), nil
	}
	session, ok := s.store.StopWatch(payload.SessionID, payload.Reason)
	if !ok {
		return textResponse(http.StatusNotFound, "watch session not found"), nil
	}
	return s.respondJSON(http.StatusOK, map[string]any{"session": session})
}

func (s *HTTPRoutesServer) respondJSON(status int, value any) (*pluginv1.HandleHTTPResponse, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &pluginv1.HandleHTTPResponse{
		StatusCode: int32(status),
		Headers: map[string]string{
			"cache-control": "no-store",
			"content-type":  "application/json",
		},
		Body: payload,
	}, nil
}

func playerLibraryAssetResponse(path string) (*pluginv1.HandleHTTPResponse, error) {
	payload, err := playerAssets.ReadFile(path)
	if err != nil {
		return textResponse(http.StatusNotFound, "asset not found"), nil
	}
	return &pluginv1.HandleHTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"cache-control": "public, max-age=31536000, immutable",
			"content-type":  "application/javascript; charset=utf-8",
		},
		Body: payload,
	}, nil
}

func playerUIAssetResponse(path string, contentType string) (*pluginv1.HandleHTTPResponse, error) {
	payload, err := playerUIAssets.ReadFile(path)
	if err != nil {
		return textResponse(http.StatusNotFound, "asset not found"), nil
	}
	return &pluginv1.HandleHTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"cache-control": "public, max-age=31536000, immutable",
			"content-type":  contentType,
		},
		Body: payload,
	}, nil
}

func (s *HTTPRoutesServer) playerPageHTML(request *pluginv1.HandleHTTPRequest) string {
	body := strings.Replace(playerPageHTMLTemplate, "__SILO_THEME__", html.EscapeString(sanitizeThemeSlug(queryValue(request, "theme"))), 1)
	assetPrefix := "assets"
	if request.GetPath() == "/dispatcharr" {
		assetPrefix = "dispatcharr/assets"
	}
	body = strings.ReplaceAll(body, "__ASSET_PREFIX__", assetPrefix)
	body = strings.ReplaceAll(body, "__ASSET_VERSION__", pluginAssetVersion())
	if request.GetPath() == "/dispatcharr/admin" {
		body = removeTemplateBlock(body, "<!-- USER_NAV_START -->", "<!-- USER_NAV_END -->")
		body = replaceTemplateBlock(body, "<!-- USER_TOPBAR_START -->", "<!-- USER_TOPBAR_END -->", adminTopbarHTML())
		body = strings.Replace(body, "__APP_TITLE__", "Live TV Admin", 2)
		return strings.Replace(body, "__ROUTE_CLASS__", "is-admin", 1)
	}
	body = strings.Replace(body, "__APP_TITLE__", "Live TV", 2)
	return strings.Replace(body, "__ROUTE_CLASS__", "", 1)
}

func pluginAssetVersion() string {
	assetVersionOnce.Do(func() {
		hash := sha256.New()
		for _, asset := range []struct {
			fs   embed.FS
			path string
		}{
			{playerUIAssets, "ui/styles.css"},
			{playerUIAssets, "ui/lineup.js"},
			{playerUIAssets, "ui/app.js"},
			{playerAssets, "assets/hls.min.js"},
			{playerAssets, "assets/mpegts.min.js"},
		} {
			if payload, err := asset.fs.ReadFile(asset.path); err == nil {
				_, _ = hash.Write(payload)
			}
		}
		assetVersionValue = hex.EncodeToString(hash.Sum(nil))[:16]
	})
	return assetVersionValue
}

func removeTemplateBlock(body string, startMarker string, endMarker string) string {
	return replaceTemplateBlock(body, startMarker, endMarker, "")
}

func replaceTemplateBlock(body string, startMarker string, endMarker string, replacement string) string {
	start := strings.Index(body, startMarker)
	if start < 0 {
		return body
	}
	end := strings.Index(body[start:], endMarker)
	if end < 0 {
		return body
	}
	end += start + len(endMarker)
	return body[:start] + replacement + body[end:]
}

func adminTopbarHTML() string {
	return `<div class="admin-topbar"><div class="admin-title"><a class="back" href="/" aria-label="Back to Silo"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5 8.25 12l7.5-7.5"/></svg></a><h1>Live TV Admin</h1></div><nav id="admin-tabs" class="admin-tabs" aria-label="Live TV admin sections"></nav><div id="admin-actions" class="admin-actions"></div></div>`
}

func sanitizeThemeSlug(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, value)
}

func (s *HTTPRoutesServer) resolveStreamURL(channelID string) (string, error) {
	snapshot := s.store.Current()
	for _, channel := range snapshot.Catalog.Channels {
		if channel.ID != channelID {
			continue
		}
		if strings.TrimSpace(channel.StreamURL) != "" {
			return channel.StreamURL, nil
		}
		if strings.HasPrefix(channel.ID, "xtream:") && s.settingsProvider != nil {
			streamID, err := strconv.ParseInt(strings.TrimPrefix(channel.ID, "xtream:"), 10, 64)
			if err != nil {
				return "", fmt.Errorf("invalid xtream channel id")
			}
			settings := s.settingsProvider()
			baseURL, username, password := xtreamConnectionSettings(settings)
			client := xtream.NewClient(baseURL, username, password)
			streamURL := client.ResolveLiveStreamURL(streamID)
			if strings.TrimSpace(streamURL) == "" {
				return "", fmt.Errorf("unable to resolve stream url")
			}
			return streamURL, nil
		}
		return "", fmt.Errorf("stream url unavailable for channel")
	}
	return "", fmt.Errorf("channel not found")
}

func (s *HTTPRoutesServer) resolveVODStreamURL(_ context.Context, itemID string) (string, error) {
	snapshot := s.store.Current()
	for _, item := range snapshot.Catalog.Content.VODItems {
		if item.ID != itemID {
			continue
		}
		if strings.TrimSpace(item.StreamURL) != "" {
			return item.StreamURL, nil
		}
		if !strings.HasPrefix(item.ID, "vod:") || s.settingsProvider == nil {
			return "", fmt.Errorf("stream url unavailable for item")
		}
		streamID, err := strconv.ParseInt(strings.TrimPrefix(item.ID, "vod:"), 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid vod item id")
		}
		settings := s.settingsProvider()
		baseURL, username, password := xtreamConnectionSettings(settings)
		client := xtream.NewClient(baseURL, username, password)
		streamURL := client.ResolveVODStreamURL(streamID, item.Container)
		if strings.TrimSpace(streamURL) == "" {
			return "", fmt.Errorf("unable to resolve vod stream url")
		}
		return streamURL, nil
	}
	return "", fmt.Errorf("vod item not found")
}

func (s *HTTPRoutesServer) dispatcharrClient() (*dispatcharr.Client, error) {
	if s.settingsProvider == nil {
		return nil, fmt.Errorf("dispatcharr settings are unavailable")
	}
	settings := s.settingsProvider()
	switch settings.EffectiveSourceMode() {
	case config.SourceModeDirectLogin:
		if strings.TrimSpace(settings.DispatcharrURL) == "" || strings.TrimSpace(settings.DispatcharrUser) == "" || strings.TrimSpace(settings.DispatcharrPass) == "" {
			return nil, fmt.Errorf("dispatcharr direct login settings are incomplete")
		}
		return dispatcharr.NewLoginClient(settings.DispatcharrURL, settings.DispatcharrUser, settings.DispatcharrPass), nil
	case config.SourceModeAPIKey:
		if strings.TrimSpace(settings.DispatcharrURL) == "" || strings.TrimSpace(settings.DispatcharrAPIKey) == "" {
			return nil, fmt.Errorf("dispatcharr api key settings are incomplete")
		}
		return dispatcharr.NewAPIKeyClient(settings.DispatcharrURL, settings.DispatcharrAPIKey), nil
	default:
		return nil, fmt.Errorf("recordings require Dispatcharr direct or API key mode")
	}
}

func (s *HTTPRoutesServer) channelByID(channelID string) (model.Channel, bool) {
	snapshot := s.store.Current()
	for _, channel := range snapshot.Catalog.Channels {
		if channel.ID == channelID {
			return channel, true
		}
	}
	return model.Channel{}, false
}

func (s *HTTPRoutesServer) dispatcharrChannelID(ctx context.Context, client *dispatcharr.Client, channel model.Channel) (int, error) {
	upstreamChannels, err := client.Channels(ctx)
	if err != nil {
		return 0, fmt.Errorf("load Dispatcharr channels: %w", err)
	}
	streamUUID := dispatcharrStreamUUID(channel.StreamURL)
	for _, upstream := range upstreamChannels {
		if streamUUID != "" && strings.EqualFold(upstream.UUID.String(), streamUUID) {
			return strconv.Atoi(upstream.ID.String())
		}
	}
	for _, upstream := range upstreamChannels {
		if channel.GuideID != "" && strings.EqualFold(upstream.EffectiveTVGID.String(), channel.GuideID) {
			return strconv.Atoi(upstream.ID.String())
		}
		if channel.GuideID != "" && strings.EqualFold(upstream.TVGID.String(), channel.GuideID) {
			return strconv.Atoi(upstream.ID.String())
		}
	}
	for _, upstream := range upstreamChannels {
		name := strings.TrimSpace(channel.Name)
		if name != "" && strings.EqualFold(upstream.EffectiveName.String(), name) {
			return strconv.Atoi(upstream.ID.String())
		}
		if name != "" && strings.EqualFold(upstream.Name.String(), name) {
			return strconv.Atoi(upstream.ID.String())
		}
	}
	return 0, fmt.Errorf("unable to match %q to a Dispatcharr channel", channel.Name)
}

func dispatcharrStreamUUID(streamURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(streamURL))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 1 {
		return ""
	}
	for index := 0; index < len(parts)-1; index++ {
		if parts[index] == "stream" {
			return parts[index+1]
		}
	}
	return ""
}

func enrichRecordings(client *dispatcharr.Client, recordings []json.RawMessage) []json.RawMessage {
	enriched := make([]json.RawMessage, 0, len(recordings))
	for _, recording := range recordings {
		enriched = append(enriched, enrichRecording(client, recording))
	}
	return enriched
}

func enrichRecording(client *dispatcharr.Client, recording json.RawMessage) json.RawMessage {
	var object map[string]any
	if err := json.Unmarshal(recording, &object); err != nil {
		return recording
	}
	id := fmt.Sprint(object["id"])
	playbackURL := recordingPlaybackURL(client, id, object)
	object["_silo"] = map[string]any{
		"playback_url":   playbackURL,
		"playback_owner": "dispatcharr",
	}
	out, err := json.Marshal(object)
	if err != nil {
		return recording
	}
	return out
}

func recordingPlaybackURL(client *dispatcharr.Client, id string, object map[string]any) string {
	custom, _ := object["custom_properties"].(map[string]any)
	if raw, ok := custom["output_file_url"].(string); ok && strings.TrimSpace(raw) != "" {
		return client.AbsoluteURL(raw)
	}
	if raw, ok := custom["file_url"].(string); ok && strings.TrimSpace(raw) != "" {
		return client.AbsoluteURL(raw)
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(id) == "<nil>" {
		return ""
	}
	return client.AbsoluteURL("/api/channels/recordings/" + strings.TrimSpace(id) + "/file/")
}

func xtreamConnectionSettings(settings config.Settings) (string, string, string) {
	if settings.SourceMode == config.SourceModeDirectLogin {
		return settings.DispatcharrURL, settings.DispatcharrUser, settings.DispatcharrPass
	}
	return settings.XtreamBaseURL, settings.XtreamUsername, settings.XtreamPassword
}

func programsForChannel(programs []model.Program, channelID string) []model.Program {
	if strings.TrimSpace(channelID) == "" {
		return append([]model.Program(nil), programs...)
	}
	filtered := make([]model.Program, 0, len(programs))
	for _, program := range programs {
		if program.ChannelID == channelID {
			filtered = append(filtered, program)
		}
	}
	return filtered
}

func normalizeChannelIDs(ids []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		value := strings.TrimSpace(id)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func currentProgramCountForChannels(programs []model.Program, channelIDs []string, nowUnix int64) int {
	if len(channelIDs) == 0 {
		return 0
	}
	channelSet := make(map[string]bool, len(channelIDs))
	for _, id := range channelIDs {
		channelSet[id] = true
	}
	channelsWithPrograms := map[string]bool{}
	for _, program := range programs {
		if !channelSet[program.ChannelID] {
			continue
		}
		start := program.StartUnix
		end := program.EndUnix
		if end == 0 {
			end = start + 1800
		}
		if start <= nowUnix+1800 && end >= nowUnix-300 {
			channelsWithPrograms[program.ChannelID] = true
		}
	}
	return len(channelsWithPrograms)
}

func liveCategories(snapshot cache.Snapshot) []model.Category {
	if len(snapshot.Catalog.Content.LiveCategories) > 0 {
		return snapshot.Catalog.Content.LiveCategories
	}
	seen := map[string]bool{}
	categories := make([]model.Category, 0)
	for _, channel := range snapshot.Catalog.Channels {
		if channel.CategoryID == "" || seen[channel.CategoryID] {
			continue
		}
		seen[channel.CategoryID] = true
		name := channel.CategoryName
		if name == "" {
			name = "Group " + channel.CategoryID
		}
		categories = append(categories, model.Category{ID: channel.CategoryID, Name: name, Kind: "live"})
	}
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Name < categories[j].Name
	})
	return categories
}

func appCapabilities(sourceMode model.SourceMode) AppCapabilities {
	return AppCapabilities{
		LiveTV:                true,
		Guide:                 true,
		VOD:                   true,
		Series:                true,
		Recordings:            dvrEnabledForSource(sourceMode),
		Favorites:             true,
		HiddenCategories:      true,
		BackendProxySupported: false,
		StreamMode:            "redirect",
		NativeLiveTVExport:    false,
	}
}

func dvrEnabledForSource(sourceMode model.SourceMode) bool {
	return sourceMode == model.SourceModeDirectLogin || sourceMode == model.SourceModeAPIKey
}

func queryValue(request *pluginv1.HandleHTTPRequest, key string) string {
	query := request.GetQuery()
	if query == nil {
		return ""
	}
	value := query.AsMap()[key]
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func appendPlaybackQuery(streamURL string, request *pluginv1.HandleHTTPRequest) string {
	parsed, err := url.Parse(streamURL)
	if err != nil {
		return streamURL
	}
	values := parsed.Query()
	for _, key := range []string{"output_profile", "output_format", "output"} {
		value := strings.TrimSpace(queryValue(request, key))
		if value != "" {
			values.Set(key, value)
		}
	}
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

func redirectResponse(location string) *pluginv1.HandleHTTPResponse {
	return &pluginv1.HandleHTTPResponse{
		StatusCode: http.StatusFound,
		Headers: map[string]string{
			"location": location,
		},
	}
}

func htmlResponse(status int, body string) *pluginv1.HandleHTTPResponse {
	return &pluginv1.HandleHTTPResponse{
		StatusCode: int32(status),
		Headers: map[string]string{
			"cache-control": "no-store",
			"content-type":  "text/html; charset=utf-8",
		},
		Body: []byte(body),
	}
}

func textResponse(status int, message string) *pluginv1.HandleHTTPResponse {
	return &pluginv1.HandleHTTPResponse{
		StatusCode: int32(status),
		Headers: map[string]string{
			"content-type": "text/plain; charset=utf-8",
		},
		Body: []byte(message),
	}
}
