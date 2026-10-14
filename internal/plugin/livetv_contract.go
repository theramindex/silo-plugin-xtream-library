package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

// The livetv/v1 contract (testdata/livetv-v1/contract.md) is what Silo's
// Jellyfin bridge calls to list and play channels without the web app.

const livetvContractVersion = 1

type livetvInfo struct {
	ContractVersion int      `json:"contractVersion"`
	Name            string   `json:"name"`
	StreamHosts     []string `json:"streamHosts"`
}

type livetvChannel struct {
	ID      string   `json:"id"`
	Number  string   `json:"number"`
	Name    string   `json:"name"`
	LogoURL string   `json:"logoUrl"`
	GroupID string   `json:"groupId"`
	Group   string   `json:"group"`
	Folders []string `json:"folders,omitempty"`
}

type livetvStream struct {
	URL       string `json:"url"`
	Container string `json:"container"`
	Mode      string `json:"mode"`
	Leased    bool   `json:"leased,omitempty"`
}

type livetvError struct {
	Code string `json:"code"`
}

func (s *HTTPRoutesServer) handleLiveTVInfo(ctx context.Context) *pluginv1.HandleHTTPResponse {
	s.ensureCatalogHydrated(ctx)
	return jsonHTTPResponse(http.StatusOK, livetvInfo{
		ContractVersion: livetvContractVersion,
		Name:            "XC for Silo",
		StreamHosts:     livetvStreamHosts(s.liveTVSettings(), s.store.Current().Catalog.Channels),
	})
}

func (s *HTTPRoutesServer) handleLiveTVChannels(ctx context.Context) *pluginv1.HandleHTTPResponse {
	s.ensureCatalogHydrated(ctx)
	snapshot := s.store.Current()
	channels := snapshot.Catalog.Channels
	folders := livetvChannelFolders(s.livetvFolderSettings(snapshot.Catalog.Source, liveCategories(snapshot)), channels)
	lineup := make([]livetvChannel, 0, len(channels))
	for _, channel := range channels {
		lineup = append(lineup, livetvChannel{
			ID:      channel.ID,
			Number:  channel.Number,
			Name:    channel.Name,
			LogoURL: livetvLogoURL(channel.LogoURL),
			GroupID: channel.CategoryID,
			Group:   channel.CategoryName,
			Folders: folders[channel.ID],
		})
	}
	return jsonHTTPResponse(http.StatusOK, lineup)
}

func (s *HTTPRoutesServer) handleLiveTVStream(ctx context.Context, request *pluginv1.HandleHTTPRequest) *pluginv1.HandleHTTPResponse {
	channelID := strings.TrimSpace(queryValue(request, "channel_id"))
	if channelID == "" {
		return textResponse(http.StatusBadRequest, "missing channel_id query parameter")
	}
	format := strings.ToLower(strings.TrimSpace(queryValue(request, "format")))
	// An empty extension keeps the source's own live format.
	ext := ""
	switch format {
	case "":
		format = "any"
	case "any":
	case "ts":
		ext = "ts"
	case "hls":
		ext = "m3u8"
	default:
		return textResponse(http.StatusBadRequest, "format must be ts, hls or any")
	}

	sessionID := strings.TrimSpace(queryValue(request, "session_id"))
	if len(sessionID) > livetvMaxSessionIDLength {
		return textResponse(http.StatusBadRequest, "session_id is too long")
	}
	// Only Xtream channels use pool accounts; other channels hold nothing.
	leased := sessionID != "" && strings.HasPrefix(channelID, "xtream:")
	poolSessionID := ""
	if leased {
		poolSessionID = livetvPoolSessionID(sessionID)
	}

	s.ensureCatalogHydrated(ctx)
	streamURL, err := s.resolveChannelStreamURL(channelID, poolSessionID, ext)
	if errors.Is(err, errAccountPoolFull) {
		return jsonHTTPResponse(http.StatusConflict, livetvError{Code: "connection_limit"})
	}
	if err != nil {
		return jsonHTTPResponse(http.StatusNotFound, livetvError{Code: "channel_not_found"})
	}
	streamURL = appendPlaybackQuery(streamURL, request)

	container := "ts"
	if publicStreamFormat(streamURL) == "hls" {
		container = "hls"
	}
	if format != "any" && format != container {
		if leased {
			s.accountPool.Release(poolSessionID)
		}
		return jsonHTTPResponse(http.StatusUnprocessableEntity, livetvError{Code: "format_unavailable"})
	}

	mode := "redirect"
	if streamURLExposesSecrets(streamURL, s.liveTVSettings()) {
		mode = "server"
	}
	return jsonHTTPResponse(http.StatusOK, livetvStream{URL: streamURL, Container: container, Mode: mode, Leased: leased})
}

