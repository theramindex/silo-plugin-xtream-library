package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSealedSourceRegistryRoundTripsAndHidesSecrets(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	box := NewSecretBox(filepath.Join(dir, ".key"))
	registry := NewSealedSourceRegistry(filepath.Join(dir, "sources.sealed"), box)
	source := XtreamSource{ID: "p", Name: "P", BaseURL: "https://p.example", Username: "user", Password: "hunter2", Enabled: true}
	if err := registry.Save([]XtreamSource{source}); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sources.sealed"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(raw), "hunter2") || strings.Contains(string(raw), "p.example") {
		t.Fatalf("sealed file holds plaintext: %s", raw)
	}
	info, err := os.Stat(filepath.Join(dir, ".key"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key file must be 0600, got %v %v", info, err)
	}

	// A fresh box over the same key file opens it (plugin restart).
	reopened := NewSealedSourceRegistry(filepath.Join(dir, "sources.sealed"), NewSecretBox(filepath.Join(dir, ".key")))
	sources, err := reopened.Load()
	if err != nil || len(sources) != 1 || sources[0].Password != "hunter2" {
		t.Fatalf("reload: %+v %v", sources, err)
	}

	// A different key cannot.
	other := NewSealedSourceRegistry(filepath.Join(dir, "sources.sealed"), NewSecretBox(filepath.Join(t.TempDir(), ".key")))
	if _, err := other.Load(); err == nil {
		t.Fatal("a different key must not open the registry")
	}
}
