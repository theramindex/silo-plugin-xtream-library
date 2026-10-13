package plugin

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

type fakeGuideFillLoader struct {
	mu      sync.Mutex
	calls   [][]string
	release chan struct{}
}

func (f *fakeGuideFillLoader) load(ctx context.Context, settings config.Settings, channelIDs []string) error {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), channelIDs...))
	release := f.release
	f.mu.Unlock()
	if release != nil {
		<-release
	}
	return nil
}

func (f *fakeGuideFillLoader) takeCalls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := f.calls
	f.calls = nil
	return calls
}

func xcGuideFillServer(channels []model.Channel, programs []model.Program, settings config.Settings) (*HTTPRoutesServer, *fakeGuideFillLoader, *time.Time) {
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{Source: model.LiveTVSource(model.SourceModeXtream), Channels: channels, Programs: programs}})
	server := NewHTTPRoutesServerWithSettings(store, func() config.Settings { return settings })
	loader := &fakeGuideFillLoader{}
	clock := time.Unix(1_800_000_000, 0)
	server.livetvGuideFill.load = loader.load
	server.livetvGuideFill.now = func() time.Time { return clock }
	return server, loader, &clock
}

func xcFillSettings() config.Settings {
	return config.Settings{SourceMode: config.SourceModeXtream, XtreamBaseURL: "http://xtream.example", XtreamUsername: "u", XtreamPassword: "p"}
}

func xcChannels(count int) ([]model.Channel, []string) {
	channels := make([]model.Channel, 0, count)
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("xtream:%d", i+1)
		channels = append(channels, model.Channel{ID: id, Name: id})
		ids = append(ids, id)
	}
	return channels, ids
}

func joinIDs(ids []string) string {
	joined := ""
	for i, id := range ids {
		if i > 0 {
			joined += ","
		}
		joined += id
	}
	return joined
}

func TestLiveTVProgramsGuideFillIsBackgroundCappedAndCooledDown(t *testing.T) {
	t.Parallel()

	channels, ids := xcChannels(150)
	server, loader, clock := xcGuideFillServer(channels, nil, xcFillSettings())
	loader.release = make(chan struct{})
	query := livetvWindowQuery(1000, 2000, map[string]any{"channel_ids": joinIDs(ids)})

	// The response does not wait for the (blocked) load.
	got := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, query))
	if len(got) != 0 {
		t.Fatalf("expected the stored (empty) guide, got %d programs", len(got))
	}
	// A second request while the fill runs starts nothing.
	decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, query))
	close(loader.release)
	server.livetvGuideFill.wg.Wait()
	calls := loader.takeCalls()
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], ids[:livetvGuideFillMaxChannels]) {
		t.Fatalf("expected one fill of the first 100 channels, got %d calls: %v", len(calls), calls)
	}

	// The first 100 are cooling down, so the next request fills the rest.
	liveTVContractGet(t, server, livetvProgramsPath, query)
	server.livetvGuideFill.wg.Wait()
	if calls := loader.takeCalls(); len(calls) != 1 || !reflect.DeepEqual(calls[0], ids[100:]) {
		t.Fatalf("expected the remaining 50 channels, got %v", calls)
	}
	// Everything is cooling down now.
	liveTVContractGet(t, server, livetvProgramsPath, query)
	server.livetvGuideFill.wg.Wait()
	if calls := loader.takeCalls(); len(calls) != 0 {
		t.Fatalf("channels in cooldown must not be refetched, got %v", calls)
	}
	// After the cooldown the first 100 are due again.
	*clock = clock.Add(livetvGuideFillCooldown)
	liveTVContractGet(t, server, livetvProgramsPath, query)
	server.livetvGuideFill.wg.Wait()
	if calls := loader.takeCalls(); len(calls) != 1 || !reflect.DeepEqual(calls[0], ids[:livetvGuideFillMaxChannels]) {
		t.Fatalf("expected a refill after the cooldown, got %v", calls)
	}
}

func TestLiveTVProgramsGuideFillSkipsCoveredAndIneligibleChannels(t *testing.T) {
	t.Parallel()

	settings := xcFillSettings()
	settings.XtreamSources = []config.XtreamSource{
		{ID: "primary", BaseURL: "http://xtream.example", Username: "u", Password: "p", Enabled: true},
		{ID: "alt", BaseURL: "http://alt.example", Username: "u", Password: "p", Enabled: true, AlternateEPGEnabled: true},
	}
	channels := []model.Channel{
		{ID: "xtream:1", Name: "Covered"},
		{ID: "xtream:2", Name: "Empty"},
		{ID: "m3u:3", Name: "Not Xtream"},
		{ID: "xtream:alt:4", Name: "Alternate guide", SourceID: "xtream-source:alt"},
		{ID: "xtream:5", Name: "Only later programs"},
	}
	programs := []model.Program{
		{ID: "p1", ChannelID: "xtream:1", Title: "On now", StartUnix: 900, EndUnix: 1500},
		{ID: "p5", ChannelID: "xtream:5", Title: "Later", StartUnix: 5000, EndUnix: 6000},
	}
	server, loader, _ := xcGuideFillServer(channels, programs, settings)
	query := livetvWindowQuery(1000, 2000, map[string]any{"channel_ids": "xtream:1,xtream:2,m3u:3,xtream:alt:4,xtream:5,xtream:unknown"})
	got := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, query))
	if ids := livetvProgramIDs(got); !reflect.DeepEqual(ids, []string{"p1"}) {
		t.Fatalf("stored programs: %v", ids)
	}
	server.livetvGuideFill.wg.Wait()
	if calls := loader.takeCalls(); len(calls) != 1 || !reflect.DeepEqual(calls[0], []string{"xtream:2", "xtream:5"}) {
		t.Fatalf("expected only uncovered Xtream channels without an alternate guide, got %v", calls)
	}

	// Without channel_ids nothing is filled.
	liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(1000, 2000, nil))
	server.livetvGuideFill.wg.Wait()
	if calls := loader.takeCalls(); len(calls) != 0 {
		t.Fatalf("the whole-lineup query must not start a fill, got %v", calls)
	}
}

func TestLiveTVProgramsGuideFillNeedsXtreamSourceMode(t *testing.T) {
	t.Parallel()

	channels, ids := xcChannels(3)
	server, loader, _ := xcGuideFillServer(channels, nil, config.Settings{SourceMode: config.SourceModeM3UXMLTV, M3UURL: "http://m3u.example/list.m3u"})
	liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(1000, 2000, map[string]any{"channel_ids": joinIDs(ids)}))
	server.livetvGuideFill.wg.Wait()
	if calls := loader.takeCalls(); len(calls) != 0 {
		t.Fatalf("a non-Xtream source must not use the short-EPG fill, got %v", calls)
	}
}

func TestLiveTVProgramsDecodesXtreamGuideText(t *testing.T) {
	t.Parallel()

	server := liveTVProgramsServer([]model.Channel{{ID: "xtream:9", Name: "News"}}, []model.Program{
		{ID: "p9", ChannelID: "xtream:9", Title: "RXZlbmluZyBOZXdz", Summary: "SGVhZGxpbmVzIGFuZCB3ZWF0aGVyLg==", StartUnix: 1000, EndUnix: 2000},
	})
	got := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(1000, 2000, map[string]any{"channel_ids": "xtream:9"})))
	if len(got) != 1 || got[0].Title != "Evening News" || got[0].Summary != "Headlines and weather." {
		t.Fatalf("expected decoded guide text, got %+v", got)
	}
}
