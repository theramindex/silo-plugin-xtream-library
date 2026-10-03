package plugin

import (
	"fmt"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

func TestCapSportsEventsByLeagueSharesSlotsAndKeepsLive(t *testing.T) {
	t.Parallel()
	events := []SportsEvent{}
	for index := 0; index < 300; index++ {
		events = append(events, SportsEvent{ID: fmt.Sprintf("mlb:%d", index), LeagueID: "mlb"})
	}
	for index := 0; index < 20; index++ {
		events = append(events, SportsEvent{ID: fmt.Sprintf("nhl:%d", index), LeagueID: "nhl"})
	}
	events = append(events, SportsEvent{ID: "live:late", LeagueID: "mlb", Live: true})

	capped := capSportsEventsByLeague(events, 60, 100)
	if len(capped) != 100 {
		t.Fatalf("overall cap = %d, want 100", len(capped))
	}
	counts := map[string]int{}
	live := false
	for _, event := range capped {
		counts[event.LeagueID]++
		live = live || event.ID == "live:late"
	}
	if counts["nhl"] != 20 {
		t.Fatalf("a large league must not crowd out a small one: %+v", counts)
	}
	if !live {
		t.Fatal("live games must be kept even past their league's share")
	}
	if capped[0].ID != "mlb:0" || capped[len(capped)-1].ID != "live:late" {
		t.Fatalf("priority order must be preserved: first=%s last=%s", capped[0].ID, capped[len(capped)-1].ID)
	}
	if same := capSportsEventsByLeague(events[:50], 10, 100); len(same) != 50 {
		t.Fatal("lists under the cap are returned untouched")
	}
}

func TestSportsEventsFromGuideCapsPerLeague(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	snapshot := cache.Snapshot{}
	snapshot.Catalog.Channels = []model.Channel{{ID: "channel:mlb", Name: "MLB Extra Innings"}, {ID: "channel:nhl", Name: "NHL Center Ice"}}
	for index := 0; index < sportsGuideEventLimit+40; index++ {
		start := now.Add(time.Duration(index) * 10 * time.Minute)
		snapshot.Catalog.Programs = append(snapshot.Catalog.Programs, model.Program{
			ID: fmt.Sprintf("mlb:%d", index), ChannelID: "channel:mlb", Title: fmt.Sprintf("MLB Baseball: Away %d at Home %d", index, index),
			StartUnix: start.Unix(), EndUnix: start.Add(3 * time.Hour).Unix(),
		})
	}
	late := now.Add(60 * time.Hour)
	for index := 0; index < 10; index++ {
		snapshot.Catalog.Programs = append(snapshot.Catalog.Programs, model.Program{
			ID: fmt.Sprintf("nhl:%d", index), ChannelID: "channel:nhl", Title: fmt.Sprintf("NHL Hockey: Visitors %d at Hosts %d", index, index),
			StartUnix: late.Unix(), EndUnix: late.Add(3 * time.Hour).Unix(),
		})
	}
	events := sportsEventsFromGuide(snapshot, now)
	counts := map[string]int{}
	for _, event := range events {
		counts[event.LeagueID]++
	}
	if len(events) != sportsGuideEventLimit || counts["nhl"] != 10 {
		t.Fatalf("later NHL games must survive a crowded MLB guide: total=%d counts=%+v", len(events), counts)
	}
}
