package plugin

import "encoding/json"

func (s *HTTPRoutesServer) adminFlag(key string, fallback bool) bool {
	if s == nil || s.store == nil {
		return fallback
	}
	if !s.store.HasAdminSettings() && s.adminStorage != nil {
		if saved, ok, err := s.adminStorage.Load(); err == nil && ok {
			s.store.SetAdminSettings(saved)
		}
	}
	settings := map[string]any{}
	_ = json.Unmarshal(s.store.AdminSettings(), &settings)
	if value, ok := settings[key].(bool); ok {
		return value
	}
	return fallback
}

func (s *HTTPRoutesServer) sportsFeatureEnabled() bool {
	return s.adminFlag("sportsEnabled", false)
}
