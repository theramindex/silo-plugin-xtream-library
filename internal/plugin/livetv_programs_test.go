package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

const livetvProgramsPath = "/silo-compat/livetv/v1/programs"

func liveTVProgramsServer(channels []model.Channel, programs []model.Program) *HTTPRoutesServer {
	store := cache.NewStore()
	store.Replace(cache.Snapshot{
		Catalog: model.CatalogState{Source: model.LiveTVSource(model.SourceModeM3UXMLTV), Channels: channels, Programs: programs},
	})
	return NewHTTPRoutesServerWithSettings(store, func() config.Settings { return config.Settings{} })
}

func decodeLiveTVPrograms(t *testing.T, response *pluginv1.HandleHTTPResponse) []livetvProgram {
	t.Helper()
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.GetStatusCode(), response.GetBody())
	}
	decoder := json.NewDecoder(bytes.NewReader(response.GetBody()))
	decoder.DisallowUnknownFields()
	var payload livetvPrograms
	if err := decoder.Decode(&payload); err != nil {
		t.Fatalf("decode programs %s: %v", response.GetBody(), err)
	}
	if payload.Programs == nil {
		t.Fatalf("programs must be an array, got %s", response.GetBody())
	}
	return payload.Programs
}

func livetvProgramIDs(programs []livetvProgram) []string {
	ids := make([]string, 0, len(programs))
	for _, program := range programs {
		ids = append(ids, program.ID)
	}
	return ids
}

func livetvWindowQuery(start, end int64, extra map[string]any) map[string]any {
	query := map[string]any{"start": fmt.Sprint(start), "end": fmt.Sprint(end)}
	for key, value := range extra {
		query[key] = value
	}
	return query
}

func livetvTestLineup() ([]model.Channel, []model.Program) {
	channels := []model.Channel{
		{ID: "m3u:news", Name: "News HD", CategoryName: "News"},
		{ID: "m3u:sports", Name: "Sports One", CategoryName: "Sports"},
		{ID: "m3u:movies", Name: "Cinema Max", CategoryName: "Entertainment"},
	}
	// Deliberately out of order so the route has to sort.
	programs := []model.Program{
		{ID: "p-movie-1", ChannelID: "m3u:movies", Title: "Heat", StartUnix: 1000, EndUnix: 4000, Categories: []string{"Movie", "Drama"}},
		{ID: "p-sports-1", ChannelID: "m3u:sports", Title: "Lakers at Celtics", StartUnix: 1000, EndUnix: 2000, Categories: []string{"Sports event", "NBA Basketball"}},
		{ID: "p-news-2", ChannelID: "m3u:news", Title: "Late News", StartUnix: 2000, EndUnix: 3000, Categories: []string{"News"}},
		{ID: "p-news-1", ChannelID: "m3u:news", Title: "Evening News", StartUnix: 1000, EndUnix: 2000, Categories: []string{"News"}, ImageURL: "/relative.png"},
		{ID: "p-news-0", ChannelID: "m3u:news", Title: "Morning", StartUnix: 0, EndUnix: 1000, Categories: []string{"News"}},
		{ID: "p-orphan", ChannelID: "m3u:gone", Title: "Not in lineup", StartUnix: 1000, EndUnix: 2000},
		{ID: "p-sports-2", ChannelID: "m3u:sports", Title: "SportsCenter", StartUnix: 2000, EndUnix: 3000, Categories: []string{"Sports non-event", "Talk"}},
	}
	return channels, programs
}

func TestLiveTVProgramsWindowOverlapAndLineupOrder(t *testing.T) {
	t.Parallel()

	channels, programs := livetvTestLineup()
	server := liveTVProgramsServer(channels, programs)
	// [1000,2000) overlaps programs starting before 2000 and ending after 1000;
	// p-news-0 ends exactly at 1000 and p-news-2 starts exactly at 2000.
	got := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(1000, 2000, nil)))
	want := []string{"p-news-1", "p-sports-1", "p-movie-1"}
	if ids := livetvProgramIDs(got); !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	if got[0].ImageURL != "" || got[0].Categories == nil || got[0].Summary != "" {
		t.Fatalf("relative image must be dropped and empty fields kept: %+v", got[0])
	}

	got = decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 5000, nil)))
	want = []string{"p-news-0", "p-news-1", "p-news-2", "p-sports-1", "p-sports-2", "p-movie-1"}
	if ids := livetvProgramIDs(got); !reflect.DeepEqual(ids, want) {
		t.Fatalf("full window ids = %v, want %v", ids, want)
	}
}

