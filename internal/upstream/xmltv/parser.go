package xmltv

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
)

const maxDecodedXMLTVBytes = 256 << 20

type Document struct {
	XMLName    xml.Name    `xml:"tv"`
	Channels   []Channel   `xml:"channel"`
	Programmes []Programme `xml:"programme"`
}

type Channel struct {
	ID           string   `xml:"id,attr"`
	DisplayNames []string `xml:"display-name"`
}

type Programme struct {
	Channel string `xml:"channel,attr"`
	Start   string `xml:"start,attr"`
	Stop    string `xml:"stop,attr"`
	Title   string `xml:"title"`
	Desc    string `xml:"desc"`
}

func Parse(data []byte) (Document, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return Document{}, fmt.Errorf("open gzip XMLTV: %w", err)
		}
		decompressed, err := io.ReadAll(io.LimitReader(reader, maxDecodedXMLTVBytes+1))
		closeErr := reader.Close()
		if err != nil {
			return Document{}, fmt.Errorf("decompress XMLTV: %w", err)
		}
		if closeErr != nil {
			return Document{}, fmt.Errorf("close gzip XMLTV: %w", closeErr)
		}
		if len(decompressed) > maxDecodedXMLTVBytes {
			return Document{}, fmt.Errorf("decompressed XMLTV exceeds %d bytes", maxDecodedXMLTVBytes)
		}
		data = decompressed
	}
	var doc Document
	err := xml.Unmarshal(data, &doc)
	return doc, err
}
