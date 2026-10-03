package model

type SyncHealth struct {
	LastSuccessUnix    int64  `json:"lastSuccessUnix"`
	LastFailureUnix    int64  `json:"lastFailureUnix"`
	LastError          string `json:"lastError,omitempty"`
	EPGStatus          string `json:"epgStatus,omitempty"`
	EPGProgramCount    int    `json:"epgProgramCount,omitempty"`
	EPGLastSuccessUnix int64  `json:"epgLastSuccessUnix,omitempty"`
	EPGLastFailureUnix int64  `json:"epgLastFailureUnix,omitempty"`
	EPGLastError       string `json:"epgLastError,omitempty"`
	// EPGWarning explains why a refresh result was not applied (for example
	// the last known good guide was kept after a sharp drop in programs).
	EPGWarning string `json:"epgWarning,omitempty"`
}

type CatalogState struct {
	Source   Source       `json:"source"`
	Channels []Channel    `json:"channels"`
	Programs []Program    `json:"programs"`
	Health   SyncHealth   `json:"health"`
	Content  ContentState `json:"content"`
}
