package plugin

import (
	"sort"
	"strings"
)

const (
	sportsEventBucketSeconds = int64(15 * 60)
	// The widest start-time gap sportsEventsSameMatchup accepts.
	sportsMatchupMaxWindowSeconds = int64(6 * 3600)
)

func mergeSportsGuideEvents(events, guideEvents []SportsEvent) []SportsEvent {
	merged := cloneSportsEvents(events)
	identities := make([]sportsEventIdentityText, 0, len(merged)+len(guideEvents))
	index := newSportsEventStartIndex()
	for position, event := range merged {
		index.add(position, event.StartUnix)
		identities = append(identities, newSportsEventIdentityText(event))
	}
	var candidates []int
	for _, guideEvent := range guideEvents {
		matched := false
		guideIdentity := newSportsEventIdentityText(guideEvent)
		candidates = index.candidates(guideEvent.StartUnix, candidates[:0])
		for _, position := range candidates {
			if !sportsStartsWithinMatchupWindow(merged[position], guideEvent) || !identities[position].sameIdentity(guideIdentity) {
				continue
			}
			overlaySportsGuideEvent(&merged[position], guideEvent)
			// The overlay may lengthen team names and change the league.
			identities[position] = newSportsEventIdentityText(merged[position])
			matched = true
			break
		}
		if !matched {
			if guideEvent.Live && guideEvent.Status == "airing" {
				// A live listing may replay a provider game of any age.
				for position, providerEvent := range merged {
					if providerEvent.Completed && providerEvent.StartUnix < guideEvent.StartUnix && identities[position].sameIdentity(guideIdentity) {
						guideEvent.Status = "replay"
						guideEvent.StatusText = "Replay"
						break
					}
				}
			}
			index.add(len(merged), guideEvent.StartUnix)
			merged = append(merged, guideEvent)
			identities = append(identities, guideIdentity)
		}
	}
	return merged
}

// sportsEventIdentityText holds the normalized names sportsEventsSameIdentity
// compares, so a merge normalizes each event once rather than per pair.
type sportsEventIdentityText struct {
	text               string
	name, shortName    string
	homeName, awayName string
	homeAbbr, awayAbbr string
	leagueName         string
}

func newSportsEventIdentityText(event SportsEvent) sportsEventIdentityText {
	return sportsEventIdentityText{
		text:       normalizeMatchText(strings.Join([]string{event.Name, event.ShortName, event.Away.Name, event.Home.Name}, " ")),
		name:       normalizeMatchText(event.Name),
		shortName:  normalizeMatchText(event.ShortName),
		homeName:   normalizeMatchText(event.Home.Name),
		awayName:   normalizeMatchText(event.Away.Name),
		homeAbbr:   normalizeMatchText(event.Home.Abbreviation),
		awayAbbr:   normalizeMatchText(event.Away.Abbreviation),
		leagueName: normalizeMatchText(event.LeagueName),
	}
}

// sameIdentity is sportsEventsSameIdentity over prepared text.
func (left sportsEventIdentityText) sameIdentity(right sportsEventIdentityText) bool {
	return right.foundIn(left.text) && left.foundIn(right.text)
}

// foundIn is strongSportsGuideMatch with the event names normalized once.
func (event sportsEventIdentityText) foundIn(text string) bool {
	if containsNormalizedTerm(text, event.name) || containsNormalizedTerm(text, event.shortName) {
		return true
	}
	homeName := containsNormalizedTerm(text, event.homeName)
	awayName := containsNormalizedTerm(text, event.awayName)
	if homeName && awayName {
		return true
	}
	homeAbbr := containsNormalizedTerm(text, event.homeAbbr)
	awayAbbr := containsNormalizedTerm(text, event.awayAbbr)
	if (homeName || homeAbbr) && (awayName || awayAbbr) {
		return true
	}
	return containsNormalizedTerm(text, event.leagueName) && (homeName || awayName || homeAbbr || awayAbbr)
}

