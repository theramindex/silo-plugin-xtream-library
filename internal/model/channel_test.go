package model

import "testing"

func TestStableChannelIDXtreamUsesUpstreamID(t *testing.T) {
	t.Parallel()

	id := StableChannelID(SourceModeXtream, ChannelIdentity{UpstreamID: "12345", Name: "News HD", StreamURL: "https://example.com/live/12345"})
	if id != "xtream:12345" {
		t.Fatalf("expected xtream upstream id to be preserved, got %q", id)
	}
}

func TestStableChannelIDM3UDeterministicFallback(t *testing.T) {
	t.Parallel()

	identity := ChannelIdentity{Name: "Sports 1", StreamURL: "https://example.com/live/sports1.m3u8", LogoURL: "https://example.com/logo.png"}
	first := StableChannelID(SourceModeM3UXMLTV, identity)
	second := StableChannelID(SourceModeM3UXMLTV, identity)

	if first == "" {
		t.Fatal("expected stable id")
	}
	if first != second {
		t.Fatalf("expected deterministic id, got %q and %q", first, second)
	}
}

func TestStableChannelIDPrefersGuideIdentifier(t *testing.T) {
	t.Parallel()

	id := StableChannelID(SourceModeM3UXMLTV, ChannelIdentity{GuideID: "tvg-sports-1", Name: "Sports 1"})
	if id != "m3u:tvg-sports-1" {
		t.Fatalf("expected guide id to win, got %q", id)
	}
}