// livetvMaxSessionIDLength bounds a contract session_id.
const livetvMaxSessionIDLength = 128

// livetvPoolSessionID keeps Silo's play sessions apart from the web app's
// watch sessions in the shared account pool.
func livetvPoolSessionID(sessionID string) string {
	return "silo-livetv:" + sessionID
}

// handleLiveTVSession serves the contract's session heartbeat and stop routes.
// A heartbeat renews the session's account lease; stop frees it.
func (s *HTTPRoutesServer) handleLiveTVSession(request *pluginv1.HandleHTTPRequest, stop bool) *pluginv1.HandleHTTPResponse {
	if request.GetMethod() != http.MethodPost {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed")
	}
	sessionID := strings.TrimSpace(queryValue(request, "session_id"))
	if sessionID == "" || len(sessionID) > livetvMaxSessionIDLength {
		return textResponse(http.StatusBadRequest, "missing or invalid session_id")
	}
	poolSessionID := livetvPoolSessionID(sessionID)
	if stop {
		s.accountPool.Release(poolSessionID)
	} else if !s.accountPool.Touch(poolSessionID) {
		return jsonHTTPResponse(http.StatusNotFound, livetvError{Code: "session_not_found"})
	}
	return jsonHTTPResponse(http.StatusOK, map[string]any{})
}

func (s *HTTPRoutesServer) liveTVSettings() config.Settings {
	if s.settingsProvider == nil {
		return config.Settings{}
	}
	return s.settingsProvider()
}

// livetvStreamHosts lists every host a stream URL may point at, as host[:port],
// so Silo can refuse to fetch anything else on the first hop.
func livetvStreamHosts(settings config.Settings, channels []model.Channel) []string {
	seen := map[string]bool{}
	add := func(raw string) {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed.Host == "" {
			return
		}
		seen[strings.ToLower(parsed.Host)] = true
	}
	for _, source := range settings.EffectiveXtreamSources() {
		add(source.BaseURL)
	}
	for _, raw := range []string{settings.XtreamBaseURL, settings.M3UURL, settings.DispatcharrURL} {
		add(raw)
	}
	for _, channel := range channels {
		add(channel.StreamURL)
	}
	hosts := make([]string, 0, len(seen))
	for host := range seen {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}

func livetvLogoURL(raw string) string {
	logo := strings.TrimSpace(raw)
	lower := strings.ToLower(logo)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return logo
	}
	return ""
}

func jsonHTTPResponse(status int, value any) *pluginv1.HandleHTTPResponse {
	payload, err := json.Marshal(value)
	if err != nil {
		return textResponse(http.StatusInternalServerError, "unable to encode response")
	}
	return &pluginv1.HandleHTTPResponse{
		StatusCode: int32(status),
		Headers: map[string]string{
			"cache-control": "no-store",
			"content-type":  "application/json",
		},
		Body: payload,
	}
}

