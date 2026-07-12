package mapping

import (
	"strconv"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xtream"
)

func MapLiveCategory(category xtream.LiveCategory) model.Category {
	return model.Category{ID: category.CategoryID, Name: category.CategoryName, Kind: "live"}
}

func MapVODCategory(category xtream.VODCategory) model.Category {
	return model.Category{ID: category.CategoryID, Name: category.CategoryName, Kind: "vod"}
}

func MapSeriesCategory(category xtream.SeriesCategory) model.Category {
	return model.Category{ID: category.CategoryID, Name: category.CategoryName, Kind: "series"}
}

func MapVODItem(stream xtream.VODStream) model.VODItem {
	return model.VODItem{
		ID:         "vod:" + strconv.FormatInt(stream.StreamID, 10),
		Name:       stream.Name,
		CategoryID: stream.CategoryID,
		PosterURL:  stream.StreamIcon,
		Rating:     stream.Rating,
		Added:      stream.Added,
		Container:  stream.ContainerExtension,
		StreamURL:  stream.DirectSource,
	}
}

func MapSeriesItem(series xtream.Series) model.SeriesItem {
	return model.SeriesItem{
		ID:          "series:" + strconv.FormatInt(series.SeriesID, 10),
		Name:        series.Name,
		CategoryID:  series.CategoryID,
		PosterURL:   series.Cover,
		Rating:      series.Rating,
		ReleaseDate: series.ReleaseDate,
		Description: series.Plot,
	}
}
