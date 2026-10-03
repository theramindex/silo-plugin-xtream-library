package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
	"github.com/theramindex/silo-plugin-xtream-library/internal/upstream/dispatcharr"
	sharedhttp "github.com/theramindex/silo-plugin-xtream-library/internal/upstream/httpclient"
	"github.com/theramindex/silo-plugin-xtream-library/internal/upstream/xmltv"
	"github.com/theramindex/silo-plugin-xtream-library/internal/upstream/xtream"
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
	// OpenURL streams a response body. XMLTV guides use it so large feeds are
	// decoded incrementally instead of being buffered in full. When nil and
	// FetchURL is set, OpenURL wraps FetchURL's bytes.
	OpenURL func(ctx context.Context, rawURL string) (io.ReadCloser, error)
}

type Service struct {
	store              *cache.Store
	snapshotStorage    cache.SnapshotStorage
	xtreamFactory      func(baseURL, username, password string) XtreamClient
	dispatcharrFactory func(settings config.Settings) DispatcharrClient
	fetchURL           func(ctx context.Context, rawURL string) ([]byte, error)
	openURL            func(ctx context.Context, rawURL string) (io.ReadCloser, error)
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

	client := &http.Client{Timeout: 5 * time.Minute}
	defaultOpen := func(ctx context.Context, rawURL string) (io.ReadCloser, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, sharedhttp.RedactErrorURL(err)
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, sharedhttp.RedactErrorURL(err)
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			_ = response.Body.Close()
			return nil, fmt.Errorf("unexpected status %d", response.StatusCode)
		}
		return response.Body, nil
	}

	fetcher := deps.FetchURL
	opener := deps.OpenURL
	if fetcher == nil {
		fetcher = func(ctx context.Context, rawURL string) ([]byte, error) {
			body, err := defaultOpen(ctx, rawURL)
			if err != nil {
				return nil, err
			}
			defer body.Close()
			return sharedhttp.ReadAllLimit(body, sharedhttp.MaxCatalogResponseBytes)
		}
		if opener == nil {
			opener = defaultOpen
		}
	}
	if opener == nil {
		injected := fetcher
		opener = func(ctx context.Context, rawURL string) (io.ReadCloser, error) {
			data, err := injected(ctx, rawURL)
			if err != nil {
				return nil, err
			}
			return io.NopCloser(bytes.NewReader(data)), nil
		}
	}

	return &Service{store: store, snapshotStorage: deps.SnapshotStorage, xtreamFactory: factory, dispatcharrFactory: dispatcharrFactory, fetchURL: fetcher, openURL: opener}
}

// fetchXMLTV streams an XMLTV document from rawURL, decoding it token by token
// under the shared 256 MiB catalog size cap so the raw body is never held in
// memory. label prefixes errors ("fetch <label> xmltv" / "parse <label> xmltv").
func (s *Service) fetchXMLTV(ctx context.Context, rawURL, label string) (xmltv.Document, error) {
	prefix := "xmltv"
	if label != "" {
		prefix = label + " xmltv"
	}
	body, err := s.openURL(ctx, rawURL)
	if err != nil {
		return xmltv.Document{}, fmt.Errorf("fetch %s: %w", prefix, sharedhttp.RedactErrorURL(err))
	}
	defer body.Close()
	doc, err := xmltv.ParseReader(body, xmltv.ParseOptions{MaxBytes: sharedhttp.MaxCatalogResponseBytes})
	if err != nil {
		return xmltv.Document{}, fmt.Errorf("parse %s: %w", prefix, sharedhttp.RedactErrorURL(err))
	}
	return doc, nil
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
