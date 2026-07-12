package app

import (
	"context"
	"fmt"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/m3u"
)

func (s *Service) ResolvePlayback(ctx context.Context, settings config.Settings, streamID int64) (string, error) {
	if err := settings.Validate(); err != nil {
		return "", err
	}
	_ = ctx

	switch settings.SourceMode {
	case config.SourceModeXtream:
		resolved := s.xtreamFactory(settings.XtreamBaseURL, settings.XtreamUsername, settings.XtreamPassword).ResolveLiveStreamURL(streamID)
		if resolved == "" {
			return "", fmt.Errorf("unable to resolve playback url")
		}
		return resolved, nil
	case config.SourceModeM3UXMLTV:
		playlistData, err := s.fetchURL(ctx, settings.M3UURL)
		if err != nil {
			return "", err
		}
		entries, err := m3u.Parse(playlistData)
		if err != nil {
			return "", err
		}
		if len(entries) == 0 {
			return "", fmt.Errorf("no playlist entries available")
		}
		return entries[0].StreamURL, nil
	default:
		return "", fmt.Errorf("source mode %q not implemented", settings.SourceMode)
	}
}
