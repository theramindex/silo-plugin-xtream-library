package plugin

import (
	"context"
	"strings"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/theramindex/silo-plugin-xtream-library/internal/app"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	SyncTaskKey           = "xtream-sync"
	ChannelRefreshTaskKey = "xtream-refresh-channels"
	EPGRefreshTaskKey     = "xtream-refresh-epg"
)

type ScheduledTaskServer struct {
	pluginv1.UnimplementedScheduledTaskServer
	coordinator      *RefreshCoordinator
	settingsProvider func() config.Settings
}

func NewScheduledTaskServer(service *app.Service, settings config.Settings) *ScheduledTaskServer {
	return &ScheduledTaskServer{coordinator: NewRefreshCoordinator(service), settingsProvider: func() config.Settings { return settings }}
}

func NewScheduledTaskServerWithProvider(service *app.Service, provider func() config.Settings) *ScheduledTaskServer {
	return &ScheduledTaskServer{coordinator: NewRefreshCoordinator(service), settingsProvider: provider}
}

func NewScheduledTaskServerWithCoordinator(coordinator *RefreshCoordinator, provider func() config.Settings) *ScheduledTaskServer {
	return &ScheduledTaskServer{coordinator: coordinator, settingsProvider: provider}
}

func (s *ScheduledTaskServer) Run(ctx context.Context, request *pluginv1.RunScheduledTaskRequest) (*pluginv1.RunScheduledTaskResponse, error) {
	taskKey := request.GetTaskKey()
	taskKind := "unknown"
	operation := RefreshCatalog
	switch {
	case isTaskKey(taskKey, SyncTaskKey):
		taskKind = "catalog"
	case isTaskKey(taskKey, ChannelRefreshTaskKey):
		taskKind = "channels"
		operation = RefreshChannels
	case isTaskKey(taskKey, EPGRefreshTaskKey):
		taskKind = "epg"
		operation = RefreshGuide
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown scheduled task %q", taskKey)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	settings := s.settingsProvider()
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	if err := s.coordinator.Run(ctx, operation, settings, time.Now().Unix()); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	job := s.coordinator.Status()

	output, err := structpb.NewStruct(map[string]any{
		"status":    "completed",
		"task":      taskKind,
		"jobId":     job.ID,
		"operation": string(job.Operation),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.RunScheduledTaskResponse{Output: output}, nil
}

func isTaskKey(taskKey string, capabilityID string) bool {
	return taskKey == capabilityID || strings.HasSuffix(taskKey, ":"+capabilityID)
}
