package plugin

import (
	"context"
	"fmt"
	sharedhttp "github.com/theramindex/silo-plugins/pkg/livemedia/httpclient"
	"github.com/theramindex/silo-plugins/pkg/livemedia/rewind"
	mediaweb "github.com/theramindex/silo-plugins/pkg/livemedia/web"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

func (s *HTTPRoutesServer) mediaController() *mediaweb.Controller {
	s.mediaOnce.Do(func() {
		s.media = mediaweb.New(mediaweb.Config{Prefix: "/xtream", Root: filepath.Join("/var/lib/continuum/plugins", "silo.ramindex.xtream", "media"), RewindOptions: func() rewind.Config {
			settings := s.adminSettingsPayload()
			return mediaweb.RewindSettings(settings)
		}, Resolve: func(ctx context.Context, channel, owner, inputID string) (mediaweb.Input, error) {
			poolID := inputID
			reservation := mediaweb.Reservation(ctx)
			if reservation != "" {
				s.watchOwnerMu.Lock()
				if s.watchOwners[reservation] != owner {
					s.watchOwnerMu.Unlock()
					return mediaweb.Input{}, fmt.Errorf("watch reservation belongs to another viewer")
				}
				if s.watchBindings == nil {
					s.watchBindings = map[string]string{}
				}
				if s.watchBindings[reservation] != "" {
					s.watchOwnerMu.Unlock()
					return mediaweb.Input{}, fmt.Errorf("watch reservation is already streaming")
				}
				s.watchBindings[reservation] = inputID
				s.watchOwnerMu.Unlock()
				poolID = reservation
			}
			settingsMap := s.adminSettingsPayload()
			candidates := s.mediaBackoff.Order(mediaweb.StationOrder(settingsMap["stationMappings"], channel))
			candidate := 0
			var source string
			var err error
			for candidate < len(candidates) {
				source, err = s.resolveChannelStreamURL(candidates[candidate], poolID, "")
				if err == nil {
					break
				}
				candidate++
			}
			if err != nil {
				s.watchOwnerMu.Lock()
				if s.watchBindings[reservation] == inputID {
					delete(s.watchBindings, reservation)
				}
				s.watchOwnerMu.Unlock()
				return mediaweb.Input{}, err
			}
			client := sharedhttp.NewRestricted(s.relayConfiguredHosts()...)
			client.Timeout = 0
			if transport, ok := client.Transport.(*http.Transport); ok {
				transport.ResponseHeaderTimeout = 10 * time.Second
			}
			var accountRequired atomic.Bool
			accountRequired.Store(strings.HasPrefix(candidates[candidate], "xtream:"))
			return mediaweb.Input{URL: source, Container: publicStreamFormat(source), Client: client, Release: func() {
				s.accountPool.Release(poolID)
				s.watchOwnerMu.Lock()
				if reservation != "" && s.watchBindings[reservation] == inputID {
					delete(s.watchBindings, reservation)
				}
				s.watchOwnerMu.Unlock()
			}, Heartbeat: func() error {
				if reservation != "" {
					s.watchOwnerMu.Lock()
					if _, ok := s.watchSeen[reservation]; ok {
						s.watchSeen[reservation] = time.Now()
					}
					s.watchOwnerMu.Unlock()
				}
				if !s.accountPool.Touch(poolID) && accountRequired.Load() {
					return fmt.Errorf("source reservation expired")
				}
				return nil
			}, Fallback: func(context.Context) (string, *http.Client, error) {
				s.accountPool.Reject(poolID)
				for candidate < len(candidates) {
					next, err := s.resolveChannelStreamURL(candidates[candidate], poolID, "")
					if err == nil && publicStreamFormat(next) == publicStreamFormat(source) {
						accountRequired.Store(strings.HasPrefix(candidates[candidate], "xtream:"))
						return next, client, nil
					}
					s.mediaBackoff.Reject(candidates[candidate])
					candidate++
				}
				return "", nil, fmt.Errorf("no alternate source is available")
			}}, nil
		}})
	})
	return s.media
}

func (s *HTTPRoutesServer) reserveMappedWatch(channel, session string) error {
	settings := s.adminSettingsPayload()
	var last error
	for _, id := range mediaweb.StationOrder(settings["stationMappings"], channel) {
		if source, ok := s.xtreamSourceForItem(id); ok {
			if _, err := s.accountPool.Lease(session, source); err != nil {
				last = err
				continue
			}
		}
		return nil
	}
	return last
}
