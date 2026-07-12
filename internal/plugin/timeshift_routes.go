package plugin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
	"github.com/theramindex/silo-plugin-dispatcharr/internal/timeshift"
)

type timeShiftStartRequest struct {
	ChannelID string `json:"channelId"`
}

type timeShiftLeaseRequest struct {
	LeaseID string `json:"leaseId"`
}

func (s *HTTPRoutesServer) timeShiftConfig() timeshift.Config {
	settings := map[string]any{}
	_ = json.Unmarshal(s.store.AdminSettings(), &settings)
	config := timeshift.Config{
		Enabled:      boolSetting(settings, "liveRewindEnabled", false),
		MaxBytes:     int64(numberSetting(settings, "liveRewindCacheGB", 5) * float64(1<<30)),
		MinFreeBytes: int64(numberSetting(settings, "liveRewindMinFreeGB", 2) * float64(1<<30)),
		MaxBuffers:   int(numberSetting(settings, "liveRewindMaxChannels", 20)),
		Window:       time.Duration(numberSetting(settings, "liveRewindWindowMinutes", 30)) * time.Minute,
		IdleTTL:      2 * time.Minute,
		LeaseTTL:     75 * time.Second,
	}
	s.timeShift.SetConfig(config)
	return config
}

func (s *HTTPRoutesServer) timeShiftSourceAllowed() bool {
	if s.settingsProvider == nil {
		return false
	}
	mode := s.settingsProvider().SourceMode
	return mode == config.SourceModeDirectLogin || mode == config.SourceModeAPIKey
}

func (s *HTTPRoutesServer) handleTimeShiftStart(request *pluginv1.HandleHTTPRequest) *pluginv1.HandleHTTPResponse {
	if request.GetMethod() != http.MethodPost {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed")
	}
	config := s.timeShiftConfig()
	if !config.Enabled || !s.timeShiftSourceAllowed() {
		return textResponse(http.StatusConflict, "live rewind is unavailable for this source")
	}
	var payload timeShiftStartRequest
	if err := json.Unmarshal(request.GetBody(), &payload); err != nil || strings.TrimSpace(payload.ChannelID) == "" {
		return textResponse(http.StatusBadRequest, "missing channelId")
	}
	streamURL, err := s.resolveStreamURL(payload.ChannelID)
	if err != nil {
		return textResponse(http.StatusNotFound, err.Error())
	}
	if strings.Contains(strings.ToLower(streamURL), ".m3u8") {
		return textResponse(http.StatusConflict, "live rewind currently supports MPEG-TS channels only")
	}
	lease, err := s.timeShift.Acquire(payload.ChannelID, streamURL, config)
	if err != nil {
		return textResponse(http.StatusBadGateway, err.Error())
	}
	return jsonHTTPResponse(http.StatusAccepted, map[string]any{
		"leaseId":      lease.ID,
		"bufferId":     lease.BufferID,
		"manifestPath": "/dispatcharr/timeshift/" + lease.BufferID + "/index.m3u8?lease=" + lease.ID,
	})
}

func (s *HTTPRoutesServer) handleTimeShiftStatus(request *pluginv1.HandleHTTPRequest) *pluginv1.HandleHTTPResponse {
	leaseID := queryValue(request, "lease_id")
	status, ok := s.timeShift.Status(leaseID)
	if !ok {
		return textResponse(http.StatusNotFound, "rewind session not found")
	}
	return jsonHTTPResponse(http.StatusOK, status)
}

func (s *HTTPRoutesServer) handleTimeShiftHeartbeat(request *pluginv1.HandleHTTPRequest) *pluginv1.HandleHTTPResponse {
	var payload timeShiftLeaseRequest
	if request.GetMethod() != http.MethodPost || json.Unmarshal(request.GetBody(), &payload) != nil || !s.timeShift.Heartbeat(payload.LeaseID) {
		return textResponse(http.StatusNotFound, "rewind session not found")
	}
	return jsonHTTPResponse(http.StatusOK, map[string]bool{"ok": true})
}

