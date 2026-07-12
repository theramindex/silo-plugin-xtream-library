package xtream

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
	Num          int    `json:"num"`
	Name         string `json:"name"`
	StreamType   string `json:"stream_type"`
	StreamID     int64  `json:"stream_id"`
	StreamIcon   string `json:"stream_icon"`
	EPGChannelID string `json:"epg_channel_id"`
	CategoryID   string `json:"category_id"`
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
