package xtream

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type LiveCategory struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
}

type VODCategory struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
}

type SeriesCategory struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
}

type LiveStream struct {
	Num                      int    `json:"num"`
	Name                     string `json:"name"`
	StreamType               string `json:"stream_type"`
	StreamID                 int64  `json:"stream_id"`
	StreamIcon               string `json:"stream_icon"`
	EPGChannelID             string `json:"epg_channel_id"`
	CategoryID               string `json:"category_id"`
	TVArchive                int    `json:"tv_archive"`
	TVArchiveDurationMinutes int    `json:"tv_archive_duration"`
}

func (stream *LiveStream) UnmarshalJSON(data []byte) error {
	type alias LiveStream
	decoded := struct {
		*alias
		ArchiveDuration json.RawMessage `json:"tv_archive_duration"`
	}{alias: (*alias)(stream)}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if len(decoded.ArchiveDuration) == 0 || string(decoded.ArchiveDuration) == "null" {
		return nil
	}
	var minutes int
	if err := json.Unmarshal(decoded.ArchiveDuration, &minutes); err == nil {
		stream.TVArchiveDurationMinutes = minutes
		return nil
	}
	var text string
	if err := json.Unmarshal(decoded.ArchiveDuration, &text); err != nil {
		return fmt.Errorf("decode tv_archive_duration: %w", err)
	}
	minutes, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return fmt.Errorf("decode tv_archive_duration %q: %w", text, err)
	}
	stream.TVArchiveDurationMinutes = minutes
	return nil
}

func (stream LiveStream) CatchupAvailable() bool {
	return stream.TVArchive == 1 && stream.TVArchiveDurationMinutes > 0
}

type VODStream struct {
	Num                int    `json:"num"`
	Name               string `json:"name"`
	StreamType         string `json:"stream_type"`
	StreamID           int64  `json:"stream_id"`
	StreamIcon         string `json:"stream_icon"`
	Rating             string `json:"rating"`
	Added              string `json:"added"`
	CategoryID         string `json:"category_id"`
	ContainerExtension string `json:"container_extension"`
	DirectSource       string `json:"direct_source"`
}

type Series struct {
	Num         int    `json:"num"`
	Name        string `json:"name"`
	SeriesID    int64  `json:"series_id"`
	Cover       string `json:"cover"`
	Plot        string `json:"plot"`
	ReleaseDate string `json:"releaseDate"`
	Rating      string `json:"rating"`
	CategoryID  string `json:"category_id"`
}

type SeriesInfo struct {
	Info     SeriesInfoMetadata
	Episodes []EpisodeInfo
}

type SeriesInfoMetadata struct {
	Name string `json:"name"`
}

type EpisodeInfo struct {
	ID                 int64  `json:"id,string"`
	EpisodeNumber      int    `json:"episode_num"`
	Title              string `json:"title"`
	ContainerExtension string `json:"container_extension"`
	SeasonNumber       int    `json:"-"`
}

type EPGListing struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	StartTimestamp string `json:"start_timestamp"`
	StopTimestamp  string `json:"stop_timestamp"`
}

type ShortEPGResponse struct {
	EPGListings []EPGListing `json:"epg_listings"`
}
