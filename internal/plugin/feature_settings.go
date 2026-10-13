package plugin

import "encoding/json"

// adminSettingsPayload returns the saved admin settings, loading them from the
// settings file the first time.
func (s *HTTPRoutesServer) adminSettingsPayload() map[string]any {
	settings := map[string]any{}
	if s == nil || s.store == nil {
		return settings
	}
	if !s.store.HasAdminSettings() && s.adminStorage != nil {
		if saved, ok, err := s.adminStorage.Load(); err == nil && ok {
			s.store.SetAdminSettings(saved)
		}
	}
	_ = json.Unmarshal(s.store.AdminSettings(), &settings)
	return settings
}

func (s *HTTPRoutesServer) adminFlag(key string, fallback bool) bool {
	if value, ok := s.adminSettingsPayload()[key].(bool); ok {
		return value
	}
	return fallback
}

func (s *HTTPRoutesServer) sportsFeatureEnabled() bool {
	return s.adminFlag("sportsEnabled", false)
}
