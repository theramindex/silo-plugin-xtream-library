package cache

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

func TestLegacySnapshotMigratesOnceWithoutRewritingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog-snapshot.json")
	legacy := Snapshot{ConfigKey: "private-source", Catalog: model.CatalogState{
		Source:   model.LiveTVSource(model.SourceModeXtream),
		Channels: []model.Channel{{ID: "a", Name: "News"}},
		Programs: []model.Program{{ID: "one", ChannelID: "a", Title: "Last good guide", StartUnix: 10, EndUnix: 30}},
	}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	storage := NewFileSnapshotStorage(path)
	defer storage.Close()
	loaded, found, err := storage.Load()
	if err != nil || !found {
		t.Fatalf("legacy restore: %v %v", found, err)
	}
	saved, err := storage.Save(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Guide == nil || saved.ProgramCount() != 1 || len(saved.Catalog.Programs) != 0 {
		t.Fatal("migration did not release the legacy programme slice")
	}
	original, err := os.ReadFile(path)
	if err != nil || string(original) != string(data) {
		t.Fatal("migration overwrote the rollback copy")
	}
	if err = os.WriteFile(path, []byte(`invalid leftover JSON`), 0600); err != nil {
		t.Fatal(err)
	}
	restored, found, err := storage.Load()
	if err != nil || !found || restored.ConfigKey != "private-source" {
		t.Fatalf("SQLite was not preferred after migration: %v %v", found, err)
	}
	programs, err := restored.ProgramQuery(context.Background(), nil, math.MinInt64, math.MaxInt64)
	if err != nil || len(programs) != 1 || programs[0].Title != "Last good guide" {
		t.Fatalf("restored guide: %+v %v", programs, err)
	}
	revision := restored.Guide.Revision
	restored.Health.LastFailureUnix = 100
	saved, err = storage.Save(restored)
	if err != nil || saved.Guide.Revision != revision {
		t.Fatalf("health-only update rewrote the schedule: %v", err)
	}
}

type gatedSnapshotStorage struct {
	*FileSnapshotStorage
	entered, release chan struct{}
}

func (s *gatedSnapshotStorage) Save(snapshot Snapshot) (Snapshot, error) {
	close(s.entered)
	<-s.release
	return s.FileSnapshotStorage.Save(snapshot)
}
func TestImportDoesNotBlockPublishedReadsOrPlaybackHeartbeats(t *testing.T) {
	base := NewFileSnapshotStorage(filepath.Join(t.TempDir(), "catalog.json"))
	defer base.Close()
	store := NewStore()
	initial := Snapshot{Catalog: model.CatalogState{Channels: []model.Channel{{ID: "a"}}, Programs: []model.Program{{ID: "old", ChannelID: "a", StartUnix: 1, EndUnix: 100}}}}
	if err := store.Replace(initial); err != nil {
		t.Fatal(err)
	}
	session := store.StartWatch("channel", "a", "News")
	gate := &gatedSnapshotStorage{FileSnapshotStorage: base, entered: make(chan struct{}), release: make(chan struct{})}
	store.SetStorage(gate)
	done := make(chan error, 1)
	go func() {
		candidate := initial
		candidate.Catalog.Programs = []model.Program{{ID: "new", ChannelID: "a", StartUnix: 2, EndUnix: 100}}
		done <- store.ReplaceExact(candidate)
	}()
	<-gate.entered
	read := make(chan Snapshot, 1)
	go func() { snapshot := store.Current(); store.HeartbeatWatch(session.ID); read <- snapshot }()
	var old Snapshot
	timedOut := false
	select {
	case old = <-read:
	case <-time.After(time.Second):
		timedOut = true
	}
	close(gate.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if timedOut {
		t.Fatal("guide import blocked catalog reads or playback heartbeat")
	}
	if len(old.Catalog.Programs) != 1 || old.Catalog.Programs[0].ID != "old" {
		t.Fatal("uncommitted guide was published")
	}
	programs, err := store.Current().ProgramQuery(context.Background(), nil, 0, 100)
	if err != nil || len(programs) != 1 || programs[0].ID != "new" {
		t.Fatalf("committed guide was not published: %+v %v", programs, err)
	}
}
