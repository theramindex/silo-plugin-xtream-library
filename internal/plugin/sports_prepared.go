package plugin

import (
	"context"
	"sort"
	"strings"
	"time"
)

func (s *HTTPRoutesServer) sportsPayload(ctx context.Context, refresh bool) SportsPayload {
	return s.sportsPayloadWithBudget(ctx, refresh, sportsBuildBudget{})
}

func (s *HTTPRoutesServer) sportsPayloadWithBudget(ctx context.Context, refresh bool, budget sportsBuildBudget) SportsPayload {
	budget = budget.withDefaults()
	now := time.Now()
	snapshot := s.store.Current()
	guideEvents, refreshScores := sportsEventsFromGuideWithScoreHints(snapshot, now)
	providerCtx, cancelProvider := context.WithTimeout(ctx, budget.Provider)
	events, updatedUnix, source, err := s.cachedSportsEvents(providerCtx, now, refresh || refreshScores)
	cancelProvider()
	if len(guideEvents) > 0 {
		if len(events) == 0 {
			events = guideEvents
			source = "EPG fallback"
			err = nil
		} else {
			events = mergeSportsGuideEvents(events, guideEvents)
			source = firstNonEmpty(source, "Sports provider") + " + EPG"
		}
		if updatedUnix <= 0 {
			updatedUnix = snapshot.Health.EPGLastSuccessUnix
			if updatedUnix <= 0 {
				updatedUnix = now.Unix()
			}
		}
	}
	// The match deadline starts after the provider fetch, independent of it.
	matchCtx, cancelMatch := context.WithTimeout(ctx, budget.Match)
	defer cancelMatch()
	channelIndex := newSportsChannelIndex(snapshot)
	incomplete := false
	for index := range events {
		events[index] = normalizeSportsEventFreshness(events[index], now)
		events[index].Home = finalizeSportsTeamIdentity(events[index].Home)
		events[index].Away = finalizeSportsTeamIdentity(events[index].Away)
		if len(events[index].Channels) > 0 {
			events[index].Channels = mergeSportsChannelMatches(events[index].Channels)
			events[index].MatchDiagnostics = make([]SportsMatchDiagnostic, 0, len(events[index].Channels))
			for _, match := range events[index].Channels {
				events[index].MatchDiagnostics = append(events[index].MatchDiagnostics, SportsMatchDiagnostic{
					ChannelID: match.ID, ChannelName: match.Name, Accepted: true,
					Evidence: "epg", Confidence: "high", Reason: firstNonEmpty(match.Reason, "EPG-derived event feed"), Score: match.Score,
				})
			}
			continue
		}
		if matchCtx.Err() != nil {
			incomplete = true
			continue
		}
		matches, diagnostics := channelIndex.MatchDetailedContext(matchCtx, events[index])
		if matchCtx.Err() != nil {
			// The channel scan stopped part way; this event's matches are partial.
			incomplete = true
		}
		events[index].Channels = mergeSportsChannelMatches(events[index].Channels, matches)
		events[index].MatchDiagnostics = diagnostics
	}
	cancelMatch()
	sort.Slice(events, func(i, j int) bool {
		if events[i].Live != events[j].Live {
			return events[i].Live
		}
		leftStart := sportsSortStartUnix(events[i])
		rightStart := sportsSortStartUnix(events[j])
		if leftStart != rightStart {
			return leftStart < rightStart
		}
		return events[i].Name < events[j].Name
	})
	events = filterPlayableSportsEvents(events)
	if enricher, ok := s.sportsProvider.(sportsEventEnricher); ok {
		enrichCtx, cancelEnrich := context.WithTimeout(ctx, budget.Enrich)
		events = enricher.EnrichEvents(enrichCtx, events, 8)
		cancelEnrich()
	}
	events = rankSportsEvents(events, now)
	events = s.proxySportsEventImages(events)
	payload := SportsPayload{
		UpdatedAtUnix: updatedUnix,
		Source:        source,
		Leagues:       sportsLeagues(events),
		Events:        events,
		FavoriteTeams: []string{},
		Incomplete:    incomplete,
	}
	if err != nil {
		payload.Error = err.Error()
	}
	return payload
}

