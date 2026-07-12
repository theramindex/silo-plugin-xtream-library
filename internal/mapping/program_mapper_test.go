package mapping

import (
	"testing"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xtream"
)

func TestMapXtreamProgramProducesStableProgramModel(t *testing.T) {
	t.Parallel()

	program := MapXtreamProgram("xtream:1001", xtream.EPGListing{ID: "epg-1", Title: "Morning News", Description: "Top headlines.", StartTimestamp: "1700000000", StopTimestamp: "1700003600"})
	if program.ID != "program:epg-1" {
		t.Fatalf("expected program id, got %q", program.ID)
	}
	if program.ChannelID != "xtream:1001" || program.Title != "Morning News" || program.Summary != "Top headlines." {
		t.Fatalf("unexpected program mapping: %+v", program)
	}
	if program.StartUnix != 1700000000 || program.EndUnix != 1700003600 {
		t.Fatalf("unexpected program timing: %+v", program)
	}
}
