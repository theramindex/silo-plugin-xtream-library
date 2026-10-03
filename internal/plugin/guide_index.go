package plugin

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

// guideIndex is a read-only view of one snapshot's guide, built once per
// snapshot version so /api/guide never copies or re-sorts the full program
// list per request. Slices handed out must not be mutated.
type guideIndex struct {
	version uint64
	// all holds every program ordered by start time, then ID.
	all []model.Program
	// allMaxDuration is the longest program in all, used to bound how far
	// before a window start an overlapping program can begin.
	allMaxDuration int64
	byChannel      map[string]channelGuide
}

type channelGuide struct {
	programs    []model.Program
	maxDuration int64
}

type guideIndexCache struct {
	mu    sync.Mutex
	index *guideIndex
}

// get returns the index for snapshot, rebuilding it only when the snapshot
// version changed.
func (c *guideIndexCache) get(snapshot cache.Snapshot) *guideIndex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index != nil && c.index.version == snapshot.Version {
		return c.index
	}
	c.index = buildGuideIndex(snapshot)
	return c.index
}

func buildGuideIndex(snapshot cache.Snapshot) *guideIndex {
	programs := make([]model.Program, len(snapshot.Catalog.Programs))
	for index, program := range snapshot.Catalog.Programs {
		programs[index] = decodeGuideProgramText(program)
	}
	sort.SliceStable(programs, func(i, j int) bool {
		if programs[i].StartUnix != programs[j].StartUnix {
			return programs[i].StartUnix < programs[j].StartUnix
		}
		return programs[i].ID < programs[j].ID
	})
	index := &guideIndex{
		version:   snapshot.Version,
		all:       programs,
		byChannel: map[string]channelGuide{},
	}
	for _, program := range programs {
		duration := programDuration(program)
		if duration > index.allMaxDuration {
			index.allMaxDuration = duration
		}
		guide := index.byChannel[program.ChannelID]
		guide.programs = append(guide.programs, program)
		if duration > guide.maxDuration {
			guide.maxDuration = duration
		}
		index.byChannel[program.ChannelID] = guide
	}
	return index
}

func programDuration(program model.Program) int64 {
	if program.EndUnix > program.StartUnix {
		return program.EndUnix - program.StartUnix
	}
	return 0
}

// guideWindow is a half-open [Start, End) range in unix seconds. Unbounded
// sides use math.MinInt64 / math.MaxInt64.
type guideWindow struct {
	Start int64
	End   int64
}

func (w guideWindow) bounded() bool {
	return w.Start != math.MinInt64 || w.End != math.MaxInt64
}

// overlaps reports whether program intersects the window. Programs without an
// end time are treated as an instant at their start.
func (w guideWindow) overlaps(program model.Program) bool {
	end := program.EndUnix
	if end <= program.StartUnix {
		return program.StartUnix >= w.Start && program.StartUnix < w.End
	}
	return program.StartUnix < w.End && end > w.Start
}

// query returns programs for channelIDs (all channels when empty) overlapping
// window, ordered by start time then ID. Unfiltered full-guide requests return
// the cached slice directly.
func (index *guideIndex) query(channelIDs []string, window guideWindow) []model.Program {
	if len(channelIDs) == 0 {
		if !window.bounded() {
			return index.all
		}
		return windowPrograms(index.all, index.allMaxDuration, window)
	}
	if len(channelIDs) == 1 {
		guide := index.byChannel[channelIDs[0]]
		if !window.bounded() {
			return nonNilPrograms(guide.programs)
		}
		return windowPrograms(guide.programs, guide.maxDuration, window)
	}
	result := make([]model.Program, 0)
	for _, channelID := range channelIDs {
		guide := index.byChannel[channelID]
		if window.bounded() {
			result = append(result, windowPrograms(guide.programs, guide.maxDuration, window)...)
		} else {
			result = append(result, guide.programs...)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].StartUnix != result[j].StartUnix {
			return result[i].StartUnix < result[j].StartUnix
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// windowPrograms binary-searches a start-sorted slice. Only programs starting
// in [window.Start-maxDuration, window.End) can overlap, so the scan is
// limited to that range.
func windowPrograms(programs []model.Program, maxDuration int64, window guideWindow) []model.Program {
	lowStart := window.Start
	if lowStart != math.MinInt64 {
		if lowStart-math.MinInt64 > maxDuration {
			lowStart -= maxDuration
		} else {
			lowStart = math.MinInt64
		}
	}
	low := sort.Search(len(programs), func(i int) bool { return programs[i].StartUnix >= lowStart })
	high := sort.Search(len(programs), func(i int) bool { return programs[i].StartUnix >= window.End })
	if low >= high {
		return []model.Program{}
	}
	result := make([]model.Program, 0, high-low)
	for _, program := range programs[low:high] {
		if window.overlaps(program) {
			result = append(result, program)
		}
	}
	return result
}

func nonNilPrograms(programs []model.Program) []model.Program {
	if programs == nil {
		return []model.Program{}
	}
	return programs
}

// guideQuery is the parsed /api/guide query contract:
//
//	channel_id  single channel ID (legacy)
//	channel_ids comma-separated channel IDs (merged with channel_id)
//	start, end  unix seconds; returns programs overlapping [start, end).
//	            Either may be omitted to leave that side open.
//
// With no parameters the full guide is returned.
type guideQuery struct {
	ChannelIDs []string
	Window     guideWindow
}

func parseGuideQuery(request *pluginv1.HandleHTTPRequest) (guideQuery, error) {
	query := guideQuery{Window: guideWindow{Start: math.MinInt64, End: math.MaxInt64}}
	seen := map[string]bool{}
	addChannel := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		query.ChannelIDs = append(query.ChannelIDs, value)
	}
	addChannel(queryValue(request, "channel_id"))
	for _, value := range strings.Split(queryValue(request, "channel_ids"), ",") {
		addChannel(value)
	}
	if len(query.ChannelIDs) > maxGuideQueryChannels {
		return guideQuery{}, fmt.Errorf("too many channel_ids (max %d)", maxGuideQueryChannels)
	}
	if raw := strings.TrimSpace(queryValue(request, "start")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return guideQuery{}, fmt.Errorf("start must be unix seconds")
		}
		query.Window.Start = value
	}
	if raw := strings.TrimSpace(queryValue(request, "end")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return guideQuery{}, fmt.Errorf("end must be unix seconds")
		}
		query.Window.End = value
	}
	if query.Window.Start >= query.Window.End {
		return guideQuery{}, fmt.Errorf("start must be before end")
	}
	return query, nil
}

const maxGuideQueryChannels = 2000
