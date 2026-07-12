package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
)

func TestFileSnapshotStorageRoundTripsCatalogSnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "catalog-snapshot.json")
	storage := NewFileSnapshotStorage(path)
	snapshot := Snapshot{
		ConfigKey: "config:123",
		Catalog: model.CatalogState{
			Source:   model.LiveTVSource(model.SourceModeDirectLogin),
			Channels: []model.Channel{{ID: "channel:1", Name: "News"}},
			Programs: []model.Program{{ID: "program:1", ChannelID: "channel:1", Title: "Morning News"}},
			Health:   model.SyncHealth{LastSuccessUnix: 100},
		},
		Health: model.SyncHealth{LastSuccessUnix: 100},
	}

	if err := storage.Save(snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	loaded, ok, err := storage.Load()
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected saved snapshot to load")
	}
	if loaded.ConfigKey != snapshot.ConfigKey {
		t.Fatalf("expected config key %q, got %q", snapshot.ConfigKey, loaded.ConfigKey)
	}
	if len(loaded.Catalog.Channels) != 1 || loaded.Catalog.Channels[0].ID != "channel:1" {
		t.Fatalf("unexpected loaded channels: %+v", loaded.Catalog.Channels)
	}
	if len(loaded.Catalog.Programs) != 1 || loaded.Catalog.Programs[0].ID != "program:1" {
		t.Fatalf("unexpected loaded programs: %+v", loaded.Catalog.Programs)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected private snapshot permissions 0600, got %04o", got)
	}
}

func TestFileSnapshotStorageSkipsEmptySnapshots(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "catalog-snapshot.json")
	storage := NewFileSnapshotStorage(path)
	if err := storage.Save(Snapshot{}); err != nil {
		t.Fatalf("save empty snapshot: %v", err)
	}
	_, ok, err := storage.Load()
	if err != nil {
		t.Fatalf("load empty snapshot: %v", err)
	}
	if ok {
		t.Fatal("expected empty snapshot not to be persisted")
	}
}