func TestLiveTVProgramsChannelIDsFilterAndOrder(t *testing.T) {
	t.Parallel()

	channels, programs := livetvTestLineup()
	server := liveTVProgramsServer(channels, programs)
	got := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 5000, map[string]any{
		"channel_ids": "m3u:movies, m3u:unknown,m3u:news,m3u:movies,m3u:gone",
	})))
	want := []string{"p-movie-1", "p-news-0", "p-news-1", "p-news-2"}
	if ids := livetvProgramIDs(got); !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}

	got = decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 5000, map[string]any{
		"channel_ids": "m3u:news", "limit": "2",
	})))
	if ids := livetvProgramIDs(got); !reflect.DeepEqual(ids, []string{"p-news-0", "p-news-1"}) {
		t.Fatalf("explicit limit with channel_ids: ids = %v", ids)
	}
}

func TestLiveTVProgramsLimitDefaultAndCeiling(t *testing.T) {
	t.Parallel()

	channels := make([]model.Channel, 0, 30)
	programs := make([]model.Program, 0, 3000)
	for c := 0; c < 30; c++ {
		id := fmt.Sprintf("m3u:%02d", c)
		channels = append(channels, model.Channel{ID: id, Name: id})
		for p := 0; p < 100; p++ {
			programs = append(programs, model.Program{ID: fmt.Sprintf("%s-%03d", id, p), ChannelID: id, Title: "Show", StartUnix: int64(p * 60), EndUnix: int64(p*60 + 60)})
		}
	}
	server := liveTVProgramsServer(channels, programs)
	window := livetvWindowQuery(0, 6000, nil)

	got := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, window))
	if len(got) != 200 || got[0].ID != "m3u:00-000" || got[199].ID != "m3u:01-099" {
		t.Fatalf("default limit: got %d programs, first %s last %s", len(got), got[0].ID, got[len(got)-1].ID)
	}
	got = decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 6000, map[string]any{"limit": "5000"})))
	if len(got) != 2000 {
		t.Fatalf("limit ceiling: got %d programs, want 2000", len(got))
	}
	got = decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 6000, map[string]any{"limit": "3"})))
	if len(got) != 3 {
		t.Fatalf("explicit limit: got %d programs, want 3", len(got))
	}
	got = decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 6000, map[string]any{"channel_ids": "m3u:00,m3u:01,m3u:02,m3u:03"})))
	if len(got) != 400 {
		t.Fatalf("channel_ids has no default limit: got %d programs, want 400", len(got))
	}
}

func TestLiveTVProgramsClassFilter(t *testing.T) {
	t.Parallel()

	channels, programs := livetvTestLineup()
	server := liveTVProgramsServer(channels, programs)
	for class, want := range map[string][]string{
		"news":   {"p-news-0", "p-news-1", "p-news-2"},
		"sports": {"p-sports-1"},
		"movie":  {"p-movie-1"},
		"series": {"p-sports-2"},
		"kids":   {},
	} {
		got := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 5000, map[string]any{"class": class})))
		if ids := livetvProgramIDs(got); !reflect.DeepEqual(ids, want) {
			t.Fatalf("class %s: ids = %v, want %v", class, ids, want)
		}
		got = decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 5000, map[string]any{"class": class, "channel_ids": "m3u:movies,m3u:sports,m3u:news"})))
		for _, program := range got {
			if !program.has(class) {
				t.Fatalf("class %s with channel_ids returned %+v", class, program)
			}
		}
	}
}