func normalizeSportsEventFreshness(event SportsEvent, now time.Time) SportsEvent {
	if !event.Live {
		return event
	}
	stale := event.EndUnix > 0 && event.EndUnix < now.Add(-2*time.Hour).Unix()
	if !stale && event.StartUnix > 0 && event.StartUnix < now.Add(-18*time.Hour).Unix() {
		stale = true
	}
	if !stale {
		return event
	}
	event.Live = false
	event.Completed = true
	event.Status = "final"
	event.StatusText = "Final"
	return event
}

func sportsPlayableChannelMatch(match SportsChannelMatch) bool {
	switch strings.ToLower(strings.TrimSpace(match.Confidence)) {
	case "low":
		return false
	case "high", "medium":
		return true
	default:
		return match.Score >= sportsChannelMinimumScore
	}
}

func filterPlayableSportsEvents(events []SportsEvent) []SportsEvent {
	playable := make([]SportsEvent, 0, len(events))
	for _, event := range events {
		channels := make([]SportsChannelMatch, 0, len(event.Channels))
		for _, match := range event.Channels {
			if sportsPlayableChannelMatch(match) {
				channels = append(channels, match)
			}
		}
		if len(channels) == 0 {
			continue
		}
		event.Channels = channels
		playable = append(playable, event)
	}
	return playable
}

func (s *HTTPRoutesServer) preparedSportsPayload(refresh bool) SportsPayload {
	now := time.Now()
	snapshot := s.store.Current()
	s.sportsPreparedMu.Lock()
	s.scheduleSportsRebuildLocked(now, snapshot.Health.EPGLastSuccessUnix, refresh)
	payload := cloneSportsPayload(s.sportsPrepared.Payload)
	payload.Refreshing = s.sportsPrepared.Refreshing
	if !s.sportsPrepared.Ready && payload.Source == "" {
		provider := s.sportsProvider
		if provider == nil {
			provider = noopSportsProvider{}
		}
		payload.Source = provider.Source()
	}
	if !s.sportsPrepared.Ready && len(payload.Events) == 0 {
		if guideEvents := sportsEventsFromGuide(snapshot, now); len(guideEvents) > 0 {
			payload.Events = guideEvents
			payload.Leagues = sportsLeagues(guideEvents)
			payload.Source = "EPG fallback"
			payload.UpdatedAtUnix = snapshot.Health.EPGLastSuccessUnix
			if payload.UpdatedAtUnix <= 0 {
				payload.UpdatedAtUnix = now.Unix()
			}
		}
	}
	s.sportsPreparedMu.Unlock()
	return payload
}

// scheduleSportsRebuildLocked starts a background rebuild when the prepared
// payload is missing, stale or behind the guide. sportsPreparedMu must be held.
func (s *HTTPRoutesServer) scheduleSportsRebuildLocked(now time.Time, guideUpdatedUnix int64, refresh bool) {
	guideChanged := guideUpdatedUnix > 0 && guideUpdatedUnix != s.sportsPrepared.GuideUpdatedUnix
	needsRefresh := !s.sportsPrepared.Ready || refresh || guideChanged || now.After(s.sportsPrepared.ExpiresAfter)
	if needsRefresh && !s.sportsPrepared.Refreshing {
		s.sportsPrepared.Refreshing = true
		go s.rebuildSportsPayload(refresh)
	}
}

// preparedSportsEvent finds one event by ID or stable ID, copying only that
// event rather than the whole payload.
func (s *HTTPRoutesServer) preparedSportsEvent(id string) (SportsEvent, bool) {
	if id == "" {
		return SportsEvent{}, false
	}
	now := time.Now()
	guideUpdatedUnix := s.store.Current().Health.EPGLastSuccessUnix
	s.sportsPreparedMu.Lock()
	if s.sportsPrepared.Ready {
		s.scheduleSportsRebuildLocked(now, guideUpdatedUnix, false)
		for _, event := range s.sportsPrepared.Payload.Events {
			if event.ID == id || event.StableID == id {
				found := cloneSportsEvents([]SportsEvent{event})[0]
				s.sportsPreparedMu.Unlock()
				return found, true
			}
		}
		s.sportsPreparedMu.Unlock()
		return SportsEvent{}, false
	}
	s.sportsPreparedMu.Unlock()
	// Before the first build the guide fallback is assembled per request.
	for _, event := range s.preparedSportsPayload(false).Events {
		if event.ID == id || event.StableID == id {
			return event, true
		}
	}
	return SportsEvent{}, false
}

