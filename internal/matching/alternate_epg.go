package matching

import (
	"regexp"
	"strings"

	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"github.com/theramindex/silo-plugin-xtream-library/internal/upstream/xmltv"
)

var alternateEPGQualitySuffix = regexp.MustCompile(`(?i)(?:\s|[-_.])+(?:uhd|4k|fhd|fullhd|hd|sd|hevc|h265|h264)$`)
var alternateEPGNonWord = regexp.MustCompile(`[^a-z0-9]+`)

type AlternateEPGMatch struct {
	ChannelIDsByXMLTVID map[string][]string
	MatchedChannels     int
	UnmatchedChannels   int
}

type AlternateEPGCoverage struct {
	MatchedChannels   int `json:"matchedChannels"`
	UnmatchedChannels int `json:"unmatchedChannels"`
	ProgramCount      int `json:"programCount"`
}

func MatchAlternateEPGChannels(channels []model.Channel, doc xmltv.Document) AlternateEPGMatch {
	result := AlternateEPGMatch{ChannelIDsByXMLTVID: map[string][]string{}}
	xmlByID := map[string]string{}
	nameIDs := map[string][]string{}
	for _, channel := range doc.Channels {
		id := strings.ToLower(strings.TrimSpace(channel.ID))
		if id == "" {
			continue
		}
		xmlByID[id] = id
		for _, displayName := range channel.DisplayNames {
			if name := normalizeAlternateEPGName(displayName); name != "" {
				nameIDs[name] = appendUnique(nameIDs[name], id)
			}
		}
	}
	for _, programme := range doc.Programmes {
		if id := strings.ToLower(strings.TrimSpace(programme.Channel)); id != "" {
			xmlByID[id] = id
		}
	}

	localNameCounts := map[string]int{}
	for _, channel := range channels {
		localNameCounts[normalizeAlternateEPGName(channel.Name)]++
	}
	usedXMLTVByName := map[string]bool{}
	matchedLocal := map[string]bool{}
	for _, channel := range channels {
		guideID := strings.ToLower(strings.TrimSpace(channel.GuideID))
		if _, ok := xmlByID[guideID]; ok {
			result.ChannelIDsByXMLTVID[guideID] = appendUnique(result.ChannelIDsByXMLTVID[guideID], channel.ID)
			matchedLocal[channel.ID] = true
		}
	}
	for _, channel := range channels {
		if matchedLocal[channel.ID] {
			continue
		}
		name := normalizeAlternateEPGName(channel.Name)
		ids := nameIDs[name]
		if name == "" || localNameCounts[name] != 1 || len(ids) != 1 || usedXMLTVByName[ids[0]] || len(result.ChannelIDsByXMLTVID[ids[0]]) > 0 {
			continue
		}
		result.ChannelIDsByXMLTVID[ids[0]] = []string{channel.ID}
		usedXMLTVByName[ids[0]] = true
		matchedLocal[channel.ID] = true
	}
	result.MatchedChannels = len(matchedLocal)
	result.UnmatchedChannels = len(channels) - result.MatchedChannels
	return result
}

func (m AlternateEPGMatch) Coverage(doc xmltv.Document) AlternateEPGCoverage {
	programCount := 0
	for _, programme := range doc.Programmes {
		programCount += len(m.ChannelIDsByXMLTVID[strings.ToLower(strings.TrimSpace(programme.Channel))])
	}
	return AlternateEPGCoverage{MatchedChannels: m.MatchedChannels, UnmatchedChannels: m.UnmatchedChannels, ProgramCount: programCount}
}

func normalizeAlternateEPGName(value string) string {
	parts := strings.Split(value, "|")
	value = strings.TrimSpace(parts[len(parts)-1])
	for alternateEPGQualitySuffix.MatchString(value) {
		value = alternateEPGQualitySuffix.ReplaceAllString(value, "")
	}
	value = strings.ToLower(strings.ReplaceAll(value, "&", " and "))
	return strings.Trim(alternateEPGNonWord.ReplaceAllString(value, " "), " ")
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