func sportsStartsWithinMatchupWindow(left, right SportsEvent) bool {
	if left.StartUnix <= 0 || right.StartUnix <= 0 {
		return true
	}
	difference := left.StartUnix - right.StartUnix
	if difference < 0 {
		difference = -difference
	}
	return difference <= sportsMatchupMaxWindowSeconds
}

// overlaySportsGuideEvent folds a guide listing into the provider game it
// matched: the guide's channels, its specific league and its team identities.
func overlaySportsGuideEvent(base *SportsEvent, guideEvent SportsEvent) {
	base.Channels = mergeSportsChannelMatches(guideEvent.Channels, base.Channels)
	if guideEvent.SportName != "" && guideEvent.SportName != "Sports" {
		base.LeagueID = guideEvent.LeagueID
		base.LeagueName = guideEvent.LeagueName
		base.SportName = guideEvent.SportName
		base.LeagueLogoURL = firstNonEmpty(guideEvent.LeagueLogoURL, base.LeagueLogoURL)
	}
	for _, pair := range [][2]*SportsTeam{{&base.Home, &guideEvent.Home}, {&base.Away, &guideEvent.Away}} {
		if normalizeMatchText(pair[0].Name) == normalizeMatchText(pair[1].Name) {
			*pair[0] = mergeSportsTeamIdentity(*pair[1], *pair[0])
		}
	}
	if guideEvent.Live && base.Status == "scheduled" {
		base.Live = true
		base.Status = "airing"
		base.StatusText = "On now"
	}
}

func sportsEventsSameMatchup(left, right SportsEvent) bool {
	return sportsStartsWithinMatchupWindow(left, right) && sportsEventsSameIdentity(left, right)
}

func sportsEventsSameIdentity(left, right SportsEvent) bool {
	leftText := normalizeMatchText(strings.Join([]string{left.Name, left.ShortName, left.Away.Name, left.Home.Name}, " "))
	rightText := normalizeMatchText(strings.Join([]string{right.Name, right.ShortName, right.Away.Name, right.Home.Name}, " "))
	return strongSportsGuideMatch(leftText, right) && strongSportsGuideMatch(rightText, left)
}

// sportsEventStartIndex buckets event positions by 15-minute start time, so
// a merge only compares events whose starts can fall in the same matchup
// window. Undated events stay candidates for everything, as before.
type sportsEventStartIndex struct {
	buckets map[int64][]int
	undated []int
	count   int
}

func newSportsEventStartIndex() *sportsEventStartIndex {
	return &sportsEventStartIndex{buckets: map[int64][]int{}}
}

func sportsEventBucket(start int64) int64 {
	bucket := start / sportsEventBucketSeconds
	if start < 0 && start%sportsEventBucketSeconds != 0 {
		bucket--
	}
	return bucket
}

func (index *sportsEventStartIndex) add(position int, start int64) {
	if position >= index.count {
		index.count = position + 1
	}
	if start <= 0 {
		index.undated = append(index.undated, position)
		return
	}
	bucket := sportsEventBucket(start)
	index.buckets[bucket] = append(index.buckets[bucket], position)
}

func (index *sportsEventStartIndex) all(buffer []int) []int {
	for position := 0; position < index.count; position++ {
		buffer = append(buffer, position)
	}
	return buffer
}

// candidates returns, in insertion order, every position that could share a
// matchup with an event starting at start.
func (index *sportsEventStartIndex) candidates(start int64, buffer []int) []int {
	if start <= 0 {
		return index.all(buffer)
	}
	buffer = append(buffer, index.undated...)
	first := sportsEventBucket(start - sportsMatchupMaxWindowSeconds)
	last := sportsEventBucket(start + sportsMatchupMaxWindowSeconds)
	for bucket := first; bucket <= last; bucket++ {
		buffer = append(buffer, index.buckets[bucket]...)
	}
	sort.Ints(buffer)
	return buffer
}
