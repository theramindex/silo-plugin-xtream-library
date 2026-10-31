package plugin

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"github.com/theramindex/silo-plugins/pkg/sportsguide"
)

func (s *HTTPRoutesServer) sportsGuidePolicy() sportsguide.Policy {
	policy := sportsguide.DecodePolicy(s.adminSettingsPayload()["sportsGuide"])
	if !s.sportsFeatureEnabled() {
		policy.Enabled = false
	}
	return policy
}
func generatedGuideScope(snapshot cache.Snapshot, policy sportsguide.Policy) string {
	profile := ""
	if snapshot.Catalog.Source.ChannelProfile != nil {
		profile = snapshot.Catalog.Source.ChannelProfile.ID
	}
	sum := sha256.Sum256([]byte(snapshot.ConfigKey + "|" + snapshot.Catalog.Source.ID + "|" + string(snapshot.Catalog.Source.Mode) + "|" + profile + "|" + sportsguide.Fingerprint(policy)))
	return fmt.Sprintf("%x", sum)
}
func guideChannels(snapshot cache.Snapshot, ids []string) []sportsguide.Channel {
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	out := []sportsguide.Channel{}
	for _, ch := range snapshot.Catalog.Channels {
		if len(ids) > 0 && !selected[ch.ID] {
			continue
		}
		out = append(out, sportsguide.Channel{ID: ch.ID, Name: ch.Name, GroupID: ch.CategoryID, GroupName: ch.CategoryName})
	}
	return out
}
func independentGuideSchedule(events []SportsEvent, source string, fetchErr error) []sportsguide.Event {
	if fetchErr != nil {
		return nil
	}
	schedule := []sportsguide.Event{}
	for _, event := range events {
		if strings.HasPrefix(event.ID, "epg:") {
			continue
		}
		provider := event.ProviderSource
		if provider == "" {
			provider = source
		}
		schedule = append(schedule, sportsguide.Event{ID: event.ID, Provider: provider, LeagueID: event.LeagueID, LeagueName: event.LeagueName, Name: event.Name, Home: event.Home.Name, Away: event.Away.Name, HomeAbbr: event.Home.Abbreviation, AwayAbbr: event.Away.Abbreviation, Start: event.StartUnix, End: event.EndUnix, ImageURL: event.ImageURL, Description: event.Description, Status: event.Status})
	}
	return schedule
}
func (s *HTTPRoutesServer) persistSportsGuide(ctx context.Context, snapshot cache.Snapshot, now time.Time, schedule []sportsguide.Event) error {
	policy := s.sportsGuidePolicy()
	scope := generatedGuideScope(snapshot, policy)
	if !policy.Enabled || scope != generatedGuideScope(s.store.Current(), s.sportsGuidePolicy()) {
		return nil
	}
	listings := sportsguide.Generate(now, guideChannels(snapshot, nil), schedule, policy)
	overlay, err := s.store.GuideOverlay()
	if err != nil {
		return err
	}
	return overlay.Replace(ctx, scope, listings)
}
func (s *HTTPRoutesServer) withSportsGuide(ctx context.Context, snapshot cache.Snapshot, index *guideIndex, ids []string, window guideWindow) (*guideIndex, error) {
	policy := s.sportsGuidePolicy()
	if !policy.Enabled {
		return index, nil
	}
	// All network and generation work stays in the existing background builder.
	hasCandidates := len(policy.Groups) > 0 || len(policy.Overrides) > 0
	for _, ch := range snapshot.Catalog.Channels {
		name := strings.ToLower(ch.Name)
		if strings.Contains(name, "@") || strings.Contains(name, " vs ") || strings.Contains(name, " at ") {
			hasCandidates = true
			break
		}
	}
	if hasCandidates {
		s.preparedSportsPayload(false)
	}
	overlay, err := s.store.GuideOverlay()
	if err != nil {
		return nil, err
	}
	generated, err := overlay.Query(ctx, generatedGuideScope(snapshot, policy), guideChannels(snapshot, ids), window.Start, window.End, time.Now())
	if err != nil {
		return nil, err
	}
	if len(generated) == 0 {
		return index, nil
	}
	programs := append([]model.Program(nil), index.query(ids, window)...)
	// Query provider coverage over whole generated blocks, so their IDs and
	// clipped intervals remain identical across adjacent guide-window requests.
	low, high := generated[0].Start, generated[0].End
	generatedIDs := []string{}
	seen := map[string]bool{}
	for _, p := range generated {
		if p.Start < low {
			low = p.Start
		}
		if p.End > high {
			high = p.End
		}
		if !seen[p.ChannelID] {
			seen[p.ChannelID] = true
			generatedIDs = append(generatedIDs, p.ChannelID)
		}
	}
	coverage, err := snapshot.ProgramQuery(ctx, generatedIDs, low, high)
	if err != nil {
		return nil, err
	}
	spans := make([]sportsguide.Span, 0, len(coverage))
	for _, p := range coverage {
		spans = append(spans, sportsguide.Span{ChannelID: p.ChannelID, Start: p.StartUnix, End: p.EndUnix})
	}
	for _, p := range sportsguide.FillGaps(generated, spans) {
		provenance := p.Generated
		programs = append(programs, model.Program{ID: p.ID, ChannelID: p.ChannelID, Title: p.Title, Summary: p.Summary, ImageURL: p.ImageURL, Categories: p.Categories, StartUnix: p.Start, EndUnix: p.End, Generated: &provenance})
	}
	snapshot.Catalog.Programs = programs
	snapshot.Guide = nil
	return buildGuideIndex(snapshot), nil
}
