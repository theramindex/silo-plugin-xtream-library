package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/matching"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/dispatcharr"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/m3u"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/xmltv"
)

func (s *Service) TestConnection(ctx context.Context, settings config.Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}

	switch settings.SourceMode {
	case config.SourceModeDirectLogin, config.SourceModeAPIKey:
		return testDispatcharrDirectConnection(ctx, s.dispatcharrFactory(settings))
	case config.SourceModeXtream:
		client := s.xtreamFactory(settings.XtreamBaseURL, settings.XtreamUsername, settings.XtreamPassword)
		return testXtreamConnection(ctx, client)
	case config.SourceModeM3UXMLTV:
		playlistData, err := s.fetchURL(ctx, settings.M3UURL)
		if err != nil {
			return err
		}
		xmltvData, err := s.fetchURL(ctx, settings.EPGXMLURL)
		if err != nil {
			return err
		}
		entries, err := m3u.Parse(playlistData)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return fmt.Errorf("no playlist entries available")
		}
		doc, err := xmltv.Parse(xmltvData)
		if err != nil {
			return err
		}
		if len(doc.Programmes) == 0 {
			return fmt.Errorf("epg is required for v1 setup")
		}
		if _, ok := matching.Match(entries[0], doc); !ok {
			return fmt.Errorf("epg does not match playlist entries")
		}
		return nil
	default:
		return fmt.Errorf("source mode %q not implemented", settings.SourceMode)
	}
}

func testDispatcharrDirectConnection(ctx context.Context, client DispatcharrClient) error {
	if err := client.TestConnection(ctx); err != nil {
		return err
	}
	version, err := client.Version(ctx)
	if err != nil {
		return fmt.Errorf("dispatcharr version check failed: %w", err)
	}
	if !dispatcharrVersionAtLeast(version, config.MinimumDispatcharrVersion) {
		return fmt.Errorf("dispatcharr %s or newer is required; connected server is %s", config.MinimumDispatcharrVersion, strings.TrimSpace(version.Version.String()))
	}
	return nil
}

func dispatcharrVersionAtLeast(version dispatcharr.VersionInfo, minimum string) bool {
	return compareVersionStrings(version.Version.String(), minimum) >= 0
}

func compareVersionStrings(current, minimum string) int {
	currentParts := versionParts(current)
	minimumParts := versionParts(minimum)
	for i := 0; i < 3; i++ {
		if currentParts[i] > minimumParts[i] {
			return 1
		}
		if currentParts[i] < minimumParts[i] {
			return -1
		}
	}
	return 0
}

func versionParts(value string) [3]int {
	value = strings.TrimSpace(strings.TrimPrefix(value, "v"))
	pieces := strings.Split(value, ".")
	var parts [3]int
	for i := 0; i < len(pieces) && i < len(parts); i++ {
		numeric := strings.Builder{}
		for _, r := range pieces[i] {
			if r < '0' || r > '9' {
				break
			}
			numeric.WriteRune(r)
		}
		parsed, _ := strconv.Atoi(numeric.String())
		parts[i] = parsed
	}
	return parts
}

func testXtreamConnection(ctx context.Context, client XtreamClient) error {
	if err := client.TestConnection(ctx); err != nil {
		return err
	}
	streams, err := client.LiveStreams(ctx)
	if err != nil {
		return err
	}
	if len(streams) == 0 {
		return fmt.Errorf("no live streams available")
	}
	epg, err := client.ShortEPG(ctx, streams[0].StreamID)
	if err != nil {
		return err
	}
	if len(epg.EPGListings) == 0 {
		return fmt.Errorf("epg is required for v1 setup")
	}
	return nil
}