func TestLiveTVProgramsRejectsBadRequests(t *testing.T) {
	t.Parallel()

	server := liveTVProgramsServer(livetvTestLineup())
	tooMany := make([]byte, 0, 2001*6)
	for i := 0; i <= livetvProgramsMaxChannels; i++ {
		if i > 0 {
			tooMany = append(tooMany, ',')
		}
		tooMany = append(tooMany, fmt.Sprintf("c%d", i)...)
	}
	for name, query := range map[string]map[string]any{
		"missing both":      nil,
		"missing end":       {"start": "1000"},
		"missing start":     {"end": "1000"},
		"non-numeric start": {"start": "soon", "end": "1000"},
		"end before start":  {"start": "2000", "end": "1000"},
		"unknown class":     {"start": "0", "end": "1000", "class": "documentary"},
		"bad limit":         {"start": "0", "end": "1000", "limit": "lots"},
		"zero limit":        {"start": "0", "end": "1000", "limit": "0"},
		"too many channels": {"start": "0", "end": "1000", "channel_ids": string(tooMany)},
	} {
		response := liveTVContractGet(t, server, livetvProgramsPath, query)
		if response.GetStatusCode() != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", name, response.GetStatusCode(), response.GetBody())
		}
		if code := decodeLiveTVCode(t, response); code != "bad_request" {
			t.Fatalf("%s: code = %q, want bad_request", name, code)
		}
	}
}

func TestLiveTVProgramsEmptyGuideIsArray(t *testing.T) {
	t.Parallel()

	for name, server := range map[string]*HTTPRoutesServer{
		"no lineup":   liveTVProgramsServer(nil, nil),
		"no programs": liveTVProgramsServer([]model.Channel{{ID: "m3u:news", Name: "News"}}, nil),
	} {
		for _, query := range []map[string]any{livetvWindowQuery(0, 1000, nil), livetvWindowQuery(0, 1000, map[string]any{"channel_ids": "m3u:news"})} {
			response := liveTVContractGet(t, server, livetvProgramsPath, query)
			if response.GetStatusCode() != http.StatusOK || string(response.GetBody()) != `{"programs":[]}` {
				t.Fatalf("%s: expected {\"programs\":[]}, got %d %s", name, response.GetStatusCode(), response.GetBody())
			}
		}
	}
}

