package matching

import (
	"testing"

	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"github.com/theramindex/silo-plugin-xtream-library/internal/upstream/xmltv"
)

func TestMatchAlternateEPGChannelsUsesGuideIDThenUniqueNormalizedName(t *testing.T) {
	t.Parallel()
	channels := []model.Channel{
		{ID: "xtream:1", Name: "US | News HD", GuideID: "provider.news"},
		{ID: "xtream:2", Name: "US | Movie Network FHD"},
		{ID: "xtream:3", Name: "Duplicate HD"},
	}
	doc := xmltv.Document{Channels: []xmltv.Channel{
		{ID: "PROVIDER.NEWS", DisplayNames: []string{"Different name"}},
		{ID: "movie.us", DisplayNames: []string{"Movie Network"}},
		{ID: "duplicate.one", DisplayNames: []string{"Duplicate"}},
		{ID: "duplicate.two", DisplayNames: []string{"Duplicate HD"}},
	}}

	result := MatchAlternateEPGChannels(channels, doc)
	if ids := result.ChannelIDsByXMLTVID["provider.news"]; len(ids) != 1 || ids[0] != "xtream:1" {
		t.Fatalf("expected case-insensitive guide ID match, got %+v", result)
	}
	if ids := result.ChannelIDsByXMLTVID["movie.us"]; len(ids) != 1 || ids[0] != "xtream:2" {
		t.Fatalf("expected safe normalized-name match, got %+v", result)
	}
	if _, matched := result.ChannelIDsByXMLTVID["duplicate.one"]; matched {
		t.Fatalf("ambiguous normalized names must remain unmatched: %+v", result)
	}
	if _, matched := result.ChannelIDsByXMLTVID["duplicate.two"]; matched {
		t.Fatalf("ambiguous normalized names must remain unmatched: %+v", result)
	}
	if result.MatchedChannels != 2 || result.UnmatchedChannels != 1 {
		t.Fatalf("unexpected coverage: %+v", result)
	}
}

func TestMatchAlternateEPGChannelsAllowsSharedExactGuideIDs(t *testing.T) {
	t.Parallel()
	channels := []model.Channel{{ID: "xtream:1", GuideID: "news.us"}, {ID: "xtream:2", GuideID: "news.us"}}
	doc := xmltv.Document{Programmes: []xmltv.Programme{{Channel: "news.us"}}}
	result := MatchAlternateEPGChannels(channels, doc)
	ids := result.ChannelIDsByXMLTVID["news.us"]
	if len(ids) != 2 || result.MatchedChannels != 2 || result.Coverage(doc).ProgramCount != 2 {
		t.Fatalf("exact guide IDs should map one XMLTV programme to both channels: %+v", result)
	}
}
