package mapping

import (
	"strconv"
	"time"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xmltv"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xtream"
)

func MapXtreamProgram(channelID string, listing xtream.EPGListing) model.Program {
	startUnix, _ := strconv.ParseInt(listing.StartTimestamp, 10, 64)
	endUnix, _ := strconv.ParseInt(listing.StopTimestamp, 10, 64)

	return model.Program{
		ID: model.StableProgramID(model.ProgramIdentity{
			UpstreamID: listing.ID,
			ChannelID:  channelID,
			Title:      listing.Title,
			StartUnix:  startUnix,
		}),
		ChannelID: channelID,
		Title:     listing.Title,
		Summary:   listing.Description,
		StartUnix: startUnix,
		EndUnix:   endUnix,
	}
}

func MapXMLTVProgramme(channelID string, programme xmltv.Programme) model.Program {
	startUnix := parseXMLTVTime(programme.Start)
	endUnix := parseXMLTVTime(programme.Stop)
	return model.Program{
		ID:        model.StableProgramID(model.ProgramIdentity{ChannelID: channelID, Title: programme.Title, StartUnix: startUnix}),
		ChannelID: channelID,
		Title:     programme.Title,
		Summary:   programme.Desc,
		StartUnix: startUnix,
		EndUnix:   endUnix,
	}
}

func parseXMLTVTime(value string) int64 {
	parsed, err := time.Parse("20060102150405 -0700", value)
	if err != nil {
		return 0
	}
	return parsed.Unix()
}
