package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

// The livetv/v1 contract (testdata/livetv-v1/contract.md) is what Silo's
// Jellyfin bridge calls to list and play channels without the web app.

const livetvContractVersion = 1

type livetvInfo struct {
	ContractVersion int      `json:"contractVersion"`
	Name            string   `json:"name"`
	StreamHosts     []string `json:"streamHosts"`
}

type livetvChannel struct {
	ID      string `json:"id"`
	Number  string `json:"number"`
	Name    string `json:"name"`
	LogoURL string `json:"logoUrl"`
	GroupID string `json:"groupId"`
	Group   string `json:"group"`
}

type livetvStream struct {
	URL       string `json:"url"`
	Container string `json:"container"`
	Mode      string `json:"mode"`
}

type livetvError struct {
	Code string `json:"code"`
}

func (s *HTTPRoutesServer) handleLiveTVInfo(ctx context.Context) *pluginv1.HandleHTTPResponse {
	s.ensureCatalogHydrated(ctx)
	return jsonHTTPResponse(http.StatusOK, livetvInfo{
		ContractVersion: livetvContractVersion,
		Name:            "XC for Silo",
		StreamHosts:     livetvStreamHosts(s.liveTVSettings(), s.store.Current().Catalog.Channels),
	})
}

func (s *HTTPRoutesServer) handleLiveTVChannels(ctx context.Context) *pluginv1.HandleHTTPResponse {
	s.ensureCatalogHydrated(ctx)
	channels := s.store.Current().Catalog.Channels
	lineup := make([]livetvChannel, 0, len(channels))
	for _, channel := range channels {
		lineup = append(lineup, livetvChannel{
			ID:      channel.ID,
			Number:  channel.Number,
			Name:    channel.Name,
			LogoURL: livetvLogoURL(channel.LogoURL),
			GroupID: channel.CategoryID,
			Group:   channel.CategoryName,
		})
	}
	return jsonHTTPResponse(http.StatusOK, lineup)
}

func (s *HTTPRoutesServer) handleLiveTVStream(ctx context.Context, request *pluginv1.HandleHTTPRequest) *pluginv1.HandleHTTPResponse {
	channelID := strings.TrimSpace(queryValue(request, "channel_id"))
	if channelID == "" {
		return textResponse(http.StatusBadRequest, "missing channel_id query parameter")
	}
	format := strings.ToLower(strings.TrimSpace(queryValue(request, "format")))
	// An empty extension keeps the source's own live format.
	ext := ""
	switch format {
	case "":
		format = "any"
	case "any":
	case "ts":
		ext = "ts"
	case "hls":
		ext = "m3u8"
	default:
		return textResponse(http.StatusBadRequest, "format must be ts, hls or any")
	}

	s.ensureCatalogHydrated(ctx)
	streamURL, err := s.resolveStreamURLWithFormat(channelID, ext)
	if err != nil {
		return jsonHTTPResponse(http.StatusNotFound, livetvError{Code: "channel_not_found"})
	}
	streamURL = appendPlaybackQuery(streamURL, request)

	container := "ts"
	if publicStreamFormat(streamURL) == "hls" {
		container = "hls"
	}
	if format != "any" && format != container {
		return jsonHTTPResponse(http.StatusUnprocessableEntity, livetvError{Code: "format_unavailable"})
	}

	mode := "redirect"
	if streamURLExposesSecrets(streamURL, s.liveTVSettings()) {
		mode = "server"
	}
	return jsonHTTPResponse(http.StatusOK, livetvStream{URL: streamURL, Container: container, Mode: mode})
}

func (s *HTTPRoutesServer) liveTVSettings() config.Settings {
	if s.settingsProvider == nil {
		return config.Settings{}
	}
	return s.settingsProvider()
}

// livetvStreamHosts lists every host a stream URL may point at, as host[:port],
// so Silo can refuse to fetch anything else on the first hop.
func livetvStreamHosts(settings config.Settings, channels []model.Channel) []string {
	seen := map[string]bool{}
	add := func(raw string) {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed.Host == "" {
			return
		}
		seen[strings.ToLower(parsed.Host)] = true
	}
	for _, source := range settings.EffectiveXtreamSources() {
		add(source.BaseURL)
	}
	for _, raw := range []string{settings.XtreamBaseURL, settings.M3UURL, settings.DispatcharrURL} {
		add(raw)
	}
	for _, channel := range channels {
		add(channel.StreamURL)
	}
	hosts := make([]string, 0, len(seen))
	for host := range seen {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}

func livetvLogoURL(raw string) string {
	logo := strings.TrimSpace(raw)
	lower := strings.ToLower(logo)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return logo
	}
	return ""
}

func jsonHTTPResponse(status int, value any) *pluginv1.HandleHTTPResponse {
	payload, err := json.Marshal(value)
	if err != nil {
		return textResponse(http.StatusInternalServerError, "unable to encode response")
	}
	return &pluginv1.HandleHTTPResponse{
		StatusCode: int32(status),
		Headers: map[string]string{
			"cache-control": "no-store",
			"content-type":  "application/json",
		},
		Body: payload,
	}
}
