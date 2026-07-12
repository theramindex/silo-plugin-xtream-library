package matching

import (
	"strings"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/m3u"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xmltv"
)

type Index struct {
	byID   map[string]xmltv.Channel
	byName map[string]xmltv.Channel
}

func NewIndex(doc xmltv.Document) *Index {
	index := &Index{
		byID:   make(map[string]xmltv.Channel, len(doc.Channels)),
		byName: map[string]xmltv.Channel{},
	}
	for _, channel := range doc.Channels {
		if key := normalize(channel.ID); key != "" {
			index.byID[key] = channel
		}
		for _, displayName := range channel.DisplayNames {
			if key := normalize(displayName); key != "" {
				if _, exists := index.byName[key]; !exists {
					index.byName[key] = channel
				}
			}
		}
	}
	return index
}

func (i *Index) Match(entry m3u.Entry) (xmltv.Channel, bool) {
	if i == nil {
		return xmltv.Channel{}, false
	}
	if key := normalize(entry.GuideID); key != "" {
		if channel, ok := i.byID[key]; ok {
			return channel, true
		}
	}
	channel, ok := i.byName[normalize(entry.Name)]
	return channel, ok
}

func Match(entry m3u.Entry, doc xmltv.Document) (xmltv.Channel, bool) {
	return NewIndex(doc).Match(entry)
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
