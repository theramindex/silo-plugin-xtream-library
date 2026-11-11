package plugin

import (
	"strings"

	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

// Source ownership belongs in IDs and configuration. Strip only the configured
// source's generated label; a provider category can legitimately contain a dot.
func (s *HTTPRoutesServer) sourceCategoryPrefixes() map[string]string {
	prefixes := make(map[string]string)
	if s.settingsProvider != nil {
		for _, source := range s.settingsProvider().EffectiveXtreamSources() {
			if source.Name != "" {
				prefixes[source.ID] = source.Name + " · "
			}
		}
	}
	return prefixes
}

func categoryNameWithoutSource(id, name string, prefixes map[string]string) string {
	sourceID, _, namespaced := strings.Cut(id, ":")
	if namespaced && prefixes[sourceID] != "" {
		return strings.TrimPrefix(name, prefixes[sourceID])
	}
	return name
}

func (s *HTTPRoutesServer) publicCategories(categories []model.Category) []model.Category {
	prefixes := s.sourceCategoryPrefixes()
	result := make([]model.Category, len(categories))
	for index, category := range categories {
		category.Name = categoryNameWithoutSource(category.ID, category.Name, prefixes)
		result[index] = category
	}
	return result
}
