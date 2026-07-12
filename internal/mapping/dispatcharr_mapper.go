package mapping

import (
	"strings"
	"time"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/dispatcharr"
)

func MapDispatcharrChannel(channel dispatcharr.Channel, streamURL string) model.Channel {
	name := firstNonEmpty(channel.EffectiveName.String(), channel.Name.String())
	number := firstNonEmpty(channel.EffectiveChannelNumber.String(), channel.ChannelNumber.String())
	guideID := firstNonEmpty(channel.EffectiveTVGID.String(), channel.TVGID.String(), channel.UUID.String())
	categoryID := channel.EffectiveGroupID.String()

	return model.Channel{
		ID: model.StableChannelID(model.SourceModeDirectLogin, model.ChannelIdentity{
			UpstreamID: channel.UUID.String(),
			GuideID:    guideID,
			Name:       name,
			LogoURL:    channel.LogoURL.String(),
			StreamURL:  streamURL,
		}),
		SourceID:   model.LiveTVSourceID,
		Name:       name,
		Number:     number,
		GuideID:    guideID,
		LogoURL:    channel.LogoURL.String(),
		StreamURL:  streamURL,
		CategoryID: categoryID,
	}
}

func MapDispatcharrCategory(category dispatcharr.ChannelGroup) model.Category {
	return model.Category{ID: category.ID.String(), Name: category.Name.String(), Kind: "live"}
}

func MapDispatcharrVODCategory(category dispatcharr.VODCategory) model.Category {
	kind := "vod"
	if strings.EqualFold(category.CategoryType.String(), "series") {
		kind = "series"
	}
	return model.Category{ID: category.ID.String(), Name: category.Name.String(), Kind: kind}
}

func MapDispatcharrProgram(channelID string, program dispatcharr.Program) model.Program {
	startUnix := parseDispatcharrTime(program.StartTime.String())
	title := firstNonEmpty(program.Title.String(), program.SubTitle.String(), "Data not available")
	return model.Program{
		ID: model.StableProgramID(model.ProgramIdentity{
			UpstreamID: program.ID.String(),
			ChannelID:  channelID,
			Title:      title,
			StartUnix:  startUnix,
		}),
		ChannelID: channelID,
		Title:     title,
		Summary:   program.Description.String(),
		StartUnix: startUnix,
		EndUnix:   parseDispatcharrTime(program.EndTime.String()),
	}
}

func MapDispatcharrMovie(movie dispatcharr.Movie, streamURL string) model.VODItem {
	return model.VODItem{
		ID:          "movie:" + firstNonEmpty(movie.UUID.String(), movie.ID.String()),
		Name:        movie.Name.String(),
		CategoryID:  movie.CategoryID.String(),
		PosterURL:   firstNonEmpty(movie.Logo.CacheURL.String(), movie.Logo.URL.String()),
		Rating:      movie.Rating.String(),
		StreamURL:   streamURL,
		Description: movie.Description.String(),
	}
}

func MapDispatcharrSeries(series dispatcharr.Series, streamURL string) model.SeriesItem {
	return model.SeriesItem{
		ID:          "series:" + firstNonEmpty(series.UUID.String(), series.ID.String()),
		Name:        series.Name.String(),
		CategoryID:  series.CategoryID.String(),
		PosterURL:   firstNonEmpty(series.Logo.CacheURL.String(), series.Logo.URL.String()),
		Rating:      series.Rating.String(),
		ReleaseDate: series.Year.String(),
		Description: series.Description.String(),
	}
}

func parseDispatcharrTime(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Unix()
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
