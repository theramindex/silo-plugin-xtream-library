package plugin

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

// Background guide fill for GET /silo-compat/livetv/v1/programs.
//
// XC's snapshot usually holds no programs: the web app loads the guide per
// channel from get_short_epg through POST /xtream/api/guide/ping. Silo's
// Jellyfin bridge never pings, so when the programs route is called with
// channel_ids it answers at once with whatever is stored and, without
// blocking the response, starts the same short-EPG load
// (RefreshCoordinator.RunGuideChannels) for the requested channels that have
// no program overlapping the requested window. The contract allows this: a
// provider that loads guide data on demand returns what it has.
//
//   - At most livetvGuideFillMaxChannels channels are taken per request, in
//     channel_ids order, after cooldown filtering.
//   - Each channel is tried at most once per livetvGuideFillCooldown, whether
//     the load succeeded or not, so repeated guide page loads don't hammer
//     the provider.
//   - Only one fill runs at a time; a request that arrives while one is
//     running starts nothing.
//   - The load runs in batches of guidePingChannelLimit, one after another,
//     each merged into the snapshot as it finishes, so the guide fills in
//     progressively on the next request.
//   - Only Xtream channels in Xtream source mode are filled, and channels of
//     sources with an alternate XMLTV guide are skipped: that guide is loaded
//     whole, and asking for one of its channels would refetch all of it.
const (
	livetvGuideFillMaxChannels   = 100
	livetvGuideFillCooldown      = 30 * time.Minute
	livetvGuideFillBatchDeadline = 30 * time.Second
)

type livetvGuideFill struct {
	mu        sync.Mutex
	running   bool
	attempted map[string]time.Time
	// wg tracks the running fill so tests can wait for it.
	wg sync.WaitGroup
	// now and load are test hooks; nil means time.Now and the short-EPG
	// load through the refresh coordinator.
	now  func() time.Time
	load func(ctx context.Context, settings config.Settings, channelIDs []string) error
}

// startLiveTVGuideFill starts a background fill for the query's channels and
// reports whether it did. It never blocks on the provider.
func (s *HTTPRoutesServer) startLiveTVGuideFill(index *guideIndex, query livetvProgramsQuery) bool {
	if len(query.ChannelIDs) == 0 || s.settingsProvider == nil {
		return false
	}
	fill := &s.livetvGuideFill
	load := fill.load
	if load == nil {
		if s.coordinator == nil {
			return false
		}
		load = s.loadLiveTVGuideChannels
	}
	settings := s.settingsProvider()
	if settings.EffectiveSourceMode() != config.SourceModeXtream {
		return false
	}

	fill.mu.Lock()
	defer fill.mu.Unlock()
	if fill.running {
		return false
	}
	now := time.Now()
	if fill.now != nil {
		now = fill.now()
	}
	channelIDs := make([]string, 0, livetvGuideFillMaxChannels)
	for _, channelID := range query.ChannelIDs {
		if len(channelIDs) >= livetvGuideFillMaxChannels {
			break
		}
		if last, ok := fill.attempted[channelID]; ok && now.Sub(last) < livetvGuideFillCooldown {
			continue
		}
		position, ok := index.lineupPosition[channelID]
		if !ok || !livetvGuideFillEligible(index.lineup[position], settings) {
			continue
		}
		guide := index.byChannel[channelID]
		if len(windowPrograms(guide.programs, guide.maxDuration, query.Window)) > 0 {
			continue
		}
		channelIDs = append(channelIDs, channelID)
	}
	if len(channelIDs) == 0 {
		return false
	}
	if fill.attempted == nil {
		fill.attempted = map[string]time.Time{}
	}
	for channelID, last := range fill.attempted {
		if now.Sub(last) >= livetvGuideFillCooldown {
			delete(fill.attempted, channelID)
		}
	}
	for _, channelID := range channelIDs {
		fill.attempted[channelID] = now
	}
	fill.running = true
	fill.wg.Add(1)
	go func() {
		defer fill.wg.Done()
		defer func() {
			fill.mu.Lock()
			fill.running = false
			fill.mu.Unlock()
		}()
		if err := load(context.Background(), settings, channelIDs); err != nil {
			log.Printf("xtream: livetv guide fill for %d channels: %v", len(channelIDs), err)
		}
	}()
	return true
}

func livetvGuideFillEligible(channel model.Channel, settings config.Settings) bool {
	if !strings.HasPrefix(channel.ID, "xtream:") {
		return false
	}
	sourceID := strings.TrimPrefix(strings.TrimSpace(channel.SourceID), "xtream-source:")
	if sourceID == "" {
		sourceID = "primary"
	}
	source, ok := settings.XtreamSourceByID(sourceID)
	return ok && !source.AlternateEPGEnabled
}

// loadLiveTVGuideChannels runs the guide ping's short-EPG load in batches of
// guidePingChannelLimit. A failed batch does not stop the rest.
func (s *HTTPRoutesServer) loadLiveTVGuideChannels(ctx context.Context, settings config.Settings, channelIDs []string) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	var firstErr error
	for start := 0; start < len(channelIDs); start += guidePingChannelLimit {
		batch := channelIDs[start:min(start+guidePingChannelLimit, len(channelIDs))]
		batchCtx, cancel := context.WithTimeout(ctx, livetvGuideFillBatchDeadline)
		err := s.coordinator.RunGuideChannels(batchCtx, settings, batch, time.Now().Unix())
		cancel()
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
