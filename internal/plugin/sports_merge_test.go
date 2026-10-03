package plugin

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

// referenceMergeSportsGuideEvents is the quadratic merge the bucketed one
// replaced, kept verbatim as the behavioral reference.
func referenceMergeSportsGuideEvents(events, guideEvents []SportsEvent) []SportsEvent {
	merged := cloneSportsEvents(events)
	for _, guideEvent := range guideEvents {
		matched := false
		for index := range merged {
			if !sportsEventsSameMatchup(merged[index], guideEvent) {
				continue
			}
			merged[index].Channels = mergeSportsChannelMatches(guideEvent.Channels, merged[index].Channels)
			if guideEvent.SportName != "" && guideEvent.SportName != "Sports" {
				merged[index].LeagueID = guideEvent.LeagueID
				merged[index].LeagueName = guideEvent.LeagueName
				merged[index].SportName = guideEvent.SportName
				merged[index].LeagueLogoURL = firstNonEmpty(guideEvent.LeagueLogoURL, merged[index].LeagueLogoURL)
			}
			for _, pair := range [][2]*SportsTeam{{&merged[index].Home, &guideEvent.Home}, {&merged[index].Away, &guideEvent.Away}} {
				if normalizeMatchText(pair[0].Name) == normalizeMatchText(pair[1].Name) {
					*pair[0] = mergeSportsTeamIdentity(*pair[1], *pair[0])
				}
			}
			if guideEvent.Live && merged[index].Status == "scheduled" {
				merged[index].Live = true
				merged[index].Status = "airing"
				merged[index].StatusText = "On now"
			}
			matched = true
			break
		}
		if !matched {
			if guideEvent.Live && guideEvent.Status == "airing" {
				for _, providerEvent := range merged {
					if providerEvent.Completed && providerEvent.StartUnix < guideEvent.StartUnix && sportsEventsSameIdentity(providerEvent, guideEvent) {
						guideEvent.Status = "replay"
						guideEvent.StatusText = "Replay"
						break
					}
				}
			}
			merged = append(merged, guideEvent)
		}
	}
	return merged
}

func randomMergeEvents(random *rand.Rand, count int, base time.Time, prefix string) []SportsEvent {
	teams := []string{"Boston Bruins", "Montreal Canadiens", "Toronto Maple Leafs", "Bruins", "Canadiens", "New York Rangers", "Texas Rangers", "Rangers", "Seattle Kraken", "Kraken"}
	offsets := []time.Duration{0, 10 * time.Minute, 2 * time.Hour, 5*time.Hour + 59*time.Minute, 6 * time.Hour, 6*time.Hour + time.Second, 7 * time.Hour, 20 * time.Hour, -6 * time.Hour, -50 * time.Hour}
	sports := []string{"Hockey", "Sports", ""}
	events := make([]SportsEvent, 0, count)
	for index := 0; index < count; index++ {
		home := teams[random.Intn(len(teams))]
		away := teams[random.Intn(len(teams))]
		sport := sports[random.Intn(len(sports))]
		event := SportsEvent{
			ID: fmt.Sprintf("%s:%d", prefix, index), LeagueID: map[string]string{"Hockey": "nhl", "Sports": "sports"}[sport], SportName: sport,
			Name: away + " at " + home, LeagueName: []string{"NHL", "", "Hockey"}[random.Intn(3)],
			Home:      SportsTeam{ID: prefix + ":" + home, Name: home, Abbreviation: []string{"", "BOS", "NYR", "SEA"}[random.Intn(4)]},
			Away:      SportsTeam{Name: away, Abbreviation: []string{"", "MTL", "TOR"}[random.Intn(3)]},
			StartUnix: base.Add(offsets[random.Intn(len(offsets))]).Unix(), Status: "scheduled",
			Channels: []SportsChannelMatch{{ID: fmt.Sprintf("channel:%d", random.Intn(9)), Name: "Channel", Score: random.Intn(100)}},
		}
		switch random.Intn(6) {
		case 0:
			event.StartUnix = 0
		case 1:
			event.Completed = true
			event.Status = "final"
		case 2:
			event.Live = true
			event.Status = "airing"
		case 3:
			event.Live = true
			event.Status = "live"
		}
		events = append(events, event)
	}
	return events
}

func TestBucketedSportsMergeMatchesQuadraticReference(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, time.October, 3, 19, 0, 0, 0, time.UTC)
	for seed := int64(1); seed <= 60; seed++ {
		random := rand.New(rand.NewSource(seed))
		events := randomMergeEvents(random, 60, base, "provider")
		guide := randomMergeEvents(random, 60, base, "guide")
		got := mergeSportsGuideEvents(events, guide)
		want := referenceMergeSportsGuideEvents(events, guide)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("seed %d: bucketed guide merge diverged\n got %d events\nwant %d events", seed, len(got), len(want))
		}
	}
}

func TestSportsEventStartIndexCandidates(t *testing.T) {
	t.Parallel()
	index := newSportsEventStartIndex()
	start := time.Date(2026, time.October, 3, 19, 0, 0, 0, time.UTC).Unix()
	index.add(0, start)
	index.add(1, 0)
	index.add(2, start+100*3600)
	index.add(3, start+6*3600)
	if got := index.candidates(start, nil); !reflect.DeepEqual(got, []int{0, 1, 3}) {
		t.Fatalf("candidates = %v", got)
	}
	if got := index.candidates(0, nil); !reflect.DeepEqual(got, []int{0, 1, 2, 3}) {
		t.Fatalf("undated events compare against everything, got %v", got)
	}
}

func BenchmarkMergeSportsGuideEvents(b *testing.B) {
	base := time.Date(2026, time.October, 3, 19, 0, 0, 0, time.UTC)
	events := make([]SportsEvent, 0, 1000)
	guide := make([]SportsEvent, 0, 1000)
	for index := 0; index < 1000; index++ {
		start := base.Add(time.Duration(index%96) * time.Hour).Unix()
		events = append(events, SportsEvent{
			ID: fmt.Sprintf("event:%d", index), Name: fmt.Sprintf("Away Club %d at Home Club %d", index, index),
			Home: SportsTeam{Name: fmt.Sprintf("Home Club %d", index)}, Away: SportsTeam{Name: fmt.Sprintf("Away Club %d", index)}, StartUnix: start,
		})
		guide = append(guide, SportsEvent{
			ID: fmt.Sprintf("guide:%d", index), Name: fmt.Sprintf("Away Club %d vs Home Club %d", index+500, index+500),
			Home: SportsTeam{Name: fmt.Sprintf("Home Club %d", index+500)}, Away: SportsTeam{Name: fmt.Sprintf("Away Club %d", index+500)}, StartUnix: start,
		})
	}
	b.Run("bucketed", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			mergeSportsGuideEvents(events, guide)
		}
	})
	b.Run("quadratic-reference", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			referenceMergeSportsGuideEvents(events, guide)
		}
	})
}
