package plugin

import (
	"slices"
	"strings"

	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

// Ranking for the program rows Jellyfin builds from the livetv/v1 programs
// route (a whole-lineup request with a class): real games first, then the rest
// in lineup order, with placeholders, radio feeds and titles every channel
// repeats last. The guide grid (channel_ids) is never reordered.

// Row tiers, best first.
const (
	livetvTierFeatured = iota // matched to a game in the sports guide
	livetvTierMatchup         // a sports program titled like a game: "A vs B", "A at B"
	livetvTierLive            // a sports program marked live
	livetvTierNormal
	livetvTierLow // placeholder titles, radio feeds, titles many channels repeat
)

// livetvRepeatedChannels is how many channels must carry the same title in one
// request before it ranks low: "Sports Play-By-Play" on forty feeds.
const livetvRepeatedChannels = 5

// livetvUnclassedLabels are guide categories for airtime sold to advertisers;
// a program with one is in no row, whatever its channel.
var livetvUnclassedLabels = map[string]bool{
	"paid programming": true, "infomercial": true, "teleshopping": true, "shopping": true,
}

// livetvUnclassedTitle reports a title for advertising airtime, such as Film4's
// overnight "Teleshopping", which the channel would otherwise class as a movie.
func livetvUnclassedTitle(title string) bool {
	normalized := " " + strings.Join(livetvWords(title), " ") + " "
	for _, phrase := range []string{
		" teleshopping ", " paid programming ", " infomercial ", " paid program ", " infomerciales ",
		" programación pagada ", " programacion pagada ", " teletienda ", " televenta ", " téléachat ", " dauerwerbesendung ",
	} {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

// livetvUnclassedProgram reports a program that belongs in no row: filler, or
// airtime sold to advertisers.
func livetvUnclassedProgram(program model.Program) bool {
	if livetvFillerTitle(program.Title) || livetvUnclassedTitle(program.Title) {
		return true
	}
	for _, label := range program.Categories {
		if livetvUnclassedLabels[strings.Join(livetvWords(label), " ")] {
			return true
		}
	}
	return false
}

// livetvPlaceholderTitle reports a program titled with its own channel's name,
// as generated guides fill a channel they have no listings for: "LBW: ESPN" on
// LBW: ESPN, and the same name with a feed code or quality tag, "MX: HBO" or
// "Sky Cinema Premiere HEVC LB (1080p)".
func livetvPlaceholderTitle(title string, channel model.Channel) bool {
	names := [][]string{livetvNameWords(channel.Name), livetvNameWords(livetvWithoutCode(channel.Name))}
	for _, words := range [][]string{livetvNameWords(title), livetvNameWords(livetvWithoutCode(title))} {
		if len(words) == 0 {
			continue
		}
		for _, name := range names {
			// A leading country or feed code without a colon: "DE DAZN 2".
			if slices.Equal(words, name) || (len(words) > 1 && len([]rune(words[0])) <= 3 && slices.Equal(words[1:], name)) {
				return true
			}
		}
	}
	return false
}

// livetvWithoutCode drops a short "LAT (AR):" or "MX:" prefix.
func livetvWithoutCode(value string) string {
	if i := strings.Index(value, ":"); i > 0 && i <= 10 {
		return value[i+1:]
	}
	return value
}

// livetvQualityWords are stream quality tags channel names and placeholder
// titles carry.
var livetvQualityWords = map[string]bool{
	"hd": true, "fhd": true, "uhd": true, "sd": true, "4k": true, "hevc": true, "lb": true, "hb": true,
	"1080p": true, "720p": true, "ᴴᴰ": true, "ᶠᴴᴰ": true, "ᵁᴴᴰ": true, "ˢᴰ": true,
}

// livetvNameWords is livetvWords without quality tags.
func livetvNameWords(value string) []string {
	words := livetvWords(value)
	return slices.DeleteFunc(words, func(word string) bool { return livetvQualityWords[word] })
}

// livetvPreviewTitle reports a slate announcing a later game, such as a team
// channel's "Next Game: Brewers @ Dodgers on 2026-10-14".
func livetvPreviewTitle(title string) bool {
	normalized := strings.Join(livetvWords(title), " ")
	for _, prefix := range []string{"next game", "next match", "up next", "coming up", "upcoming"} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

// livetvRadioChannel reports an audio feed, such as a team's radio call.
func livetvRadioChannel(channel model.Channel) bool {
	for _, word := range livetvWords(channel.Name + " " + channel.CategoryName) {
		if word == "radio" {
			return true
		}
	}
	return false
}

// livetvMatchupTitle reports a title naming two sides: "Bruins at Sharks",
// "Warriors vs. Lakers", "Australia v New Zealand".
func livetvMatchupTitle(title string) bool {
	if strings.Contains(title, "@") {
		return true
	}
	normalized := " " + strings.Join(livetvWords(title), " ") + " "
	for _, marker := range []string{" vs ", " v ", " at ", " versus ", " contre ", " gegen "} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

// livetvLiveTitle reports a title marked live, including the superscript
// "ᴸᶦᵛᵉ" some guides append.
func livetvLiveTitle(title string) bool {
	if strings.Contains(title, "ᴸᶦᵛᵉ") {
		return true
	}
	normalized := " " + strings.Join(livetvWords(title), " ") + " "
	for _, marker := range []string{" live ", " en vivo ", " en direct "} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

// livetvRowTier ranks one classified candidate for a row.
func livetvRowTier(candidate livetvCandidate, channel model.Channel, repeated bool) int {
	title := candidate.program.Title
	switch {
	case livetvRadioChannel(channel), candidate.placeholder && candidate.slot == nil, livetvPreviewTitle(title):
		return livetvTierLow
	case candidate.slot != nil:
		return livetvTierFeatured
	case candidate.flags.IsSports && livetvMatchupTitle(title):
		return livetvTierMatchup
	case repeated:
		return livetvTierLow
	case candidate.flags.IsSports && (candidate.flags.IsLive || livetvLiveTitle(title)):
		return livetvTierLive
	}
	return livetvTierNormal
}

// livetvRepeatedTitles returns the titles at least livetvRepeatedChannels
// channels carry among candidates.
func livetvRepeatedTitles(candidates []livetvCandidate) map[string]bool {
	channels := map[string]map[string]bool{}
	for _, candidate := range candidates {
		title := strings.Join(livetvWords(candidate.program.Title), " ")
		if channels[title] == nil {
			channels[title] = map[string]bool{}
		}
		channels[title][candidate.program.ChannelID] = true
	}
	repeated := map[string]bool{}
	for title, set := range channels {
		if len(set) >= livetvRepeatedChannels {
			repeated[title] = true
		}
	}
	return repeated
}
