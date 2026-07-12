package xmltv

import "encoding/xml"

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
	var doc Document
	err := xml.Unmarshal(data, &doc)
	return doc, err
}
