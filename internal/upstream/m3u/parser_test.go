package m3u

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePlaylistExtractsChannels(t *testing.T) {
	t.Parallel()

	data := readFixture(t, "sample.m3u")
	entries, err := Parse(data)
	if err != nil {
		t.Fatalf("parse playlist: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].GuideID != "news.hd" || entries[0].Name != "News HD" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "m3u", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
