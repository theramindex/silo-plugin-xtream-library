package xmltv

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestParseXMLTVAcceptsGzipFeeds(t *testing.T) {
	t.Parallel()
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(`<tv><channel id="news"><display-name>News</display-name></channel></tv>`)); err != nil {
		t.Fatalf("compress fixture: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	doc, err := Parse(compressed.Bytes())
	if err != nil || len(doc.Channels) != 1 || doc.Channels[0].ID != "news" {
		t.Fatalf("parse gzip XMLTV: doc=%+v err=%v", doc, err)
	}
}

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