func (s *HTTPRoutesServer) rebuildSportsPayload(refresh bool) {
	guideUpdatedUnix := s.store.Current().Health.EPGLastSuccessUnix
	s.sportsPreparedMu.Lock()
	budget := s.sportsPrepared.budget.withDefaults()
	s.sportsPreparedMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), budget.total())
	payload := s.sportsPayloadWithBudget(ctx, refresh, budget)
	cancel()

	now := time.Now()
	s.sportsPreparedMu.Lock()
	previousComplete := s.sportsPrepared.Ready && !s.sportsPrepared.Incomplete && len(s.sportsPrepared.Payload.Events) > 0
	switch {
	case payload.Error != "" && s.sportsPrepared.Ready && len(s.sportsPrepared.Payload.Events) > 0:
		stale := cloneSportsPayload(s.sportsPrepared.Payload)
		stale.Error = payload.Error
		payload = stale
	case payload.Incomplete && previousComplete:
		// Keep serving the last complete payload rather than one with
		// unmatched channels, and retry soon. Recording the guide revision
		// makes the retry wait for the short deadline instead of restarting on
		// the next request.
		s.sportsPrepared.Refreshing = false
		s.sportsPrepared.GuideUpdatedUnix = guideUpdatedUnix
		s.sportsPrepared.ExpiresAfter = now.Add(sportsIncompleteRetry)
		s.sportsPreparedMu.Unlock()
		return
	}
	payload.Refreshing = false
	s.sportsPrepared.Payload = cloneSportsPayload(payload)
	s.sportsPrepared.Ready = true
	s.sportsPrepared.Refreshing = false
	s.sportsPrepared.Incomplete = payload.Incomplete
	s.sportsPrepared.GuideUpdatedUnix = guideUpdatedUnix
	s.sportsPrepared.ExpiresAfter = now.Add(sportsPreparedTTL(payload))
	s.sportsPreparedMu.Unlock()
}

func sportsPreparedTTL(payload SportsPayload) time.Duration {
	if payload.Incomplete {
		return sportsIncompleteRetry
	}
	if payload.Error != "" || len(payload.Events) == 0 {
		return 30 * time.Second
	}
	return sportsCacheTTL(payload.Events)
}

func cloneSportsPayload(payload SportsPayload) SportsPayload {
	clone := payload
	clone.Events = cloneSportsEvents(payload.Events)
	clone.Leagues = append([]SportsLeague(nil), payload.Leagues...)
	clone.FavoriteTeams = append([]string(nil), payload.FavoriteTeams...)
	return clone
}

func sportsSortStartUnix(event SportsEvent) int64 {
	if event.StartUnix > 0 {
		return event.StartUnix
	}
	return 1<<62 - 1
}

func (s *HTTPRoutesServer) cachedSportsEvents(ctx context.Context, now time.Time, refresh bool) ([]SportsEvent, int64, string, error) {
	s.sportsMu.Lock()
	defer s.sportsMu.Unlock()

	if !refresh && now.Before(s.sportsCache.ExpiresAfter) {
		return cloneSportsEvents(s.sportsCache.Events), s.sportsCache.UpdatedUnix, s.sportsCache.Source, nil
	}
	provider := s.sportsProvider
	if provider == nil {
		provider = noopSportsProvider{}
	}
	events, err := provider.Events(ctx, now)
	source := provider.Source()
	if err != nil {
		if len(s.sportsCache.Events) > 0 {
			return cloneSportsEvents(s.sportsCache.Events), s.sportsCache.UpdatedUnix, s.sportsCache.Source, err
		}
		return []SportsEvent{}, now.Unix(), source, err
	}
	for index := range events {
		events[index].ProviderSource = strings.TrimSpace(source)
	}
	events = normalizeSportsEvents(events)
	updatedUnix := now.Unix()
	s.sportsCache = sportsEventCache{
		Events:       cloneSportsEvents(events),
		UpdatedUnix:  updatedUnix,
		Source:       source,
		ExpiresAfter: now.Add(sportsCacheTTL(events)),
	}
	return cloneSportsEvents(events), updatedUnix, source, nil
}

func sportsCacheTTL(events []SportsEvent) time.Duration {
	for _, event := range events {
		if event.Live {
			return 30 * time.Second
		}
	}
	return 5 * time.Minute
}
