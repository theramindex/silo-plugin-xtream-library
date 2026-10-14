package plugin

import (
	"fmt"
	"strings"
	"testing"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

// rowTitles lists a whole-lineup row's titles in order.
func rowTitles(channels []model.Channel, programs []model.Program, class string, sports map[string][]livetvSportsSlot) []string {
	for i := range programs {
		programs[i].ID = fmt.Sprint("p", i)
		programs[i].StartUnix, programs[i].EndUnix = 1000, 5000
	}
	index := buildGuideIndex(cache.Snapshot{Catalog: model.CatalogState{Channels: channels, Programs: programs}})
	var titles []string
	for _, p := range livetvSelectPrograms(index, livetvProgramsQuery{Window: guideWindow{Start: 2000, End: 2060}, Class: class, Limit: 200}, sports) {
		titles = append(titles, p.Title)
	}
	return titles
}

// The Sports row puts matched games first, then programs titled like a game,
// then live sports, then the rest; placeholders, previews, radio feeds and
// titles many channels repeat come last, and filler is left out even during a
// game's slot. A placeholder matched to a game takes the game's name.
func TestLiveTVSportsRowOrder(t *testing.T) {
	t.Parallel()

	channels := []model.Channel{
		{ID: "talk", Name: "Sports Talk", CategoryName: "Sports"},
		{ID: "espn", Name: "LBW: ESPN", CategoryName: "Sports"},
		{ID: "radio", Name: "Radio: Boston Bruins", CategoryName: "Sports"},
		{ID: "team", Name: "Los Angeles Dodgers", CategoryName: "Sports"},
		{ID: "game", Name: "NESN", CategoryName: "Sports"},
		{ID: "live", Name: "Willow", CategoryName: "Sports"},
		{ID: "event", Name: "NBA 08", CategoryName: "Sports"},
		{ID: "matched", Name: "DE Sportdigital (FHD)", CategoryName: "Sports"},
	}
	programs := []model.Program{
		{ChannelID: "talk", Title: "Pro Football Talk"},
		{ChannelID: "espn", Title: "MX: LBW: ESPN"},
		{ChannelID: "radio", Title: "Radio: Boston Bruins at San Jose Sharks"},
		{ChannelID: "team", Title: "Next Game: Brewers @ Dodgers on 2026-10-14"},
		{ChannelID: "game", Title: "NHL Hockey : Boston Bruins at San Jose Sharks"},
		{ChannelID: "live", Title: "Cricket ᴸᶦᵛᵉ"},
		{ChannelID: "event", Title: "NBA 08: NO EVENT"},
		{ChannelID: "matched", Title: "DE Sportdigital (FHD)"},
	}
	for i := range livetvRepeatedChannels {
		id := fmt.Sprint("feed", i)
		channels = append(channels, model.Channel{ID: id, Name: "Feed " + id, CategoryName: "Sports"})
		programs = append(programs, model.Program{ChannelID: id, Title: "Sports Play-By-Play"})
	}
	slot := []livetvSportsSlot{{StartUnix: 1000, EndUnix: 5000, Name: "Braga vs Sporting"}}
	sports := map[string][]livetvSportsSlot{"matched": slot, "event": slot, "radio": slot}

	got := strings.Join(rowTitles(channels, programs, "sports", sports), " | ")
	want := strings.Join([]string{
		"Braga vs Sporting",
		"NHL Hockey : Boston Bruins at San Jose Sharks",
		"Cricket ᴸᶦᵛᵉ",
		"Pro Football Talk",
		"MX: LBW: ESPN",
		"Radio: Boston Bruins at San Jose Sharks",
		"Next Game: Brewers @ Dodgers on 2026-10-14",
	}, " | ") + strings.Repeat(" | Sports Play-By-Play", livetvRepeatedChannels)
	if got != want {
		t.Fatalf("sports row =\n%s\nwant\n%s", got, want)
	}
}

// Airtime sold to advertisers is in no row, whatever the channel suggests.
func TestLiveTVRowsLeaveOutAdvertising(t *testing.T) {
	t.Parallel()

	channels := []model.Channel{{ID: "film4", Name: "Film4", CategoryName: "Movies"}, {ID: "cine", Name: "Cine Estelar", CategoryName: "Movies"}, {ID: "amc", Name: "AMC", CategoryName: "Movies"}}
	programs := []model.Program{
		{ChannelID: "film4", Title: "Teleshopping"},
		{ChannelID: "cine", Title: "Programación pagada"},
		{ChannelID: "amc", Title: "Grumpy Old Men"},
		{ChannelID: "amc", Title: "Best Blender Ever", Categories: []string{"Paid Programming"}},
	}
	if got := strings.Join(rowTitles(channels, programs, "movie", nil), " | "); got != "Grumpy Old Men" {
		t.Fatalf("movie row = %s", got)
	}
}

func TestLiveTVPlaceholderTitle(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		title, channel string
		want           bool
	}{
		{"LBW: ESPN", "LBW: ESPN", true},
		{"MX: HBO", "HBO", true},
		{"DE DAZN 2 (FHD)", "DAZN 2", true},
		{"Sky Cinema Premiere HEVC LB (1080p)", "Sky Cinema Premiere", true},
		{"EN◈ OnePlay Marvel 1 ᴴᴰ", "EN◈ OnePlay Marvel 1 ᴴᴰ", true},
		{"Family Matters", "LBW: BET", false},
		{"NBA 01: Hawks vs Thunder", "NBA 01", false},
		{"", "HBO", false},
	} {
		if got := livetvPlaceholderTitle(tc.title, model.Channel{Name: tc.channel}); got != tc.want {
			t.Errorf("livetvPlaceholderTitle(%q, %q) = %v, want %v", tc.title, tc.channel, got, tc.want)
		}
	}
}
