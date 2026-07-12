package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceRegistryRoundTripsCredentialsPrivately(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sources.json")
	registry := NewSourceRegistry(path)
	sources := []XtreamSource{{ID: " Backup Source ", Name: "Backup", BaseURL: "https://provider.example/", Username: "demo", Password: "secret", Enabled: true}}
	if err := registry.Save(sources); err != nil {
		t.Fatalf("save sources: %v", err)
	}
	loaded, err := registry.Load()
	if err != nil {
		t.Fatalf("load sources: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ID != "backup-source" || loaded[0].Password != "secret" || loaded[0].LiveFormat != "m3u8" {
		t.Fatalf("unexpected source: %+v", loaded)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat source registry: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected private registry permissions, got %o", info.Mode().Perm())
	}
}