func TestLiveTVProgramsMatchesFixtureShape(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "livetv-v1", "fixtures", "programs.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var fixture livetvPrograms
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("fixture does not decode into livetvPrograms: %v", err)
	}
	if len(fixture.Programs) == 0 {
		t.Fatal("fixture has no programs")
	}
	var fixtureKeys struct {
		Programs []map[string]json.RawMessage `json:"programs"`
	}
	if err := json.Unmarshal(raw, &fixtureKeys); err != nil {
		t.Fatalf("decode fixture keys: %v", err)
	}

	channels, programs := livetvTestLineup()
	server := liveTVProgramsServer(channels, programs)
	response := liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 5000, nil))
	decodeLiveTVPrograms(t, response)
	var got struct {
		Programs []map[string]json.RawMessage `json:"programs"`
	}
	if err := json.Unmarshal(response.GetBody(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for i, item := range got.Programs {
		if len(item) != len(fixtureKeys.Programs[0]) {
			t.Fatalf("program %d has keys %v, fixture has %v", i, item, fixtureKeys.Programs[0])
		}
		for key := range fixtureKeys.Programs[0] {
			if _, ok := item[key]; !ok {
				t.Fatalf("program %d is missing fixture key %q", i, key)
			}
		}
	}

	// The fixture's sports row classifies the same way the route does.
	sports := fixture.Programs[1]
	flags := livetvProgramFlags(model.Program{Categories: sports.Categories}, model.Channel{}, false)
	if !flags.IsSports || flags.IsSeries || flags.IsMovie {
		t.Fatalf("fixture sports categories classified as %+v", flags)
	}
}

func TestLiveTVProgramFlags(t *testing.T) {
	t.Parallel()

	plain := model.Channel{ID: "c", Name: "Channel 5", CategoryName: "US Entertainment"}
	cases := []struct {
		name        string
		categories  []string
		channel     model.Channel
		sportsMatch bool
		want        livetvFlags
	}{
		{name: "movie", categories: []string{"Movie"}, channel: plain, want: livetvFlags{IsMovie: true}},
		{name: "feature film beats drama series", categories: []string{"Feature Film", "Drama"}, channel: plain, want: livetvFlags{IsMovie: true}},
		{name: "film case-insensitive", categories: []string{"FILM"}, channel: plain, want: livetvFlags{IsMovie: true}},
		{name: "sports event", categories: []string{"Sports event", "Basketball"}, channel: plain, want: livetvFlags{IsSports: true}},
		{name: "league label", categories: []string{"NFL Football"}, channel: plain, want: livetvFlags{IsSports: true}},
		{name: "sports event with series label", categories: []string{"Sports", "Series"}, channel: plain, want: livetvFlags{IsSports: true}},
		{name: "sports non-event talk", categories: []string{"Sports non-event", "Talk"}, channel: plain, want: livetvFlags{IsSeries: true}},
		{name: "news", categories: []string{"News"}, channel: plain, want: livetvFlags{IsNews: true}},
		{name: "newsmagazine series", categories: []string{"Newsmagazine", "Series"}, channel: plain, want: livetvFlags{IsNews: true, IsSeries: true}},
		{name: "children animated", categories: []string{"Children", "Animated"}, channel: plain, want: livetvFlags{IsKids: true, IsSeries: true}},
		{name: "kids", categories: []string{"Kids"}, channel: plain, want: livetvFlags{IsKids: true}},
		{name: "family alone is not kids", categories: []string{"Family", "Sitcom"}, channel: plain, want: livetvFlags{IsSeries: true}},
		{name: "series words", categories: []string{"Reality"}, channel: plain, want: livetvFlags{IsSeries: true}},
		{name: "game show", categories: []string{"Game show"}, channel: plain, want: livetvFlags{IsSeries: true}},
		{name: "live label", categories: []string{"News", "Live"}, channel: plain, want: livetvFlags{IsNews: true, IsLive: true}},
		{name: "unmapped labels fall back to sports channel", categories: []string{"Entertainment", "Special"}, channel: model.Channel{ID: "s", Name: "beIN 1", CategoryName: "Fútbol"}, want: livetvFlags{IsSports: true}},
		{name: "no categories news channel", channel: model.Channel{ID: "n", Name: "Noticias 24"}, want: livetvFlags{IsNews: true}},
		{name: "no categories kids channel", channel: model.Channel{ID: "k", Name: "Cartoon Network"}, want: livetvFlags{IsKids: true}},
		{name: "no categories movie channel", channel: model.Channel{ID: "m", Name: "Cinema Max", CategoryName: "Premium"}, want: livetvFlags{IsMovie: true}},
		{name: "sports words outrank news on channel", channel: model.Channel{ID: "sn", Name: "Sports News"}, want: livetvFlags{IsSports: true}},
		{name: "no categories plain channel", channel: plain, want: livetvFlags{}},
		{name: "categories decide over channel", categories: []string{"News"}, channel: model.Channel{ID: "s2", Name: "ESPN", CategoryName: "Sports"}, want: livetvFlags{IsNews: true}},
		{name: "sports match overrides series", categories: []string{"Series", "Talk"}, channel: plain, sportsMatch: true, want: livetvFlags{IsSports: true}},
		{name: "sports match without categories", channel: plain, sportsMatch: true, want: livetvFlags{IsSports: true}},
		{name: "channel word inside a word does not match", channel: model.Channel{ID: "w", Name: "Newsmax Sportsman"}, want: livetvFlags{}},
	}
	for _, tc := range cases {
		got := livetvProgramFlags(model.Program{Categories: tc.categories}, tc.channel, tc.sportsMatch)
		if got != tc.want {
			t.Fatalf("%s: flags = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// Categories seen on the live server must not be mistaken for sports leagues.
func TestLiveTVCategorySignalsForLiveServerLabels(t *testing.T) {
	t.Parallel()

	for _, label := range []string{"Series", "Show", "Episode", "News", "Religious", "Entertainment", "Music", "Special", "Variety", "Children", "Documentary", "Educational", "Animated", "Sitcom", "Reality", "Talk", "Drama", "Paid Programming", "Shopping", "Comedy", "Game show", "Newsmagazine", "Sports non-event", "Public affairs"} {
		if livetvCategorySignal(label).flags.IsSports {
			t.Fatalf("%q must not classify as sports", label)
		}
	}
	for _, label := range []string{"Sports event", "Sports", "sport", "NBA Basketball", "College Football", "Premier League"} {
		if !livetvCategorySignal(label).flags.IsSports {
			t.Fatalf("%q must classify as sports", label)
		}
	}
}

func TestLiveTVGenericSportsTitle(t *testing.T) {
	t.Parallel()

	for title, want := range map[string]bool{
		"":                  true,
		"NBA Basketball":    true,
		"LIVE: NFL":         true,
		"College Football":  true,
		"Premier League":    true,
		"Lakers at Celtics": false,
		"SportsCenter":      false,
		"Evening News":      false,
	} {
		if got := livetvGenericSportsTitle(title, ""); got != want {
			t.Fatalf("livetvGenericSportsTitle(%q) = %v, want %v", title, got, want)
		}
	}
	if !livetvGenericSportsTitle("Liga MX", "Liga MX") {
		t.Fatal("a title equal to the event's league must be generic")
	}
}

func TestLiveTVProgramsSportsEnrichment(t *testing.T) {
	t.Parallel()

	channels := []model.Channel{{ID: "m3u:espn", Name: "ESPN", CategoryName: "Entertainment"}, {ID: "m3u:other", Name: "Other"}}
	programs := []model.Program{
		{ID: "generic", ChannelID: "m3u:espn", Title: "NBA Basketball", StartUnix: 1000, EndUnix: 4000, ImageURL: "https://img.example/guide.jpg", Categories: []string{"Talk"}},
		{ID: "titled", ChannelID: "m3u:espn", Title: "Thunder at Spurs", StartUnix: 5000, EndUnix: 8000, ImageURL: "https://img.example/titled.jpg"},
		{ID: "open-end", ChannelID: "m3u:espn", Title: "Postgame", StartUnix: 20000, EndUnix: 21000},
		{ID: "before", ChannelID: "m3u:espn", Title: "Morning Show", StartUnix: 0, EndUnix: 1000, Categories: []string{"Talk"}},
		{ID: "other-channel", ChannelID: "m3u:other", Title: "Movie Night", StartUnix: 1000, EndUnix: 4000},
	}
	server := liveTVProgramsServer(channels, programs)
	// XC's sports feature is off unless an admin turns it on.
	server.store.SetAdminSettings(json.RawMessage(`{"sportsEnabled":true}`))
	server.sportsPrepared = sportsPreparedCache{Ready: true, ExpiresAfter: time.Now().Add(time.Minute), Payload: SportsPayload{Events: []SportsEvent{
		{ID: "e1", Name: "Lakers at Celtics", LeagueName: "NBA", StartUnix: 1500, EndUnix: 4500, Live: true,
			GameThumbsBackgroundURL: "https://thumbs.example/lakers.jpg", ImageURL: "/xtream/api/sports/image?id=1",
			Channels: []SportsChannelMatch{{ID: "m3u:espn"}}},
		{ID: "e2", Name: "Thunder vs Spurs", LeagueName: "NBA", StartUnix: 5000, EndUnix: 8000,
			GameThumbsBackgroundURL: "/xtream/api/sports/image?id=2", Channels: []SportsChannelMatch{{ID: "m3u:espn"}}},
		// EndUnix 0 lasts three hours from its start.
		{ID: "e3", Name: "Heat at Knicks", StartUnix: 20000 - 3*3600 + 500, Channels: []SportsChannelMatch{{ID: "m3u:espn"}}},
	}}}

	got := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 30000, nil)))
	byID := map[string]livetvProgram{}
	for _, program := range got {
		byID[program.ID] = program
	}
	generic := byID["generic"]
	if generic.Title != "Lakers at Celtics" || generic.ImageURL != "https://thumbs.example/lakers.jpg" || !generic.IsSports || generic.IsSeries || !generic.IsLive {
		t.Fatalf("generic program not enriched: %+v", generic)
	}
	titled := byID["titled"]
	if titled.Title != "Thunder at Spurs" || titled.ImageURL != "https://img.example/titled.jpg" || !titled.IsSports || titled.IsLive {
		t.Fatalf("titled program must keep its title and image: %+v", titled)
	}
	if openEnd := byID["open-end"]; !openEnd.IsSports || openEnd.Title != "Postgame" {
		t.Fatalf("event without an end must cover three hours: %+v", openEnd)
	}
	if before := byID["before"]; before.IsSports || !before.IsSeries {
		t.Fatalf("a program that only touches the event must not match: %+v", before)
	}
	if other := byID["other-channel"]; other.IsSports {
		t.Fatalf("events only enrich their own channels: %+v", other)
	}
	sportsOnly := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 30000, map[string]any{"class": "sports"})))
	if ids := livetvProgramIDs(sportsOnly); !reflect.DeepEqual(ids, []string{"generic", "titled", "open-end"}) {
		t.Fatalf("class=sports must use enriched flags: %v", ids)
	}

	server.store.SetAdminSettings(json.RawMessage(`{"sportsEnabled":false}`))
	disabled := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 30000, map[string]any{"channel_ids": "m3u:espn"})))
	if disabled[1].ID != "generic" || disabled[1].Title != "NBA Basketball" || disabled[1].IsSports {
		t.Fatalf("sports enrichment must be off when the sports feature is off: %+v", disabled[1])
	}
}