func (s *HTTPRoutesServer) handleTimeShiftStop(request *pluginv1.HandleHTTPRequest) *pluginv1.HandleHTTPResponse {
	var payload timeShiftLeaseRequest
	if request.GetMethod() != http.MethodPost || json.Unmarshal(request.GetBody(), &payload) != nil {
		return textResponse(http.StatusBadRequest, "invalid rewind session")
	}
	s.timeShift.Release(payload.LeaseID)
	return jsonHTTPResponse(http.StatusOK, map[string]bool{"ok": true})
}

func (s *HTTPRoutesServer) handleTimeShiftAdminStatus(request *pluginv1.HandleHTTPRequest) *pluginv1.HandleHTTPResponse {
	if !s.adminSettingsAuthorized(request) {
		return textResponse(http.StatusForbidden, "Silo administrator access is required")
	}
	s.timeShiftConfig()
	return jsonHTTPResponse(http.StatusOK, s.timeShift.Stats())
}

func (s *HTTPRoutesServer) handleTimeShiftClear(request *pluginv1.HandleHTTPRequest) *pluginv1.HandleHTTPResponse {
	if request.GetMethod() != http.MethodPost || !s.adminSettingsAuthorized(request) {
		return textResponse(http.StatusForbidden, "Silo administrator access is required")
	}
	if err := s.timeShift.Clear(); err != nil {
		return textResponse(http.StatusInternalServerError, "could not clear rewind cache")
	}
	return jsonHTTPResponse(http.StatusOK, map[string]bool{"ok": true})
}

func (s *HTTPRoutesServer) handleTimeShiftMedia(request *pluginv1.HandleHTTPRequest) *pluginv1.HandleHTTPResponse {
	relative := strings.TrimPrefix(request.GetPath(), "/dispatcharr/timeshift/")
	parts := strings.Split(relative, "/")
	leaseID := queryValue(request, "lease")
	if len(parts) == 2 && parts[1] == "index.m3u8" {
		manifest, ok := s.timeShift.Manifest(parts[0], leaseID)
		if !ok {
			return textResponse(http.StatusTooEarly, "rewind buffer is not ready")
		}
		return mediaHTTPResponse(http.StatusOK, "application/vnd.apple.mpegurl", manifest, "no-store")
	}
	if len(parts) == 3 && parts[1] == "segment" && strings.HasSuffix(parts[2], ".ts") {
		seq, err := strconv.ParseInt(strings.TrimSuffix(parts[2], ".ts"), 10, 64)
		if err != nil {
			return textResponse(http.StatusBadRequest, "invalid rewind segment")
		}
		segment, ok := s.timeShift.Segment(parts[0], leaseID, seq)
		if !ok {
			return textResponse(http.StatusNotFound, "rewind segment not found")
		}
		return mediaHTTPResponse(http.StatusOK, "video/mp2t", segment, "public, max-age=300, immutable")
	}
	return textResponse(http.StatusNotFound, "rewind route not found")
}

func jsonHTTPResponse(status int, value any) *pluginv1.HandleHTTPResponse {
	body, err := json.Marshal(value)
	if err != nil {
		return textResponse(http.StatusInternalServerError, fmt.Sprintf("encode response: %v", err))
	}
	return mediaHTTPResponse(status, "application/json", body, "no-store")
}

func mediaHTTPResponse(status int, contentType string, body []byte, cacheControl string) *pluginv1.HandleHTTPResponse {
	return &pluginv1.HandleHTTPResponse{StatusCode: int32(status), Headers: map[string]string{
		"content-type": contentType, "cache-control": cacheControl,
	}, Body: body}
}

func boolSetting(settings map[string]any, key string, fallback bool) bool {
	value, ok := settings[key].(bool)
	if !ok {
		return fallback
	}
	return value
}

func numberSetting(settings map[string]any, key string, fallback float64) float64 {
	value, ok := settings[key].(float64)
	if !ok {
		return fallback
	}
	return value
}
