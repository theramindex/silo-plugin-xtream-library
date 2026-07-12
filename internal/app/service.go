package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/cache"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/dispatcharr"
	sharedhttp "github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/httpclient"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xtream"
)

type XtreamClient interface {
	TestConnection(ctx context.Context) error
	LiveStreams(ctx context.Context) ([]xtream.LiveStream, error)
	ShortEPG(ctx context.Context, streamID int64) (xtream.ShortEPGResponse, error)
	ResolveLiveStreamURL(streamID int64) string
}

type DispatcharrClient interface {
	TestConnection(ctx context.Context) error
	Version(ctx context.Context) (dispatcharr.VersionInfo, error)
	Channels(ctx context.Context) ([]dispatcharr.Channel, error)
	ChannelGroups(ctx context.Context) ([]dispatcharr.ChannelGroup, error)
	ChannelProfiles(ctx context.Context) ([]dispatcharr.ChannelProfile, error)
	CurrentUser(ctx context.Context) (dispatcharr.CurrentUser, error)
	Programs(ctx context.Context) ([]dispatcharr.Program, error)
	SearchPrograms(ctx context.Context, start, end time.Time) ([]dispatcharr.ProgramSearchResult, error)
	VODCategories(ctx context.Context) ([]dispatcharr.VODCategory, error)
	Movies(ctx context.Context) ([]dispatcharr.Movie, error)
	Series(ctx context.Context) ([]dispatcharr.Series, error)
	LiveStreamURL(channelUUID string) string
	LogoCacheURL(logoID string) string
	MovieStreamURL(movieUUID string) string
	SeriesStreamURL(seriesUUID string) string
	AbsoluteURL(raw string) string
}

type Dependencies struct {
	Store              *cache.Store
	SnapshotStorage    cache.SnapshotStorage
	XtreamFactory      func(baseURL, username, password string) XtreamClient
	DispatcharrFactory func(settings config.Settings) DispatcharrClient
	FetchURL           func(ctx context.Context, rawURL string) ([]byte, error)
}

type Service struct {
	store              *cache.Store
	snapshotStorage    cache.SnapshotStorage
	xtreamFactory      func(baseURL, username, password string) XtreamClient
	dispatcharrFactory func(settings config.Settings) DispatcharrClient
	fetchURL           func(ctx context.Context, rawURL string) ([]byte, error)
}

const SourceModeResetWarning = "Changing source mode resets cached channel and guide data before rebuilding Live TV."

func NewService(deps Dependencies) *Service {
	store := deps.Store
	if store == nil {
		store = cache.NewStore()
	}

	factory := deps.XtreamFactory
	if factory == nil {
		factory = func(baseURL, username, password string) XtreamClient {
			return xtream.NewClient(baseURL, username, password)
		}
	}
	dispatcharrFactory := deps.DispatcharrFactory
	if dispatcharrFactory == nil {
		dispatcharrFactory = func(settings config.Settings) DispatcharrClient {
			if settings.EffectiveSourceMode() == config.SourceModeAPIKey {
				return dispatcharr.NewAPIKeyClient(settings.DispatcharrURL, settings.DispatcharrAPIKey)
			}
			return dispatcharr.NewLoginClient(settings.DispatcharrURL, settings.DispatcharrUser, settings.DispatcharrPass)
		}
	}

	fetcher := deps.FetchURL
	if fetcher == nil {
		client := &http.Client{Timeout: 5 * time.Minute}
		fetcher = func(ctx context.Context, rawURL string) ([]byte, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
			if err != nil {
				return nil, err
			}
			response, err := client.Do(req)
			if err != nil {
				return nil, sharedhttp.RedactErrorURL(err)
			}
			defer response.Body.Close()
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				return nil, fmt.Errorf("unexpected status %d", response.StatusCode)
			}
			return sharedhttp.ReadAllLimit(response.Body, sharedhttp.MaxCatalogResponseBytes)
		}
	}

	return &Service{store: store, snapshotStorage: deps.SnapshotStorage, xtreamFactory: factory, dispatcharrFactory: dispatcharrFactory, fetchURL: fetcher}
}

func (s *Service) replaceSnapshot(snapshot cache.Snapshot) error {
	s.store.Replace(snapshot)
	return s.persistSnapshot()
}

func (s *Service) replaceSnapshotExact(snapshot cache.Snapshot) error {
	s.store.ReplaceExact(snapshot)
	return s.persistSnapshot()
}

func (s *Service) replaceSnapshotAfterSync(snapshot cache.Snapshot, exactGuide bool) error {
	if exactGuide && len(snapshot.Catalog.Programs) > 0 {
		return s.replaceSnapshotExact(snapshot)
	}
	return s.replaceSnapshot(snapshot)
}

func (s *Service) replacePrograms(programs []model.Program, atUnix int64) error {
	if len(programs) == 0 {
		err := fmt.Errorf("no guide programs were returned")
		s.store.RecordEPGFailure(atUnix, err.Error())
		_ = s.persistSnapshot()
		return err
	}
	s.store.ReplacePrograms(programs, atUnix)
	return s.persistSnapshot()
}

func (s *Service) persistSnapshot() error {
	if s.snapshotStorage == nil {
		return nil
	}
	if err := s.snapshotStorage.Save(s.store.Current()); err != nil {
		return fmt.Errorf("persist catalog snapshot: %w", err)
	}
	return nil
}

func (s *Service) SwitchSourceMode(ctx context.Context, previous, next config.Settings, nowUnix int64) (string, error) {
	warning := ""
	if previous.SourceMode != "" && previous.SourceMode != next.SourceMode {
		s.store.Replace(cache.Snapshot{Catalog: model.CatalogState{Source: model.LiveTVSource(model.SourceMode(next.SourceMode))}})
		warning = SourceModeResetWarning
	}
	if err := s.SyncNow(ctx, next, nowUnix); err != nil {
		return warning, err
	}
	return warning, nil
}