// livetvProgram is one guide entry in GET /silo-compat/livetv/v1/programs.
type livetvProgram struct {
	ID         string   `json:"id"`
	ChannelID  string   `json:"channelId"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	StartUnix  int64    `json:"startUnix"`
	EndUnix    int64    `json:"endUnix"`
	ImageURL   string   `json:"imageUrl"`
	Categories []string `json:"categories"`
	livetvFlags
	// Featured marks a program matched to a game in the sports guide.
	Featured bool `json:"featured"`
}

type livetvPrograms struct {
	Programs []livetvProgram `json:"programs"`
}

// livetvFlags classifies a program for Jellyfin's Movies, Sports, News, Kids
// and Shows rows. A program may set none or several.
type livetvFlags struct {
	IsMovie  bool `json:"isMovie"`
	IsSports bool `json:"isSports"`
	IsNews   bool `json:"isNews"`
	IsKids   bool `json:"isKids"`
	IsSeries bool `json:"isSeries"`
	IsLive   bool `json:"isLive"`
}

func (f livetvFlags) has(class string) bool {
	switch class {
	case "movie":
		return f.IsMovie
	case "sports":
		return f.IsSports
	case "news":
		return f.IsNews
	case "kids":
		return f.IsKids
	case "series":
		return f.IsSeries
	}
	return true
}

func (f livetvFlags) classified() bool {
	return f.IsMovie || f.IsSports || f.IsNews || f.IsKids || f.IsSeries
}

const (
	livetvProgramsMaxChannels   = 2000
	livetvProgramsDefaultLimit  = 200
	livetvProgramsMaxLimit      = 2000
	livetvSportsDefaultDuration = 3 * 60 * 60
)

type livetvProgramsQuery struct {
	Window     guideWindow
	ChannelIDs []string
	Class      string
	// Limit is the most programs to return; 0 means no limit.
	Limit int
}

func parseLiveTVProgramsQuery(request *pluginv1.HandleHTTPRequest) (livetvProgramsQuery, bool) {
	var query livetvProgramsQuery
	start, err := strconv.ParseInt(strings.TrimSpace(queryValue(request, "start")), 10, 64)
	if err != nil {
		return query, false
	}
	end, err := strconv.ParseInt(strings.TrimSpace(queryValue(request, "end")), 10, 64)
	if err != nil || start >= end {
		return query, false
	}
	query.Window = guideWindow{Start: start, End: end}

	query.Class = strings.ToLower(strings.TrimSpace(queryValue(request, "class")))
	switch query.Class {
	case "", "movie", "sports", "news", "kids", "series":
	default:
		return query, false
	}

	seen := map[string]bool{}
	for _, value := range strings.Split(queryValue(request, "channel_ids"), ",") {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		query.ChannelIDs = append(query.ChannelIDs, value)
	}
	if len(query.ChannelIDs) > livetvProgramsMaxChannels {
		return query, false
	}

	if len(query.ChannelIDs) == 0 {
		query.Limit = livetvProgramsDefaultLimit
	}
	if raw := strings.TrimSpace(queryValue(request, "limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return query, false
		}
		query.Limit = limit
	}
	if len(query.ChannelIDs) == 0 && query.Limit > livetvProgramsMaxLimit {
		query.Limit = livetvProgramsMaxLimit
	}
	return query, true
}

// handleLiveTVPrograms serves the guide grid. Programs come from the
// per-snapshot guide index, ordered by lineup position (or channel_ids order),
// then start time, then ID, and are classified by livetvClassifier with sports
// enrichment from the already-prepared sports guide. Text was decoded by
// decodeGuideProgramText when the index was built.
func (s *HTTPRoutesServer) handleLiveTVPrograms(ctx context.Context, request *pluginv1.HandleHTTPRequest) *pluginv1.HandleHTTPResponse {
	query, ok := parseLiveTVProgramsQuery(request)
	if !ok {
		return jsonHTTPResponse(http.StatusBadRequest, livetvError{Code: "bad_request"})
	}
	s.ensureCatalogHydrated(ctx)
	index := s.guideIndex.get(s.store.Current())
	sports := s.livetvSportsSlots()
	programs := livetvSelectPrograms(index, query, sports)
	// The guide is loaded per channel on demand; see livetv_guide_fill.go.
	s.startLiveTVGuideFill(index, query)
	return jsonHTTPResponse(http.StatusOK, livetvPrograms{Programs: programs})
}

type livetvCandidate struct {
	program  model.Program
	position int
	flags    livetvFlags
	slot     *livetvSportsSlot
}

func livetvSelectPrograms(index *guideIndex, query livetvProgramsQuery, sports map[string][]livetvSportsSlot) []livetvProgram {
	classifier := newLivetvClassifier()
	classify := func(candidate *livetvCandidate) {
		channel := index.lineup[candidate.position]
		candidate.slot = livetvMatchSportsSlot(sports[candidate.program.ChannelID], candidate.program)
		candidate.flags = classifier.flags(candidate.program, channel, candidate.slot != nil)
		if candidate.slot == nil && livetvFillerTitle(candidate.program.Title) {
			// Filler such as "NO EVENT" stays in the grid but in no row.
			candidate.flags = livetvFlags{}
		}
		if candidate.slot != nil && candidate.slot.Live {
			candidate.flags.IsLive = true
		}
	}

	var candidates []livetvCandidate
	if len(query.ChannelIDs) > 0 {
		// Each channel's programs are already ordered by start then ID, so
		// walking channel_ids in order yields the contract order directly.
		for _, channelID := range query.ChannelIDs {
			position, ok := index.lineupPosition[channelID]
			if !ok {
				continue
			}
			guide := index.byChannel[channelID]
			for _, program := range windowPrograms(guide.programs, guide.maxDuration, query.Window) {
				candidate := livetvCandidate{program: program, position: position}
				classify(&candidate)
				if !candidate.flags.has(query.Class) {
					continue
				}
				candidates = append(candidates, candidate)
				if query.Limit > 0 && len(candidates) >= query.Limit {
					return livetvBuildPrograms(candidates)
				}
			}
		}
		return livetvBuildPrograms(candidates)
	}

	// Whole lineup: the windowed slice is a fresh copy ordered by start then
	// ID, so a stable sort on (featured first, lineup position) gives the
	// contract order. With a class filter every windowed program is
	// classified up front; without one only the programs that survive the
	// limit are, though every one is matched against the sports guide, which
	// is a map lookup.
	windowed := windowPrograms(index.all, index.allMaxDuration, query.Window)
	candidates = make([]livetvCandidate, 0, len(windowed))
	for _, program := range windowed {
		position, ok := index.lineupPosition[program.ChannelID]
		if !ok {
			continue
		}
		candidate := livetvCandidate{program: program, position: position}
		if query.Class != "" {
			classify(&candidate)
			if !candidate.flags.has(query.Class) {
				continue
			}
		} else {
			candidate.slot = livetvMatchSportsSlot(sports[program.ChannelID], program)
		}
		candidates = append(candidates, candidate)
	}
	// Sort small references rather than whole candidates.
	type ref struct {
		index, position int32
		featured        bool
	}
	refs := make([]ref, len(candidates))
	for i := range candidates {
		refs[i] = ref{index: int32(i), position: int32(candidates[i].position), featured: candidates[i].slot != nil}
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].featured != refs[j].featured {
			return refs[i].featured
		}
		return refs[i].position < refs[j].position
	})
	if query.Limit > 0 && len(refs) > query.Limit {
		refs = refs[:query.Limit]
	}
	ordered := make([]livetvCandidate, 0, len(refs))
	for _, r := range refs {
		candidate := candidates[r.index]
		if query.Class == "" {
			classify(&candidate)
		}
		ordered = append(ordered, candidate)
	}
	return livetvBuildPrograms(ordered)
}

func livetvBuildPrograms(candidates []livetvCandidate) []livetvProgram {
	programs := make([]livetvProgram, 0, len(candidates))
	for _, candidate := range candidates {
		program := candidate.program
		categories := program.Categories
		if categories == nil {
			categories = []string{}
		}
		item := livetvProgram{
			ID:          program.ID,
			ChannelID:   program.ChannelID,
			Title:       program.Title,
			Summary:     program.Summary,
			StartUnix:   program.StartUnix,
			EndUnix:     program.EndUnix,
			ImageURL:    livetvLogoURL(program.ImageURL),
			Categories:  categories,
			livetvFlags: candidate.flags,
		}
		if slot := candidate.slot; slot != nil {
			item.Featured = true
			if slot.Name != "" && livetvGenericSportsTitle(item.Title, slot.LeagueName) {
				item.Title = slot.Name
			}
			if slot.ImageURL != "" {
				item.ImageURL = slot.ImageURL
			}
		}
		programs = append(programs, item)
	}
	return programs
}

// livetvSportsSlot is the part of a prepared sports event the programs route
// needs, keyed by the channel that carries it.
type livetvSportsSlot struct {
	StartUnix  int64
	EndUnix    int64
	Name       string
	LeagueName string
	// ImageURL is the event's GameThumbs background when it is an absolute
	// https URL; the payload's other images are plugin-relative proxy paths.
	ImageURL string
	Live     bool
}

// livetvSportsSlots builds a channel ID to sports event lookup from the
// prepared sports payload once per request. It never builds sports itself: it
// only schedules the usual background rebuild and reads what is ready, without
// cloning the payload. Before the first build finishes it returns nil and
// programs fall back to category classification.
func (s *HTTPRoutesServer) livetvSportsSlots() map[string][]livetvSportsSlot {
	if !s.sportsFeatureEnabled() {
		return nil
	}
	now := time.Now()
	guideUpdatedUnix := s.store.Current().Health.EPGLastSuccessUnix
	s.sportsPreparedMu.Lock()
	defer s.sportsPreparedMu.Unlock()
	s.scheduleSportsRebuildLocked(now, guideUpdatedUnix, false)
	if !s.sportsPrepared.Ready {
		return nil
	}
	return livetvSportsSlotsFromEvents(s.sportsPrepared.Payload.Events)
}

func livetvSportsSlotsFromEvents(events []SportsEvent) map[string][]livetvSportsSlot {
	slots := map[string][]livetvSportsSlot{}
	for _, event := range events {
		if event.StartUnix <= 0 || len(event.Channels) == 0 {
			continue
		}
		end := event.EndUnix
		if end <= event.StartUnix {
			end = event.StartUnix + livetvSportsDefaultDuration
		}
		slot := livetvSportsSlot{
			StartUnix:  event.StartUnix,
			EndUnix:    end,
			Name:       strings.TrimSpace(event.Name),
			LeagueName: event.LeagueName,
			ImageURL:   livetvHTTPSURL(event.GameThumbsBackgroundURL),
			Live:       event.Live,
		}
		for _, channel := range event.Channels {
			if channel.ID != "" {
				slots[channel.ID] = append(slots[channel.ID], slot)
			}
		}
	}
	return slots
}

// livetvMatchSportsSlot returns the event overlapping program the most, or nil.
func livetvMatchSportsSlot(slots []livetvSportsSlot, program model.Program) *livetvSportsSlot {
	if len(slots) == 0 {
		return nil
	}
	end := program.EndUnix
	if end <= program.StartUnix {
		end = program.StartUnix + 1
	}
	var best *livetvSportsSlot
	var bestOverlap int64
	for i := range slots {
		overlap := min(end, slots[i].EndUnix) - max(program.StartUnix, slots[i].StartUnix)
		if overlap > bestOverlap {
			best, bestOverlap = &slots[i], overlap
		}
	}
	return best
}

func livetvHTTPSURL(raw string) string {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Scheme, "https") {
		return ""
	}
	return value
}

// livetvGenericSportsTitles are guide titles that name a sport or league
// rather than the game, so a matched sports event's name is more useful.
var livetvGenericSportsTitles = map[string]bool{
	"sports": true, "sport": true, "game": true, "live": true, "live sports": true,
	"sports event": true, "tba": true, "tbd": true, "to be announced": true,
	"college football": true, "college basketball": true, "nfl": true, "nfl football": true,
	"nba": true, "nba basketball": true, "wnba": true, "wnba basketball": true,
	"mlb": true, "mlb baseball": true, "nhl": true, "nhl hockey": true,
	"mls": true, "mls soccer": true, "premier league": true, "uefa champions league": true,
	"champions league": true, "ufc": true, "mma": true, "boxing": true, "golf": true,
	"pga": true, "tennis": true, "nascar": true, "nascar cup series": true,
	"formula 1": true, "f1": true, "cricket": true, "soccer": true, "football": true,
	"basketball": true, "baseball": true, "hockey": true,
}

// livetvGenericSportsTitle reports whether a program title is a placeholder
// (empty, a bare sport or league label, or the event's league name) rather
// than a real title such as "Lakers at Celtics" or "SportsCenter".
func livetvGenericSportsTitle(title, leagueName string) bool {
	normalized := strings.Join(livetvWords(title), " ")
	if normalized == "" {
		return true
	}
	for _, prefix := range []string{"live ", "en vivo ", "en direct "} {
		normalized = strings.TrimPrefix(normalized, prefix)
	}
	if livetvGenericSportsTitles[normalized] {
		return true
	}
	if league := strings.Join(livetvWords(leagueName), " "); league != "" && normalized == league {
		return true
	}
	return false
}

// livetvClassifier derives livetvFlags. It memoizes per category label and
// per channel so a request over a large guide classifies each distinct label
// once.
type livetvClassifier struct {
	labels   map[string]livetvLabelSignal
	channels map[string]livetvFlags
}

type livetvLabelSignal struct {
	flags  livetvFlags
	series bool
}

func newLivetvClassifier() *livetvClassifier {
	return &livetvClassifier{labels: map[string]livetvLabelSignal{}, channels: map[string]livetvFlags{}}
}

// livetvProgramFlags classifies one program. The rules:
//
//   - Categories (case-insensitive) decide first: movie from movie/film/feature
//     film; sports from sports event/sports/sport or any label the guide's
//     league matcher recognises (except "Sports non-event", which is sports
//     talk and analysis); news from news/newsmagazine; kids from
//     children/kids. "Family" alone does not mean kids, since guides apply it
//     to general-audience sitcoms and films. isLive comes from a label with
//     the word "live".
//   - Only when no category gives movie, sports, news, kids or series does the
//     channel's group and name decide, using the web app's words (sports,
//     then news, then kids, then movie; the first match wins).
//   - sportsMatch (the program overlaps a prepared sports event on its
//     channel) forces isSports.
//   - isSeries comes from series/episode/sitcom/drama/reality/talk/game
//     show/comedy/soap/show/documentary/animated, but never on a movie or a
//     sports program.
//
// livetvPlaceholderTitles are whole titles that say a guide has no data,
// matching Dispatcharr for Silo's guide import.
var livetvPlaceholderTitles = map[string]bool{
	"tba": true, "to be announced": true, "sendepause": true, "sign off": true, "off air": true,
	"no games today": true, "no game today": true, "data not available": true, "no data available": true,
	"no guide data available": true, "no information available": true, "no programming available": true,
	"programming unavailable": true, "information not available": true,
}

// livetvFillerTitle reports guide filler: titles that say there is no data,
// plus the "no event" and "off air" slates sports and event channels fill
// their schedules with.
func livetvFillerTitle(title string) bool {
	words := livetvWords(title)
	if len(words) == 0 || livetvPlaceholderTitles[strings.Join(words, " ")] {
		return true
	}
	normalized := " " + strings.Join(words, " ") + " "
	for _, phrase := range []string{" no event ", " no events ", " no live event ", " no live events ", " no game ", " no games ", " off air ", " event not started ", " stream offline "} {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

func livetvProgramFlags(program model.Program, channel model.Channel, sportsMatch bool) livetvFlags {
	return newLivetvClassifier().flags(program, channel, sportsMatch)
}

func (c *livetvClassifier) flags(program model.Program, channel model.Channel, sportsMatch bool) livetvFlags {
	var flags livetvFlags
	series := false
	for _, label := range program.Categories {
		signal, ok := c.labels[label]
		if !ok {
			signal = livetvCategorySignal(label)
			c.labels[label] = signal
		}
		flags.IsMovie = flags.IsMovie || signal.flags.IsMovie
		flags.IsSports = flags.IsSports || signal.flags.IsSports
		flags.IsNews = flags.IsNews || signal.flags.IsNews
		flags.IsKids = flags.IsKids || signal.flags.IsKids
		flags.IsLive = flags.IsLive || signal.flags.IsLive
		series = series || signal.series
	}
	if !flags.classified() && !series {
		fallback, ok := c.channels[channel.ID]
		if !ok {
			fallback = livetvChannelSignal(channel)
			c.channels[channel.ID] = fallback
		}
		flags.IsMovie = fallback.IsMovie
		flags.IsSports = fallback.IsSports
		flags.IsNews = fallback.IsNews
		flags.IsKids = fallback.IsKids
	}
	if sportsMatch {
		flags.IsSports = true
	}
	flags.IsSeries = series && !flags.IsMovie && !flags.IsSports
	return flags
}

var (
	livetvMovieLabels  = map[string]bool{"movie": true, "movies": true, "film": true, "films": true, "feature film": true}
	livetvSportsLabels = map[string]bool{"sports event": true, "sports": true, "sport": true}
	livetvNewsLabels   = map[string]bool{"news": true, "newsmagazine": true}
	livetvKidsLabels   = map[string]bool{"children": true, "childrens": true, "children s": true, "kids": true}
	livetvSeriesLabels = map[string]bool{
		"series": true, "episode": true, "sitcom": true, "drama": true, "reality": true, "talk": true,
		"game show": true, "comedy": true, "soap": true, "show": true, "documentary": true, "animated": true,
	}
)

func livetvCategorySignal(label string) livetvLabelSignal {
	words := livetvWords(label)
	normalized := strings.Join(words, " ")
	var signal livetvLabelSignal
	if normalized == "" {
		return signal
	}
	signal.flags.IsMovie = livetvMovieLabels[normalized]
	signal.flags.IsNews = livetvNewsLabels[normalized]
	signal.flags.IsKids = livetvKidsLabels[normalized]
	signal.series = livetvSeriesLabels[normalized]
	signal.flags.IsSports = livetvSportsLabels[normalized] ||
		(normalized != "sports non event" && !signal.flags.IsMovie && !signal.flags.IsNews && !signal.flags.IsKids && !signal.series && guideSportsLeagueMatch(label).OK)
	for _, word := range words {
		if word == "live" {
			signal.flags.IsLive = true
		}
	}
	return signal
}

var (
	livetvChannelSportsWords = map[string]bool{
		// The web app's list also has "team" and "teams"; they are left out
		// here because 24/7 channels named after shows ("Team Umizoomi")
		// would land in Sports.
		"sport": true, "sports": true, "league": true, "football": true,
		"soccer": true, "futbol": true, "fútbol": true, "mlb": true, "nba": true, "nfl": true,
		"nhl": true, "mls": true, "f1": true,
	}
	livetvChannelNewsWords  = map[string]bool{"news": true, "noticias": true, "nouvelles": true}
	livetvChannelKidsWords  = map[string]bool{"kids": true, "children": true, "cartoon": true, "junior": true}
	livetvChannelMovieWords = map[string]bool{"movie": true, "movies": true, "cinema": true, "film": true, "films": true}
)

// livetvChannelSignal ports the web app's inferredInternationalRoot word
// lists (internal/plugin/ui/app.js) to classify by channel group and name.
func livetvChannelSignal(channel model.Channel) livetvFlags {
	words := livetvWords(channel.CategoryName + " " + channel.Name)
	for _, rule := range []struct {
		words map[string]bool
		flags livetvFlags
	}{
		{livetvChannelSportsWords, livetvFlags{IsSports: true}},
		{livetvChannelNewsWords, livetvFlags{IsNews: true}},
		{livetvChannelKidsWords, livetvFlags{IsKids: true}},
		{livetvChannelMovieWords, livetvFlags{IsMovie: true}},
	} {
		for _, word := range words {
			if rule.words[word] {
				return rule.flags
			}
		}
	}
	return livetvFlags{}
}

// livetvWords lowercases value and splits it on anything but letters and
// digits, so "Sports-Event", "sports event" and "SPORTS  EVENT" agree.
func livetvWords(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
