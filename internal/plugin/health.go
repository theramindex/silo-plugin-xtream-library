package plugin

import (
	"github.com/theramindex/silo-plugin-dispatcharr/internal/cache"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
)

type HealthPayload struct {
	Status             string               `json:"status"`
	SourceID           string               `json:"sourceId"`
	SourceName         string               `json:"sourceName"`
	ChannelCount       int                  `json:"channelCount"`
	ProgramCount       int                  `json:"programCount"`
	LastSuccessUnix    int64                `json:"lastSuccessUnix"`
	LastFailureUnix    int64                `json:"lastFailureUnix"`
	LastError          string               `json:"lastError,omitempty"`
	EPGStatus          string               `json:"epgStatus,omitempty"`
	EPGProgramCount    int                  `json:"epgProgramCount,omitempty"`
	EPGLastSuccessUnix int64                `json:"epgLastSuccessUnix,omitempty"`
	EPGLastFailureUnix int64                `json:"epgLastFailureUnix,omitempty"`
	EPGLastError       string               `json:"epgLastError,omitempty"`
	ProfileAccess      *model.ProfileAccess `json:"profileAccess,omitempty"`
	Refresh            RefreshJob           `json:"refresh"`
}

func BuildHealthPayload(snapshot cache.Snapshot) HealthPayload {
	status := "ok"
	if snapshot.Health.LastError != "" {
		status = "error"
	}
	return HealthPayload{
		Status:             status,
		SourceID:           snapshot.Catalog.Source.ID,
		SourceName:         snapshot.Catalog.Source.Name,
		ChannelCount:       len(snapshot.Catalog.Channels),
		ProgramCount:       len(snapshot.Catalog.Programs),
		LastSuccessUnix:    snapshot.Health.LastSuccessUnix,
		LastFailureUnix:    snapshot.Health.LastFailureUnix,
		LastError:          snapshot.Health.LastError,
		EPGStatus:          snapshot.Health.EPGStatus,
		EPGProgramCount:    snapshot.Health.EPGProgramCount,
		EPGLastSuccessUnix: snapshot.Health.EPGLastSuccessUnix,
		EPGLastFailureUnix: snapshot.Health.EPGLastFailureUnix,
		EPGLastError:       snapshot.Health.EPGLastError,
		ProfileAccess:      snapshot.Catalog.Source.ProfileAccess,
	}
}
