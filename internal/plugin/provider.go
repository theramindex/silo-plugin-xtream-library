package plugin

import "github.com/theramindex/silo-plugin-xtream-library/internal/model"

type Descriptor struct {
	SourceID string
	Name     string
}

func ProviderDescriptor() Descriptor {
	return Descriptor{SourceID: model.LiveTVSourceID, Name: "Live TV"}
}
