package matching

import (
	"testing"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/m3u"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xmltv"
)

func TestMatchGuideIDPreferred(t *testing.T) {
	t.Parallel()

	entry := m3u.Entry{GuideID: "news.hd", Name: "News HD"}
	doc := xmltv.Document{Channels: []xmltv.Channel{{ID: "news.hd", DisplayNames: []string{"News HD"}}}}
	match, ok := Match(entry, doc)
	if !ok {
		t.Fatal("expected match")
	}
	if match.ID != "news.hd" {
		t.Fatalf("expected guide id match, got %+v", match)
	}
}
