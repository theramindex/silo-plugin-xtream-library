package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/cache"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
)

type EventsPayload struct {
	UpdatedAtUnix int64            `json:"updatedAtUnix"`
	Source        string           `json:"source"`
	Categories    []EventCategory  `json:"categories"`
	Events        []BroadcastEvent `json:"events"`
}

type EventCategory struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	LiveCount     int    `json:"liveCount"`
	UpcomingCount int    `json:"upcomingCount"`
}

type BroadcastEvent struct {
	ID           string                 `json:"id"`
	CategoryID   string                 `json:"categoryId"`
	CategoryName string                 `json:"categoryName"`
	Name         string                 `json:"name"`
	ShortName    string                 `json:"shortName,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Keyword      string                 `json:"keyword,omitempty"`
	StartUnix    int64                  `json:"startUnix"`
	EndUnix      int64                  `json:"endUnix,omitempty"`
	Live         bool                   `json:"live"`
	Completed    bool                   `json:"completed"`
	Channels     []SportsChannelMatch   `json:"channels"`
	EventSeries  bool                   `json:"eventSeries,omitempty"`
	Windows      []EventBroadcastWindow `json:"windows,omitempty"`
}

type EventBroadcastWindow struct {
	StartUnix int64                `json:"startUnix"`
	EndUnix   int64                `json:"endUnix,omitempty"`
	Channels  []SportsChannelMatch `json:"channels"`
}

type EventKeywordRule struct {
	CategoryID         string   `json:"categoryId"`
	CategoryName       string   `json:"categoryName"`
	Keywords           []string `json:"keywords"`
	ExcludeKeywords    []string `json:"excludeKeywords,omitempty"`
	EventSeries        bool     `json:"eventSeries,omitempty"`
	GroupWindowMinutes int      `json:"groupWindowMinutes,omitempty"`
}

func (s *HTTPRoutesServer) handleEvents(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if request.GetMethod() != "" && request.GetMethod() != http.MethodGet {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed"), nil
	}
	return s.respondJSON(http.StatusOK, s.eventsPayload(time.Now()))
}

func (s *HTTPRoutesServer) eventsPayload(now time.Time) EventsPayload {
	snapshot := s.store.Current()
	rules := s.eventKeywordRules()
	events := detectGuideBroadcastEvents(snapshot, now, rules)
	sort.Slice(events, func(i, j int) bool {
		if events[i].Live != events[j].Live {
			return events[i].Live
		}
		leftStart := broadcastEventSortStartUnix(events[i])
		rightStart := broadcastEventSortStartUnix(events[j])
		if leftStart != rightStart {
			return leftStart < rightStart
		}
		return events[i].Name < events[j].Name
	})
	return EventsPayload{
		UpdatedAtUnix: now.Unix(),
		Source:        "epg",
		Categories:    eventCategories(events),
		Events:        events,
	}
}

func (s *HTTPRoutesServer) eventKeywordRules() []EventKeywordRule {
	if s.store != nil && s.store.HasAdminSettings() {
		return eventKeywordRulesFromAdminSettings(s.store.AdminSettings())
	}
	if s.settingsProvider != nil {
		settings := s.settingsProvider()
		if len(settings.AdminSettings) > 0 {
			return eventKeywordRulesFromAdminSettings(settings.AdminSettings)
		}
	}
	return defaultEventKeywordRules()
}

func eventKeywordRulesFromAdminSettings(raw json.RawMessage) []EventKeywordRule {
	var payload map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return defaultEventKeywordRules()
	}
	rules := normalizeEventKeywordRules(payload["eventKeywords"])
	if len(rules) == 0 {
		return defaultEventKeywordRules()
	}
	return rules
}

func defaultEventKeywordRules() []EventKeywordRule {
	return []EventKeywordRule{
		{CategoryID: "awards", CategoryName: "Awards", Keywords: []string{"Academy Awards", "The Oscars", "Oscars", "Tony Awards", "The Tonys", "Golden Globes", "Grammy Awards", "Grammys", "Emmy Awards", "Emmys", "CMA Awards", "ACM Awards", "Billboard Music Awards", "American Music Awards", "BET Awards", "MTV Video Music Awards", "Critics Choice Awards", "SAG Awards"}},
		{CategoryID: "civic", CategoryName: "Civic", Keywords: []string{"State of the Union", "Presidential Address", "Joint Session", "Inauguration", "Election Night", "Presidential Debate"}},
		{CategoryID: "parades", CategoryName: "Parades", Keywords: []string{"Thanksgiving Day Parade", "Macy's Thanksgiving Day Parade", "Rose Parade", "Christmas Parade"}},
		{CategoryID: "entertainment", CategoryName: "Entertainment", Keywords: []string{"Live Special", "Special Presentation", "Red Carpet", "Ceremony", "Tribute Concert", "Benefit Concert", "Festival"}},
		{CategoryID: "golf", CategoryName: "Golf", Keywords: []string{"PGA Tour", "LPGA Tour", "DP World Tour", "The Masters", "U.S. Open Golf", "The Open Championship", "Ryder Cup"}, ExcludeKeywords: []string{"Golf Central", "highlights", "replay", "preview", "recap", "best of"}, EventSeries: true, GroupWindowMinutes: 60},
		{CategoryID: "motor-racing", CategoryName: "Motor Racing", Keywords: []string{"Formula 1", "F1 Grand Prix", "Grand Prix"}, ExcludeKeywords: []string{"highlights", "replay", "practice recap", "post race", "pre race"}, EventSeries: true, GroupWindowMinutes: 60},
		{CategoryID: "combat-sports", CategoryName: "Combat Sports", Keywords: []string{"UFC", "Ultimate Fighting Championship", "MMA"}, ExcludeKeywords: []string{"highlights", "replay", "countdown", "weigh-in", "preview", "recap"}, EventSeries: true, GroupWindowMinutes: 60},
		{CategoryID: "tennis", CategoryName: "Tennis", Keywords: []string{"ATP Tour", "WTA Tour", "Wimbledon", "US Open Tennis", "French Open Tennis", "Australian Open Tennis"}, ExcludeKeywords: []string{"highlights", "replay", "preview", "recap", "best of"}, EventSeries: true, GroupWindowMinutes: 60},
	}
}

func normalizeEventKeywordRules(value any) []EventKeywordRule {
	rows, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]EventKeywordRule); ok {
			return normalizeTypedEventKeywordRules(typed)
		}
		return []EventKeywordRule{}
	}
	rules := make([]EventKeywordRule, 0, len(rows))
	for _, row := range rows {
		object, ok := row.(map[string]any)
		if !ok {
			continue
		}
		categoryID, _ := object["categoryId"].(string)
		categoryName, _ := object["categoryName"].(string)
		keywords := normalizeKeywordValues(object["keywords"])
		excludeKeywords := normalizeKeywordValues(object["excludeKeywords"])
		eventSeries, _ := object["eventSeries"].(bool)
		groupWindowMinutes := normalizeEventGroupWindowMinutes(object["groupWindowMinutes"], eventSeries)
		rule := EventKeywordRule{
			CategoryID:         normalizeEventCategoryID(categoryID, categoryName),
			CategoryName:       strings.TrimSpace(categoryName),
			Keywords:           keywords,
			ExcludeKeywords:    excludeKeywords,
			EventSeries:        eventSeries,
			GroupWindowMinutes: groupWindowMinutes,
		}
		if rule.CategoryName == "" {
			rule.CategoryName = eventCategoryName(rule.CategoryID)
		}
		if rule.CategoryID != "" && len(rule.Keywords) > 0 {
			rules = append(rules, rule)
		}
	}
	return rules
}

func normalizeTypedEventKeywordRules(values []EventKeywordRule) []EventKeywordRule {
	normalized := make([]EventKeywordRule, 0, len(values))
	for _, rule := range values {
		rule.CategoryID = normalizeEventCategoryID(rule.CategoryID, rule.CategoryName)
		rule.CategoryName = strings.TrimSpace(rule.CategoryName)
		if rule.CategoryName == "" {
			rule.CategoryName = eventCategoryName(rule.CategoryID)
		}
		rule.Keywords = normalizeKeywordStrings(rule.Keywords)
		rule.ExcludeKeywords = normalizeKeywordStrings(rule.ExcludeKeywords)
		rule.GroupWindowMinutes = normalizeEventGroupWindowMinutes(rule.GroupWindowMinutes, rule.EventSeries)
		if rule.CategoryID != "" && len(rule.Keywords) > 0 {
			normalized = append(normalized, rule)
		}
	}
	return normalized
}

func normalizeEventGroupWindowMinutes(value any, eventSeries bool) int {
	minutes := 0
	switch typed := value.(type) {
	case int:
		minutes = typed
	case int64:
		minutes = int(typed)
	case float64:
		minutes = int(typed)
	}
	if !eventSeries {
		return 0
	}
	if minutes == 0 {
		return 60
	}
	if minutes < 15 {
		return 15
	}
	if minutes > 360 {
		return 360
	}
	return minutes
}

func normalizeKeywordValues(value any) []string {
	if values, ok := value.([]string); ok {
		return normalizeKeywordStrings(values)
	}
	rows, ok := value.([]any)
	if !ok {
		if text, ok := value.(string); ok {
			return normalizeKeywordText(text)
		}
		return []string{}
	}
	keywords := make([]string, 0, len(rows))
	for _, row := range rows {
		if text, ok := row.(string); ok {
			keywords = append(keywords, text)
		}
	}
	return normalizeKeywordStrings(keywords)
}

func normalizeKeywordText(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})
	return normalizeKeywordStrings(parts)
}

func normalizeKeywordStrings(values []string) []string {
	seen := map[string]bool{}
	keywords := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := normalizeMatchText(value)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keywords = append(keywords, value)
	}
	return keywords
}

func normalizeEventCategoryID(categoryID, categoryName string) string {
	value := normalizeMatchText(firstNonEmpty(categoryID, categoryName))
	switch value {
	case "awards", "award":
		return "awards"
	case "civic", "politics", "political":
		return "civic"
	case "parades", "parade":
		return "parades"
	case "entertainment", "specials", "special":
		return "entertainment"
	default:
		return strings.ReplaceAll(value, " ", "-")
	}
}

func eventCategoryName(categoryID string) string {
	switch categoryID {
	case "awards":
		return "Awards"
	case "civic":
		return "Civic"
	case "parades":
		return "Parades"
	case "entertainment":
		return "Entertainment"
	default:
		return strings.TrimSpace(categoryID)
	}
}

func detectGuideBroadcastEvents(snapshot cache.Snapshot, now time.Time, rules []EventKeywordRule) []BroadcastEvent {
	nowUnix := now.Unix()
	maxUnix := now.AddDate(0, 0, 60).Unix()
	categoryNames := map[string]string{}
	for _, category := range liveCategories(snapshot) {
		categoryNames[category.ID] = category.Name
	}
	byKey := map[string]*BroadcastEvent{}
	programs := append([]model.Program(nil), snapshot.Catalog.Programs...)
	sort.SliceStable(programs, func(i, j int) bool {
		if programs[i].StartUnix != programs[j].StartUnix {
			return programs[i].StartUnix < programs[j].StartUnix
		}
		return programs[i].ID < programs[j].ID
	})
	for _, program := range programs {
		if program.EndUnix > 0 && program.EndUnix < nowUnix-6*3600 {
			continue
		}
		if program.StartUnix > maxUnix {
			continue
		}
		rule, keyword, ok := matchEventKeyword(program, rules)
		if !ok {
			continue
		}
		channel, ok := channelByIDFromSnapshot(snapshot, program.ChannelID)
		if !ok {
			continue
		}
		key := broadcastProgramMergeKey(program, rule.CategoryID)
		event := byKey[key]
		if event == nil {
			event = &BroadcastEvent{
				ID:           "guide:" + firstNonEmpty(program.ID, shortHash(program.Title+"|"+fmt.Sprintf("%d", program.StartUnix))),
				CategoryID:   rule.CategoryID,
				CategoryName: rule.CategoryName,
				Name:         program.Title,
				ShortName:    program.Title,
				Description:  program.Summary,
				Keyword:      keyword,
				StartUnix:    program.StartUnix,
				EndUnix:      program.EndUnix,
				EventSeries:  rule.EventSeries,
			}
			byKey[key] = event
		}
		if event.Description == "" {
			event.Description = program.Summary
		}
		match := SportsChannelMatch{
			ID:           channel.ID,
			Name:         channel.Name,
			CategoryName: firstNonEmpty(categoryNames[channel.CategoryID], channel.CategoryName),
			LogoURL:      channel.LogoURL,
			Reason:       "guide: " + keyword,
			Score:        100,
		}
		event.Channels = appendEventChannelMatch(event.Channels, match)
		appendEventBroadcastWindow(event, program, match, rule)
	}
	events := make([]BroadcastEvent, 0, len(byKey))
	for _, event := range byKey {
		finalizeBroadcastEvent(event, now)
		events = append(events, *event)
	}
	return events
}

func matchEventKeyword(program model.Program, rules []EventKeywordRule) (EventKeywordRule, string, bool) {
	text := normalizeMatchText(strings.Join([]string{program.Title, program.Summary}, " "))
	for _, rule := range rules {
		for _, keyword := range rule.Keywords {
			normalizedKeyword := normalizeMatchText(keyword)
			if normalizedKeyword != "" && strings.Contains(" "+text+" ", " "+normalizedKeyword+" ") {
				if matchEventExclusion(text, rule.ExcludeKeywords) {
					break
				}
				return rule, keyword, true
			}
		}
	}
	return EventKeywordRule{}, "", false
}

func matchEventExclusion(normalizedText string, exclusions []string) bool {
	for _, exclusion := range exclusions {
		normalizedExclusion := normalizeMatchText(exclusion)
		if normalizedExclusion != "" && strings.Contains(" "+normalizedText+" ", " "+normalizedExclusion+" ") {
			return true
		}
	}
	return false
}

func appendEventBroadcastWindow(event *BroadcastEvent, program model.Program, match SportsChannelMatch, rule EventKeywordRule) {
	windowIndex := 0
	if rule.EventSeries {
		windowIndex = -1
		windowSeconds := int64(normalizeEventGroupWindowMinutes(rule.GroupWindowMinutes, true) * 60)
		for index := range event.Windows {
			if program.StartUnix >= event.Windows[index].StartUnix && program.StartUnix-event.Windows[index].StartUnix <= windowSeconds {
				windowIndex = index
				break
			}
		}
		if windowIndex < 0 {
			event.Windows = append(event.Windows, EventBroadcastWindow{StartUnix: program.StartUnix, EndUnix: program.EndUnix, Channels: []SportsChannelMatch{}})
			windowIndex = len(event.Windows) - 1
		}
	} else if len(event.Windows) == 0 {
		event.Windows = append(event.Windows, EventBroadcastWindow{StartUnix: program.StartUnix, EndUnix: program.EndUnix, Channels: []SportsChannelMatch{}})
	}
	window := &event.Windows[windowIndex]
	if window.StartUnix == 0 || (program.StartUnix > 0 && program.StartUnix < window.StartUnix) {
		window.StartUnix = program.StartUnix
	}
	if program.EndUnix > window.EndUnix {
		window.EndUnix = program.EndUnix
	}
	window.Channels = appendEventChannelMatch(window.Channels, match)
}

func finalizeBroadcastEvent(event *BroadcastEvent, now time.Time) {
	sort.Slice(event.Windows, func(i, j int) bool { return event.Windows[i].StartUnix < event.Windows[j].StartUnix })
	for index := range event.Windows {
		sortEventChannelMatches(event.Windows[index].Channels)
	}
	sortEventChannelMatches(event.Channels)
	if len(event.Windows) > 0 {
		event.StartUnix = event.Windows[0].StartUnix
		event.EndUnix = event.Windows[len(event.Windows)-1].EndUnix
	}
	event.Live = broadcastEventIsLive(*event, now)
	event.Completed = broadcastEventIsCompleted(*event, now)
}

func sortEventChannelMatches(matches []SportsChannelMatch) {
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Name < matches[j].Name
	})
}

func appendEventChannelMatch(matches []SportsChannelMatch, match SportsChannelMatch) []SportsChannelMatch {
	for index, existing := range matches {
		if existing.ID == match.ID {
			if match.Score > existing.Score {
				matches[index] = match
			}
			return matches
		}
	}
	return append(matches, match)
}

func channelByIDFromSnapshot(snapshot cache.Snapshot, channelID string) (model.Channel, bool) {
	for _, channel := range snapshot.Catalog.Channels {
		if channel.ID == channelID {
			return channel, true
		}
	}
	return model.Channel{}, false
}

func broadcastProgramMergeKey(program model.Program, categoryID string) string {
	day := ""
	if program.StartUnix > 0 {
		day = time.Unix(program.StartUnix, 0).UTC().Format("2006-01-02")
	}
	return categoryID + "|" + normalizeMatchText(program.Title) + "|" + day
}

func broadcastEventSortStartUnix(event BroadcastEvent) int64 {
	if event.StartUnix > 0 {
		return event.StartUnix
	}
	return 1<<62 - 1
}

func shortHash(value string) string {
	sum := sportsHash(value)
	if len(sum) > 16 {
		return sum[:16]
	}
	return sum
}

func eventCategories(events []BroadcastEvent) []EventCategory {
	byID := map[string]*EventCategory{}
	for _, event := range events {
		id := firstNonEmpty(event.CategoryID, "events")
		category := byID[id]
		if category == nil {
			category = &EventCategory{ID: id, Name: firstNonEmpty(event.CategoryName, eventCategoryName(id))}
			byID[id] = category
		}
		if event.Live {
			category.LiveCount++
		} else if !event.Completed {
			category.UpcomingCount++
		}
	}
	categories := make([]EventCategory, 0, len(byID))
	for _, category := range byID {
		categories = append(categories, *category)
	}
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Name < categories[j].Name
	})
	return categories
}

func broadcastEventIsLive(event BroadcastEvent, now time.Time) bool {
	nowUnix := now.Unix()
	for _, window := range event.Windows {
		end := window.EndUnix
		if end == 0 {
			end = window.StartUnix + 3*3600
		}
		if window.StartUnix > 0 && window.StartUnix <= nowUnix && end >= nowUnix {
			return true
		}
	}
	if event.StartUnix == 0 {
		return false
	}
	end := event.EndUnix
	if end == 0 {
		end = event.StartUnix + 3*3600
	}
	return event.StartUnix <= nowUnix && end >= nowUnix
}

func broadcastEventIsCompleted(event BroadcastEvent, now time.Time) bool {
	if len(event.Windows) == 0 {
		return event.EndUnix > 0 && event.EndUnix < now.Unix()
	}
	for _, window := range event.Windows {
		end := window.EndUnix
		if end == 0 {
			end = window.StartUnix + 3*3600
		}
		if end >= now.Unix() {
			return false
		}
	}
	return true
}
