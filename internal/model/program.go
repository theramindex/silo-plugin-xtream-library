package model

import (
	"strconv"

	"github.com/theramindex/silo-plugins/pkg/sportsguide"
)

type ProgramIdentity struct {
	UpstreamID string
	ChannelID  string
	Title      string
	StartUnix  int64
}

type Program struct {
	Generated  *sportsguide.Provenance `json:"generated,omitempty"`
	ID         string                  `json:"id"`
	ChannelID  string                  `json:"channelId"`
	Title      string                  `json:"title"`
	Summary    string                  `json:"summary,omitempty"`
	ImageURL   string                  `json:"imageUrl,omitempty"`
	Categories []string                `json:"categories,omitempty"`
	StartUnix  int64                   `json:"startUnix"`
	EndUnix    int64                   `json:"endUnix"`
}

func StableProgramID(identity ProgramIdentity) string {
	if normalize(identity.UpstreamID) != "" {
		return "program:" + normalize(identity.UpstreamID)
	}

	return "program:" + stableHash(identity.ChannelID+"|"+normalize(identity.Title)+"|"+strconv.FormatInt(identity.StartUnix, 10))
}
