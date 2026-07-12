package model

type SourceMode string

const (
	SourceModeDirectLogin SourceMode = "direct_login"
	SourceModeAPIKey      SourceMode = "api_key"
	SourceModeXtream      SourceMode = "xtream"
	SourceModeM3UXMLTV    SourceMode = "m3u_xmltv"
	LiveTVSourceID        string     = "source:live-tv"
)

type Source struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Mode           SourceMode       `json:"mode"`
	ChannelProfile *ChannelProfile  `json:"channelProfile,omitempty"`
	Profiles       []ChannelProfile `json:"profiles,omitempty"`
	ProfileAccess  *ProfileAccess   `json:"profileAccess,omitempty"`
}

type ChannelProfile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ChannelCount int    `json:"channelCount"`
}

type ProfileAccess struct {
	Status                 string `json:"status"`
	ProfileCount           int    `json:"profileCount"`
	ChannelMembershipCount int    `json:"channelMembershipCount"`
	Message                string `json:"message,omitempty"`
}

func LiveTVSource(mode SourceMode) Source {
	return Source{ID: LiveTVSourceID, Name: "Live TV", Mode: mode}
}
