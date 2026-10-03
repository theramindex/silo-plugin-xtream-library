package plugin

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

// syntheticSportsGuide builds a large lineup: every channel carries hourly
// programs over several days, and a few dozen channels carry real fixtures.
func syntheticSportsGuide(channels, hours, fixtures int, now time.Time) (cache.Snapshot, []SportsEvent) {
	snapshot := cache.Snapshot{}
	base := now.Truncate(time.Hour).Add(-24 * time.Hour)
	events := make([]SportsEvent, 0, fixtures)
	for index := 0; index < fixtures; index++ {
		home := fmt.Sprintf("Home Club %03d", index)
		away := fmt.Sprintf("Away Club %03d", index)
		start := base.Add(time.Duration(24+index%48) * time.Hour)
		events = append(events, SportsEvent{
			ID: fmt.Sprintf("event:%d", index), LeagueID: "nhl", LeagueName: "NHL", Name: away + " at " + home,
			StartUnix: start.Unix(), Home: SportsTeam{Name: home}, Away: SportsTeam{Name: away},
		})
	}
	for channel := 0; channel < channels; channel++ {
		id := fmt.Sprintf("channel:%d", channel)
		snapshot.Catalog.Channels = append(snapshot.Catalog.Channels, model.Channel{ID: id, Name: fmt.Sprintf("Network %d HD", channel), CategoryName: "Entertainment"})
		for hour := 0; hour < hours; hour++ {
			start := base.Add(time.Duration(hour) * time.Hour)
			title := fmt.Sprintf("Program %d-%d", channel, hour)
			if channel < fixtures && hour == 24+channel%48 {
				title = events[channel].Away.Name + " vs " + events[channel].Home.Name
			}
			snapshot.Catalog.Programs = append(snapshot.Catalog.Programs, model.Program{
				ID: fmt.Sprintf("%s:%d", id, hour), ChannelID: id, Title: title, StartUnix: start.Unix(), EndUnix: start.Add(time.Hour).Unix(),
			})
		}
	}
	return snapshot, events
}

func BenchmarkSportsChannelMatchingLargeGuide(b *testing.B) {
	now := time.Now()
	snapshot, events := syntheticSportsGuide(1500, 96, 120, now)
	index := newSportsChannelIndex(snapshot)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for _, event := range events {
			index.MatchDetailedContext(ctx, event)
		}
	}
}

func BenchmarkSportsChannelIndexBuildLargeGuide(b *testing.B) {
	snapshot, _ := syntheticSportsGuide(1500, 96, 120, time.Now())
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		newSportsChannelIndex(snapshot)
	}
}

func TestSyntheticSportsGuideMatchesEachFixtureChannel(t *testing.T) {
	t.Parallel()
	snapshot, events := syntheticSportsGuide(200, 96, 20, time.Now())
	index := newSportsChannelIndex(snapshot)
	for position, event := range events {
		matches, _ := index.MatchDetailed(event)
		assertSportsMatch(t, matches, fmt.Sprintf("channel:%d", position))
		if len(matches) != 1 {
			t.Fatalf("fixture %d should match only its own channel, got %+v", position, matches)
		}
	}
}