func TestLiveTVProgramsFeaturedSportsComeFirstAndFillerIsUnclassified(t *testing.T) {
	t.Parallel()

	channels := []model.Channel{
		{ID: "m3u:flo", Name: "FloSports 331", CategoryName: "US Sports | FloSports"},
		{ID: "m3u:espn", Name: "ESPN", CategoryName: "US Sports"},
	}
	programs := []model.Program{
		{ID: "filler", ChannelID: "m3u:flo", Title: "SEC+ / ACC extra 95: NO EVENT", StartUnix: 1000, EndUnix: 4000},
		{ID: "flo-show", ChannelID: "m3u:flo", Title: "Hockey Weekly", StartUnix: 4000, EndUnix: 5000},
		{ID: "game", ChannelID: "m3u:espn", Title: "Thunder at Spurs", StartUnix: 1500, EndUnix: 4500},
	}
	server := liveTVProgramsServer(channels, programs)
	// XC's sports feature is off unless an admin turns it on.
	server.store.SetAdminSettings(json.RawMessage(`{"sportsEnabled":true}`))
	server.sportsPrepared = sportsPreparedCache{Ready: true, ExpiresAfter: time.Now().Add(time.Minute), Payload: SportsPayload{Events: []SportsEvent{
		{ID: "e1", Name: "Thunder at Spurs", StartUnix: 1500, EndUnix: 4500, Channels: []SportsChannelMatch{{ID: "m3u:espn"}}},
	}}}

	all := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 10000, nil)))
	if ids := livetvProgramIDs(all); !reflect.DeepEqual(ids, []string{"game", "filler", "flo-show"}) {
		t.Fatalf("whole-lineup order = %v, want the featured game first, then lineup order", ids)
	}
	if !all[0].Featured || all[1].Featured || all[2].Featured {
		t.Fatalf("featured flags = %v %v %v", all[0].Featured, all[1].Featured, all[2].Featured)
	}
	if filler := all[1]; filler.IsSports || filler.IsSeries || filler.IsMovie || filler.IsNews || filler.IsKids {
		t.Fatalf("filler must stay in the grid but in no row: %+v", filler)
	}
	sports := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 10000, map[string]any{"class": "sports", "limit": "1"})))
	if ids := livetvProgramIDs(sports); !reflect.DeepEqual(ids, []string{"game"}) {
		t.Fatalf("class=sports limit=1 = %v, want the featured game", ids)
	}
	// channel_ids keeps request order; featured does not reorder it.
	scoped := decodeLiveTVPrograms(t, liveTVContractGet(t, server, livetvProgramsPath, livetvWindowQuery(0, 10000, map[string]any{"channel_ids": "m3u:flo,m3u:espn"})))
	if ids := livetvProgramIDs(scoped); !reflect.DeepEqual(ids, []string{"filler", "flo-show", "game"}) {
		t.Fatalf("channel_ids order = %v", ids)
	}
}

func TestLiveTVFillerTitle(t *testing.T) {
	t.Parallel()
	for title, want := range map[string]bool{
		"SEC+ / ACC extra 95: NO EVENT": true, "Off Air": true, "TBA": true, "No Live Events": true, "": true,
		"Lakers at Celtics": false, "Event Horizon": false, "Hockey Weekly": false,
	} {
		if got := livetvFillerTitle(title); got != want {
			t.Errorf("livetvFillerTitle(%q) = %v, want %v", title, got, want)
		}
	}
}
