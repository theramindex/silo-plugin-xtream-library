package m3u

import (
	"bytes"
	"strings"
)

type Entry struct {
	GuideID   string
	Name      string
	LogoURL   string
	StreamURL string
}

func Parse(data []byte) ([]Entry, error) {
	lines := strings.Split(strings.ReplaceAll(string(bytes.TrimSpace(data)), "\r\n", "\n"), "\n")
	entries := make([]Entry, 0)
	var current Entry
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || line == "#EXTM3U" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			current = parseEXTINF(line)
			continue
		}
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			current.StreamURL = line
			entries = append(entries, current)
			current = Entry{}
		}
	}
	return entries, nil
}

func parseEXTINF(line string) Entry {
	entry := Entry{}
	if idx := strings.Index(line, "tvg-id="); idx >= 0 {
		entry.GuideID = quotedValue(line[idx+7:])
	}
	if idx := strings.Index(line, "tvg-logo="); idx >= 0 {
		entry.LogoURL = quotedValue(line[idx+9:])
	}
	if idx := strings.LastIndex(line, ","); idx >= 0 {
		entry.Name = strings.TrimSpace(line[idx+1:])
	}
	return entry
}

func quotedValue(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, `"`) {
		return ""
	}
	value = value[1:]
	if idx := strings.Index(value, `"`); idx >= 0 {
		return value[:idx]
	}
	return ""
}
