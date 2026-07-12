package model

import "testing"

func TestStableProgramIDUsesUpstreamProgramIDWhenAvailable(t *testing.T) {
	t.Parallel()

	id := StableProgramID(ProgramIdentity{UpstreamID: "epg-1", ChannelID: "xtream:12345", Title: "Morning News", StartUnix: 1_700_000_000})
	if id != "program:epg-1" {
		t.Fatalf("expected upstream program id, got %q", id)
	}
}

func TestStableProgramIDFallsBackDeterministically(t *testing.T) {
	t.Parallel()

	identity := ProgramIdentity{ChannelID: "xtream:12345", Title: "Morning News", StartUnix: 1_700_000_000}
	first := StableProgramID(identity)
	second := StableProgramID(identity)

	if first == "" {
		t.Fatal("expected stable program id")
	}
	if first != second {
		t.Fatalf("expected deterministic id, got %q and %q", first, second)
	}
}

func TestLiveTVSourceIDIsStable(t *testing.T) {
	t.Parallel()

	if LiveTVSourceID != "source:live-tv" {
		t.Fatalf("unexpected live tv source id %q", LiveTVSourceID)
	}
}
