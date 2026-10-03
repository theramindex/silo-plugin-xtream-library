package cache

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

type Snapshot struct {
	Catalog                model.CatalogState
	Health                 model.SyncHealth
	PlaybackResolvedAtUnix int64
	ConfigKey              string
	// Version increases on every store mutation. It is process-local (not
	// persisted) and lets readers cache data derived from a snapshot, such as
	// the sorted guide index.
	Version uint64 `json:"-"`
}

type Store struct {
	mu            sync.RWMutex
	snapshot      Snapshot
	version       uint64
	adminSettings json.RawMessage
	sessions      map[string]WatchSession
}

type WatchSession struct {
	ID                string `json:"id"`
	ItemKind          string `json:"itemKind"`
	ItemID            string `json:"itemId"`
	ItemName          string `json:"itemName,omitempty"`
	StartedAtUnix     int64  `json:"startedAtUnix"`
	LastHeartbeatUnix int64  `json:"lastHeartbeatUnix"`
	EndedAtUnix       int64  `json:"endedAtUnix,omitempty"`
	EndReason         string `json:"endReason,omitempty"`
}

const (
	watchSessionTTLSeconds  = int64((6 * time.Hour) / time.Second)
	endedSessionTTLSeconds  = int64(time.Hour / time.Second)
	maximumWatchSessionRows = 2048
)

func NewStore() *Store {
	return &Store{sessions: map[string]WatchSession{}}
}

func (s *Store) Current() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

func (s *Store) Replace(snapshot Snapshot) {
	s.replace(snapshot, true)
}

func (s *Store) ReplaceExact(snapshot Snapshot) {
	s.replace(snapshot, false)
}

func (s *Store) replace(snapshot Snapshot, preserveGuide bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	regressionWarning := ""
	if !preserveGuide && len(snapshot.Catalog.Programs) > 0 && sameGuideScope(s.snapshot, snapshot) && haveProgramChannels(snapshot.Catalog.Channels, s.snapshot.Catalog.Programs) {
		regressionWarning = guideRegressionWarning(s.snapshot.Catalog.Programs, snapshot.Catalog.Programs, snapshot.Health.LastSuccessUnix)
	}
	if regressionWarning != "" || (preserveGuide && shouldPreserveGuide(s.snapshot, snapshot)) {
		snapshot.Catalog.Programs = append([]model.Program(nil), s.snapshot.Catalog.Programs...)
		snapshot.Catalog.Health.EPGStatus = preservedEPGStatus(s.snapshot)
		snapshot.Catalog.Health.EPGProgramCount = len(s.snapshot.Catalog.Programs)
		snapshot.Catalog.Health.EPGLastSuccessUnix = s.snapshot.Health.EPGLastSuccessUnix
		snapshot.Catalog.Health.EPGLastFailureUnix = s.snapshot.Health.EPGLastFailureUnix
		snapshot.Catalog.Health.EPGLastError = s.snapshot.Health.EPGLastError
		snapshot.Catalog.Health.EPGWarning = regressionWarning
		if regressionWarning != "" {
			// Health mirrors the preserved guide, not the rejected result.
			snapshot.Health.EPGStatus = snapshot.Catalog.Health.EPGStatus
			snapshot.Health.EPGProgramCount = snapshot.Catalog.Health.EPGProgramCount
			snapshot.Health.EPGLastSuccessUnix = snapshot.Catalog.Health.EPGLastSuccessUnix
			snapshot.Health.EPGLastFailureUnix = snapshot.Catalog.Health.EPGLastFailureUnix
			snapshot.Health.EPGLastError = snapshot.Catalog.Health.EPGLastError
			snapshot.Health.EPGWarning = regressionWarning
		}
	}
	snapshot.Health.LastFailureUnix = 0
	snapshot.Health.LastError = ""
	if snapshot.Health.EPGStatus == "" && snapshot.Health.LastSuccessUnix != 0 && len(snapshot.Catalog.Programs) > 0 {
		snapshot.Catalog.Health.EPGStatus = "ok"
		snapshot.Catalog.Health.EPGProgramCount = len(snapshot.Catalog.Programs)
		snapshot.Catalog.Health.EPGLastSuccessUnix = snapshot.Health.LastSuccessUnix
		snapshot.Health.EPGStatus = "ok"
		snapshot.Health.EPGProgramCount = len(snapshot.Catalog.Programs)
		snapshot.Health.EPGLastSuccessUnix = snapshot.Health.LastSuccessUnix
	}
	if snapshot.Health.EPGStatus == "" {
		snapshot.Health.EPGStatus = s.snapshot.Health.EPGStatus
		snapshot.Health.EPGProgramCount = s.snapshot.Health.EPGProgramCount
		snapshot.Health.EPGLastSuccessUnix = s.snapshot.Health.EPGLastSuccessUnix
		snapshot.Health.EPGLastFailureUnix = s.snapshot.Health.EPGLastFailureUnix
		snapshot.Health.EPGLastError = s.snapshot.Health.EPGLastError
		snapshot.Health.EPGWarning = s.snapshot.Health.EPGWarning
	}
	s.version++
	snapshot.Version = s.version
	s.snapshot = snapshot
}

