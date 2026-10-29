package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"github.com/theramindex/silo-plugins/pkg/epg"
)

const DefaultSnapshotFile = "/var/lib/continuum/plugins/silo.ramindex.xtream/catalog-snapshot.json"

type SnapshotStorage interface {
	Load() (Snapshot, bool, error)
	Save(Snapshot) (Snapshot, error)
	Path() string
}

type FileSnapshotStorage struct {
	path, legacy string
	mu           sync.Mutex
	db           *epg.Store[model.Program]
}

func NewFileSnapshotStorage(path string) *FileSnapshotStorage {
	if strings.TrimSpace(path) == "" {
		path = os.Getenv("XTREAM_CATALOG_SNAPSHOT_FILE")
	}
	if strings.TrimSpace(path) == "" {
		path = DefaultSnapshotFile
	}
	legacy := path
	if strings.HasSuffix(path, ".sqlite") {
		legacy = strings.TrimSuffix(path, ".sqlite") + ".json"
	} else {
		path = strings.TrimSuffix(path, ".json") + ".sqlite"
	}
	return &FileSnapshotStorage{path: path, legacy: legacy}
}
func (s *FileSnapshotStorage) Path() string { return s.path }
func programFields(p model.Program) epg.Fields {
	return epg.Fields{ID: p.ID, ChannelID: p.ChannelID, Start: p.StartUnix, End: p.EndUnix}
}
func (s *FileSnapshotStorage) open() error {
	if s.db != nil {
		return nil
	}
	db, err := epg.Open(s.path, programFields)
	if err != nil {
		return err
	}
	s.db = db
	return nil
}
func (s *FileSnapshotStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
func (s *FileSnapshotStorage) Load() (Snapshot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.open(); err != nil {
		return Snapshot{}, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, view, found, err := s.db.Load(ctx)
	if err != nil {
		return Snapshot{}, false, err
	}
	if found {
		var snapshot Snapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return Snapshot{}, false, err
		}
		snapshot.Guide = view
		return snapshot, true, nil
	}
	data, err = os.ReadFile(s.legacy)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return Snapshot{}, false, nil
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, false, fmt.Errorf("decode legacy catalog snapshot: %w", err)
	}
	if len(snapshot.Catalog.Channels) == 0 {
		return Snapshot{}, false, nil
	}
	// The caller can serve the legacy guide while importing it in the background.
	return snapshot, true, nil
}
func (s *FileSnapshotStorage) Save(snapshot Snapshot) (Snapshot, error) {
	if len(snapshot.Catalog.Channels) == 0 {
		return snapshot, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.open(); err != nil {
		return Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return s.save(ctx, snapshot)
}
func (s *FileSnapshotStorage) save(ctx context.Context, snapshot Snapshot) (Snapshot, error) {
	programs := snapshot.Catalog.Programs
	snapshot.Catalog.Programs = nil
	metadata, err := json.Marshal(snapshot)
	if err != nil {
		return Snapshot{}, err
	}
	if snapshot.Guide != nil && programs == nil {
		err = s.db.SaveMetadata(ctx, metadata, snapshot.Guide)
	} else {
		snapshot.Guide, err = s.db.Replace(ctx, metadata, programs)
	}
	return snapshot, err
}

// ProgramCount and ProgramQuery work for both transient imports and durable
// guide views. Normal requests never materialize the full persisted schedule.
func (s Snapshot) ProgramCount() int {
	if s.Guide != nil {
		return s.Guide.Count
	}
	return len(s.Catalog.Programs)
}
func (s Snapshot) ProgramQuery(ctx context.Context, ids []string, start, end int64) ([]model.Program, error) {
	if s.Guide != nil {
		return s.Guide.Query(ctx, epg.Query{ChannelIDs: ids, Start: start, End: end})
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	out := []model.Program{}
	for _, p := range s.Catalog.Programs {
		if len(ids) > 0 && !selected[p.ChannelID] {
			continue
		}
		if p.StartUnix >= end {
			continue
		}
		if p.EndUnix > p.StartUnix {
			if p.EndUnix <= start {
				continue
			}
		} else if p.StartUnix < start {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
func (s Snapshot) WithPrograms(ctx context.Context) (Snapshot, error) {
	if s.Guide == nil {
		return s, nil
	}
	programs, err := s.ProgramQuery(ctx, nil, math.MinInt64, math.MaxInt64)
	if err != nil {
		return Snapshot{}, err
	}
	s.Catalog.Programs = programs
	s.Guide = nil
	return s, nil
}
func (s Snapshot) coversChannels(channels []model.Channel) bool {
	if s.Guide == nil {
		return haveProgramChannels(channels, s.Catalog.Programs)
	}
	ids := map[string]bool{}
	for _, ch := range channels {
		ids[ch.ID] = true
	}
	for _, id := range s.Guide.Channels {
		if id != "" && !ids[id] {
			return false
		}
	}
	return true
}
func snapshotRegressionWarning(previous Snapshot, next []model.Program, at int64) (string, error) {
	if previous.Guide == nil {
		return guideRegressionWarning(previous.Catalog.Programs, next, at), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	count, channels, err := previous.Guide.Coverage(ctx, at)
	if err != nil {
		return "", err
	}
	nextCount, nextChannels := relevantGuideCoverage(next, at)
	if count == 0 || (float64(nextCount) >= float64(count)*guideRegressionThreshold && float64(nextChannels) >= float64(channels)*guideRegressionThreshold) {
		return "", nil
	}
	return fmt.Sprintf("Guide refresh returned %d programs across %d channels (previously %d across %d); kept the last known good guide.", nextCount, nextChannels, count, channels), nil
}
