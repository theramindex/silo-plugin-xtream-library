package xmltv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseXMLTVExtractsChannelsAndProgrammes(t *testing.T) {
	t.Parallel()

	data := readFixture(t, "sample.xml")
	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("parse xmltv: %v", err)
	}
	if len(doc.Channels) != 1 || len(doc.Programmes) != 1 {
		t.Fatalf("unexpected parsed document: %+v", doc)
	}
	if doc.Channels[0].ID != "news.hd" || doc.Programmes[0].Channel != "news.hd" {
		t.Fatalf("unexpected parsed values: %+v %+v", doc.Channels[0], doc.Programmes[0])
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "xmltv", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
