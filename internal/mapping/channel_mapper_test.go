package mapping

import (
	"testing"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xtream"
)

func TestMapXtreamChannelPreservesSourceOfTruthFields(t *testing.T) {
	t.Parallel()

	channel := MapXtreamChannel(xtream.LiveStream{Num: 7, Name: "News HD", StreamID: 1001, StreamIcon: "https://example.com/news.png", EPGChannelID: "news.hd"})
	if channel.ID != "xtream:1001" {
		t.Fatalf("expected stable xtream id, got %q", channel.ID)
	}
	if channel.Name != "News HD" || channel.Number != "7" || channel.LogoURL != "https://example.com/news.png" || channel.GuideID != "news.hd" {
		t.Fatalf("unexpected channel mapping: %+v", channel)
	}
	if channel.SourceID != model.LiveTVSourceID {
		t.Fatalf("expected live tv source id, got %q", channel.SourceID)
	}
}