// touchLocked marks an in-place snapshot mutation. Callers hold s.mu.
func (s *Store) touchLocked() {
	s.version++
	s.snapshot.Version = s.version
}

// shouldPreserveGuide decides whether a non-exact sync keeps the current
// guide. It deliberately ignores the transient EPG status (a refresh marks the
// guide "loading" before running) and only looks at the data itself.
func shouldPreserveGuide(current, next Snapshot) bool {
	if !sameGuideScope(current, next) {
		return false
	}
	if len(current.Catalog.Programs) == 0 {
		return false
	}
	if len(next.Catalog.Programs) >= len(current.Catalog.Programs) {
		return false
	}
	return haveProgramChannels(next.Catalog.Channels, current.Catalog.Programs)
}

// sameGuideScope reports whether two snapshots describe the same source
// configuration, so the older guide is still valid for the newer lineup.
// Source mode or configuration changes always accept the new guide.
func sameGuideScope(current, next Snapshot) bool {
	if current.ConfigKey != "" && next.ConfigKey != "" && current.ConfigKey != next.ConfigKey {
		return false
	}
	return sameCatalogSource(current.Catalog.Source, next.Catalog.Source)
}

// guideRegressionThreshold: a refresh whose program count or channel coverage
// falls below this fraction of the previous guide is treated as an upstream
// glitch and the last known good guide is kept.
const guideRegressionThreshold = 0.5

// guideRegressionWarning returns a non-empty warning when next drops more than
// half of the still-relevant programs or covered channels in previous.
// Programs that already ended before atUnix are not counted, so a stale guide
// cannot block a legitimately smaller fresh one.
func guideRegressionWarning(previous, next []model.Program, atUnix int64) string {
	previousCount, previousChannels := relevantGuideCoverage(previous, atUnix)
	if previousCount == 0 {
		return ""
	}
	nextCount, nextChannels := relevantGuideCoverage(next, atUnix)
	if float64(nextCount) >= float64(previousCount)*guideRegressionThreshold && float64(nextChannels) >= float64(previousChannels)*guideRegressionThreshold {
		return ""
	}
	return fmt.Sprintf("Guide refresh returned %d programs across %d channels (previously %d across %d); kept the last known good guide.", nextCount, nextChannels, previousCount, previousChannels)
}

func relevantGuideCoverage(programs []model.Program, atUnix int64) (int, int) {
	count := 0
	channels := map[string]struct{}{}
	for _, program := range programs {
		if atUnix > 0 && program.EndUnix != 0 && program.EndUnix <= atUnix {
			continue
		}
		count++
		channels[program.ChannelID] = struct{}{}
	}
	return count, len(channels)
}

func preservedEPGStatus(current Snapshot) string {
	status := current.Catalog.Health.EPGStatus
	if status == "" || status == "loading" {
		status = current.Health.EPGStatus
	}
	if status == "" || status == "loading" {
		if len(current.Catalog.Programs) > 0 {
			return "ok"
		}
	}
	return status
}

func sameCatalogSource(current, next model.Source) bool {
	if current.ID != next.ID || current.Name != next.Name || current.Mode != next.Mode {
		return false
	}
	if current.ChannelProfile == nil || next.ChannelProfile == nil {
		return current.ChannelProfile == nil && next.ChannelProfile == nil
	}
	return current.ChannelProfile.ID == next.ChannelProfile.ID && current.ChannelProfile.Name == next.ChannelProfile.Name
}

func haveProgramChannels(channels []model.Channel, programs []model.Program) bool {
	channelIDs := make(map[string]bool, len(channels))
	for _, channel := range channels {
		channelIDs[channel.ID] = true
	}
	for _, program := range programs {
		if program.ChannelID != "" && !channelIDs[program.ChannelID] {
			return false
		}
	}
	return true
}

func (s *Store) AdminSettings() json.RawMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.adminSettings) == 0 {
		return json.RawMessage(`{}`)
	}
	return append(json.RawMessage(nil), s.adminSettings...)
}

func (s *Store) HasAdminSettings() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.adminSettings) > 0
}

func (s *Store) SetAdminSettings(settings json.RawMessage) json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.adminSettings = append(json.RawMessage(nil), settings...)
	return append(json.RawMessage(nil), s.adminSettings...)
}

func (s *Store) StartWatch(kind, id, name string) WatchSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureSessions()
	now := time.Now().Unix()
	s.pruneWatchSessionsLocked(now)
	session := WatchSession{
		ID:                newSessionID(),
		ItemKind:          kind,
		ItemID:            id,
		ItemName:          name,
		StartedAtUnix:     now,
		LastHeartbeatUnix: now,
	}
	s.sessions[session.ID] = session

	return session
}

func (s *Store) HeartbeatWatch(id string) (WatchSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureSessions()
	session, ok := s.sessions[id]
	if !ok || session.EndedAtUnix != 0 {
		return WatchSession{}, false
	}
	session.LastHeartbeatUnix = time.Now().Unix()
	s.sessions[id] = session
	return session, true
}

