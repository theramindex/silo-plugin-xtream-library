package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const DefaultSourceRegistryFile = "/var/lib/continuum/plugins/silo.ramindex.xtream/sources.json"

var sourceIDPattern = regexp.MustCompile(`[^a-z0-9]+`)

type SourceRegistry struct {
	mu   sync.RWMutex
	path string
}

func NewSourceRegistry(path string) *SourceRegistry {
	if strings.TrimSpace(path) == "" {
		path = DefaultSourceRegistryFile
	}
	return &SourceRegistry{path: path}
}

func NormalizeSourceID(value string) string {
	value = sourceIDPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-")
	return strings.Trim(value, "-")
}

func NormalizeXtreamSource(source XtreamSource) (XtreamSource, error) {
	source.ID = NormalizeSourceID(source.ID)
	source.Name = strings.TrimSpace(source.Name)
	source.BaseURL = strings.TrimRight(strings.TrimSpace(source.BaseURL), "/")
	source.Username = strings.TrimSpace(source.Username)
	source.LiveFormat = source.EffectiveLiveFormat()
	if source.ID == "" || source.Name == "" || source.BaseURL == "" || source.Username == "" || source.Password == "" {
		return XtreamSource{}, fmt.Errorf("source id, name, server, username, and password are required")
	}
	return source, nil
}

func (r *SourceRegistry) Load() ([]XtreamSource, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	data, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read source registry: %w", err)
	}
	var sources []XtreamSource
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, fmt.Errorf("decode source registry: %w", err)
	}
	return sources, nil
}

func (r *SourceRegistry) Save(sources []XtreamSource) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	normalized := make([]XtreamSource, 0, len(sources))
	seen := map[string]bool{}
	for _, source := range sources {
		value, err := NormalizeXtreamSource(source)
		if err != nil {
			return err
		}
		if seen[value.ID] {
			return fmt.Errorf("source id %q is duplicated", value.ID)
		}
		seen[value.ID] = true
		normalized = append(normalized, value)
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("encode source registry: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return fmt.Errorf("create source registry directory: %w", err)
	}
	temporary := r.path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write source registry: %w", err)
	}
	if err := os.Rename(temporary, r.path); err != nil {
		return fmt.Errorf("replace source registry: %w", err)
	}
	return nil
}
