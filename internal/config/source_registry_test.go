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

func TestDeriveXtreamSourceIDUsesDomainAndUsername(t *testing.T) {
	t.Parallel()
	got := DeriveXtreamSourceID("https://IPTV.Example:8443/player_api.php", " Viewer+One ")
	if got != "iptv-example-8443-viewer-one" {
		t.Fatalf("unexpected derived source id %q", got)
	}
}

func TestNormalizeXtreamSourceDefaultsDisplayNameAndAcceptsSchemeLessIdentity(t *testing.T) {
	t.Parallel()
	source, err := NormalizeXtreamSource(XtreamSource{
		ID:       DeriveXtreamSourceID("provider.example:8080", "viewer"),
		BaseURL:  "provider.example:8080",
		Username: "viewer",
		Password: "secret",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("normalize source: %v", err)
	}
	if source.ID != "provider-example-8080-viewer" || source.Name != "provider.example" {
		t.Fatalf("unexpected defaults: %+v", source)
	}
}

func TestNormalizeXtreamSourceReportsSpecificMissingField(t *testing.T) {
	t.Parallel()
	_, err := NormalizeXtreamSource(XtreamSource{ID: "provider-user", BaseURL: "https://provider.example", Username: "user"})
	if err == nil || err.Error() != "source requires password" {
		t.Fatalf("expected password-specific validation, got %v", err)
	}
}

func TestNormalizeXtreamSourceDefaultsAlternateEPGPolicy(t *testing.T) {
	t.Parallel()
	source, err := NormalizeXtreamSource(XtreamSource{
		ID:                  "provider-user",
		BaseURL:             "https://provider.example",
		Username:            "user",
		Password:            "secret",
		Enabled:             true,
		AlternateEPGEnabled: true,
		AlternateEPGURL:     " https://epg.example/guide.xml ",
	})
	if err != nil {
		t.Fatalf("normalize source: %v", err)
	}
	if source.AlternateEPGURL != "https://epg.example/guide.xml" || source.AlternateEPGPolicy != "fill_missing" {
		t.Fatalf("unexpected alternate EPG defaults: %+v", source)
	}
}

func TestNormalizeXtreamSourceRejectsEnabledAlternateEPGWithoutURL(t *testing.T) {
	t.Parallel()
	_, err := NormalizeXtreamSource(XtreamSource{
		ID:                  "provider-user",
		BaseURL:             "https://provider.example",
		Username:            "user",
		Password:            "secret",
		Enabled:             true,
		AlternateEPGEnabled: true,
	})
	if err == nil || err.Error() != "enabled alternate EPG requires an XMLTV URL" {
		t.Fatalf("expected alternate EPG URL validation, got %v", err)
	}
}
