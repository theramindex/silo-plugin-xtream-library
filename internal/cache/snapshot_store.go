package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const DefaultSnapshotFile = "/var/lib/continuum/plugins/silo.ramindex.dispatcharr/catalog-snapshot.json"

type SnapshotStorage interface {
	Load() (Snapshot, bool, error)
	Save(Snapshot) error
	Path() string
}

type FileSnapshotStorage struct {
	path string
	mu   sync.Mutex
}

func NewFileSnapshotStorage(path string) *FileSnapshotStorage {
	if strings.TrimSpace(path) == "" {
		path = os.Getenv("DISPATCHARR_CATALOG_SNAPSHOT_FILE")
	}
	if strings.TrimSpace(path) == "" {
		path = DefaultSnapshotFile
	}
	return &FileSnapshotStorage{path: path}
}

func (s *FileSnapshotStorage) Path() string {
	return s.path
}

func (s *FileSnapshotStorage) Load() (Snapshot, bool, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return Snapshot{}, false, nil
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, false, fmt.Errorf("decode catalog snapshot file: %w", err)
	}
	if len(snapshot.Catalog.Channels) == 0 {
		return Snapshot{}, false, nil
	}
	return snapshot, true, nil
}

func (s *FileSnapshotStorage) Save(snapshot Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(snapshot.Catalog.Channels) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode catalog snapshot file: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".catalog-snapshot-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}