func (s *Store) StopWatch(id, reason string) (WatchSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureSessions()
	session, ok := s.sessions[id]
	if !ok {
		return WatchSession{}, false
	}
	if session.EndedAtUnix == 0 {
		session.EndedAtUnix = time.Now().Unix()
		if reason == "" {
			reason = "stopped"
		}
		session.EndReason = reason
		s.sessions[id] = session
	}
	return session, true
}

func (s *Store) RecordFailure(atUnix int64, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.touchLocked()

	s.snapshot.Health.LastFailureUnix = atUnix
	s.snapshot.Health.LastError = message
	if s.snapshot.Catalog.Health.LastSuccessUnix != 0 && s.snapshot.Health.LastSuccessUnix == 0 {
		s.snapshot.Health.LastSuccessUnix = s.snapshot.Catalog.Health.LastSuccessUnix
	}
}

func (s *Store) MarkEPGLoading() {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.touchLocked()

	s.snapshot.Health.EPGStatus = "loading"
	s.snapshot.Health.EPGLastError = ""
}

func (s *Store) ClearGuidePrograms(atUnix int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.touchLocked()

	s.snapshot.Catalog.Programs = nil
	s.snapshot.Catalog.Health.EPGStatus = "loading"
	s.snapshot.Catalog.Health.EPGProgramCount = 0
	s.snapshot.Catalog.Health.EPGLastSuccessUnix = 0
	s.snapshot.Catalog.Health.EPGLastFailureUnix = 0
	s.snapshot.Catalog.Health.EPGLastError = ""
	s.snapshot.Health.EPGStatus = "loading"
	s.snapshot.Health.EPGProgramCount = 0
	s.snapshot.Health.EPGLastSuccessUnix = 0
	s.snapshot.Health.EPGLastFailureUnix = 0
	s.snapshot.Health.EPGLastError = ""
	_ = atUnix
}

// ReplacePrograms swaps in a refreshed guide. When the result loses more than
// half of the current guide's programs or channel coverage, the current guide
// is kept and a warning is recorded instead; it reports whether the new
// programs were applied.
func (s *Store) ReplacePrograms(programs []model.Program, atUnix int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.touchLocked()

	if warning := guideRegressionWarning(s.snapshot.Catalog.Programs, programs, atUnix); warning != "" {
		status := preservedEPGStatus(s.snapshot)
		s.snapshot.Catalog.Health.EPGStatus = status
		s.snapshot.Catalog.Health.EPGWarning = warning
		s.snapshot.Health.EPGStatus = status
		s.snapshot.Health.EPGProgramCount = len(s.snapshot.Catalog.Programs)
		s.snapshot.Health.EPGWarning = warning
		return false
	}

	s.snapshot.Catalog.Programs = append([]model.Program(nil), programs...)
	s.snapshot.Catalog.Health.EPGStatus = "ok"
	s.snapshot.Catalog.Health.EPGProgramCount = len(programs)
	s.snapshot.Catalog.Health.EPGLastSuccessUnix = atUnix
	s.snapshot.Health.EPGStatus = "ok"
	s.snapshot.Health.EPGProgramCount = len(programs)
	s.snapshot.Health.EPGLastSuccessUnix = atUnix
	s.snapshot.Health.EPGLastFailureUnix = 0
	s.snapshot.Health.EPGLastError = ""
	s.snapshot.Catalog.Health.EPGWarning = ""
	s.snapshot.Health.EPGWarning = ""
	return true
}

func (s *Store) RecordEPGFailure(atUnix int64, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.touchLocked()

	s.snapshot.Health.EPGStatus = "failed"
	s.snapshot.Health.EPGLastFailureUnix = atUnix
	s.snapshot.Health.EPGLastError = message
	s.snapshot.Catalog.Health.EPGStatus = "failed"
	s.snapshot.Catalog.Health.EPGLastFailureUnix = atUnix
	s.snapshot.Catalog.Health.EPGLastError = message
}

func (s *Store) ensureSessions() {
	if s.sessions == nil {
		s.sessions = map[string]WatchSession{}
	}
}

func (s *Store) pruneWatchSessionsLocked(now int64) {
	for id, session := range s.sessions {
		if session.EndedAtUnix > 0 && session.EndedAtUnix < now-endedSessionTTLSeconds {
			delete(s.sessions, id)
			continue
		}
		if session.EndedAtUnix == 0 && session.LastHeartbeatUnix < now-watchSessionTTLSeconds {
			delete(s.sessions, id)
		}
	}
	for len(s.sessions) >= maximumWatchSessionRows {
		oldestID := ""
		oldestUnix := now
		for id, session := range s.sessions {
			if oldestID == "" || session.LastHeartbeatUnix < oldestUnix {
				oldestID = id
				oldestUnix = session.LastHeartbeatUnix
			}
		}
		if oldestID == "" {
			break
		}
		delete(s.sessions, oldestID)
	}
}

func newSessionID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(buf[:])
}
