package plugin

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
)

func TestRefreshCoordinatorCancelsObsoleteConfigAndSerializesJobs(t *testing.T) {
	t.Parallel()

	target := &blockingRefreshTarget{started: make(chan string, 2)}
	coordinator := NewRefreshCoordinator(target)
	t.Cleanup(coordinator.Close)

	oldSettings := config.Settings{SourceMode: config.SourceModeAPIKey, DispatcharrURL: "https://old.example", DispatcharrAPIKey: "old"}
	newSettings := config.Settings{SourceMode: config.SourceModeAPIKey, DispatcharrURL: "https://new.example", DispatcharrAPIKey: "new"}
	oldJob, started := coordinator.Start(RefreshForce, oldSettings)
	if !started || oldJob.ID == "" {
		t.Fatalf("expected old refresh to start, got %+v", oldJob)
	}
	if got := waitForRefreshStart(t, target.started); got != oldSettings.DispatcharrURL {
		t.Fatalf("expected old refresh first, got %q", got)
	}

	newJob, started := coordinator.Start(RefreshForce, newSettings)
	if !started || newJob.ID == oldJob.ID {
		t.Fatalf("expected replacement refresh, got %+v", newJob)
	}
	if got := waitForRefreshStart(t, target.started); got != newSettings.DispatcharrURL {
		t.Fatalf("expected new refresh after cancellation, got %q", got)
	}
	if err := coordinator.Wait(t.Context()); err != nil {
		t.Fatalf("wait for refresh: %v", err)
	}

	status := coordinator.Status()
	if status.ID != newJob.ID || status.State != RefreshSucceeded {
		t.Fatalf("unexpected final status: %+v", status)
	}
	if target.maximumActive() != 1 {
		t.Fatalf("expected serialized refreshes, max active was %d", target.maximumActive())
	}
}

func TestRefreshCoordinatorWaiterReceivesItsReplacedJobCancellation(t *testing.T) {
	t.Parallel()

	target := &blockingRefreshTarget{started: make(chan string, 2)}
	coordinator := NewRefreshCoordinator(target)
	t.Cleanup(coordinator.Close)
	oldSettings := config.Settings{SourceMode: config.SourceModeAPIKey, DispatcharrURL: "https://old.example", DispatcharrAPIKey: "old"}
	newSettings := config.Settings{SourceMode: config.SourceModeAPIKey, DispatcharrURL: "https://new.example", DispatcharrAPIKey: "new"}
	oldResult := make(chan error, 1)
	go func() { oldResult <- coordinator.Run(t.Context(), RefreshForce, oldSettings, 100) }()
	waitForRefreshStart(t, target.started)

	if _, started := coordinator.Start(RefreshForce, newSettings); !started {
		t.Fatal("expected replacement refresh to start")
	}
	waitForRefreshStart(t, target.started)
	if err := <-oldResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected replaced waiter to receive cancellation, got %v", err)
	}
	if err := coordinator.Wait(t.Context()); err != nil {
		t.Fatalf("wait for replacement refresh: %v", err)
	}
}

func TestRefreshCoordinatorRunsChannelOnlyOperation(t *testing.T) {
	t.Parallel()

	target := &countingRefreshTarget{}
	coordinator := NewRefreshCoordinator(target)
	t.Cleanup(coordinator.Close)
	settings := config.Settings{SourceMode: config.SourceModeAPIKey, DispatcharrURL: "https://dispatcharr.example", DispatcharrAPIKey: "secret"}

	if err := coordinator.Run(t.Context(), RefreshChannels, settings, 100); err != nil {
		t.Fatalf("run channel refresh: %v", err)
	}
	if target.channelCalls != 1 || target.catalogCalls != 0 {
		t.Fatalf("expected channel-only refresh, got channels=%d catalog=%d", target.channelCalls, target.catalogCalls)
	}
}

func TestRefreshOperationCoverageKeepsForceRefreshDistinct(t *testing.T) {
	t.Parallel()

	if !refreshOperationCovers(RefreshCatalog, RefreshGuide) {
		t.Fatal("catalog refresh should cover guide refresh")
	}
	if refreshOperationCovers(RefreshCatalog, RefreshForce) {
		t.Fatal("ordinary catalog refresh must not cover an explicit force refresh")
	}
	if !refreshOperationCovers(RefreshForce, RefreshChannels) {
		t.Fatal("force refresh should cover channel refresh")
	}
}

func TestRefreshCoordinatorDoesNotReplaceFullRefreshWithGuideWarm(t *testing.T) {
	t.Parallel()

	target := &controlledRefreshTarget{started: make(chan RefreshOperation, 2), release: make(chan struct{})}
	coordinator := NewRefreshCoordinator(target)
	t.Cleanup(coordinator.Close)
	settings := config.Settings{SourceMode: config.SourceModeAPIKey, DispatcharrURL: "https://dispatcharr.example", DispatcharrAPIKey: "secret"}
	if _, started := coordinator.Start(RefreshForce, settings); !started {
		t.Fatal("expected full refresh to start")
	}
	if operation := <-target.started; operation != RefreshForce {
		t.Fatalf("expected force refresh, got %s", operation)
	}
	if job, started := coordinator.Start(RefreshGuide, settings); started || job.Operation != RefreshForce {
		t.Fatalf("guide warm replaced full refresh: job=%+v started=%v", job, started)
	}
	close(target.release)
	if err := coordinator.Wait(t.Context()); err != nil {
		t.Fatalf("wait for full refresh: %v", err)
	}
}

func TestRefreshCoordinatorDoesNotCancelChannelRecoveryForGuideWarm(t *testing.T) {
	t.Parallel()

	target := &controlledRefreshTarget{started: make(chan RefreshOperation, 2), release: make(chan struct{})}
	coordinator := NewRefreshCoordinator(target)
	t.Cleanup(coordinator.Close)
	settings := config.Settings{SourceMode: config.SourceModeAPIKey, DispatcharrURL: "https://dispatcharr.example", DispatcharrAPIKey: "secret"}
	channelJob, started := coordinator.Start(RefreshChannels, settings)
	if !started {
		t.Fatal("expected channel recovery to start")
	}
	if operation := <-target.started; operation != RefreshChannels {
		t.Fatalf("expected channel refresh, got %s", operation)
	}
	if job, started := coordinator.Start(RefreshGuide, settings); started || job.ID != channelJob.ID {
		t.Fatalf("guide warm canceled channel recovery: job=%+v started=%v", job, started)
	}
	select {
	case operation := <-target.started:
		t.Fatalf("unexpected replacement refresh started: %s", operation)
	case <-time.After(50 * time.Millisecond):
	}
	close(target.release)
	if err := coordinator.Wait(t.Context()); err != nil {
		t.Fatalf("wait for channel recovery: %v", err)
	}
}

func TestRefreshCoordinatorSerializesDifferentTasksForSameConfig(t *testing.T) {
	t.Parallel()

	target := &controlledRefreshTarget{started: make(chan RefreshOperation, 2), release: make(chan struct{})}
	coordinator := NewRefreshCoordinator(target)
	t.Cleanup(coordinator.Close)
	settings := config.Settings{SourceMode: config.SourceModeAPIKey, DispatcharrURL: "https://dispatcharr.example", DispatcharrAPIKey: "secret"}
	if _, started := coordinator.Start(RefreshGuide, settings); !started {
		t.Fatal("expected guide refresh to start")
	}
	if operation := <-target.started; operation != RefreshGuide {
		t.Fatalf("expected guide refresh, got %s", operation)
	}
	result := make(chan error, 1)
	go func() { result <- coordinator.Run(t.Context(), RefreshChannels, settings, 100) }()
	select {
	case operation := <-target.started:
		t.Fatalf("channel refresh started before guide completed: %s", operation)
	case <-time.After(50 * time.Millisecond):
	}
	close(target.release)
	if operation := waitForRefreshOperation(t, target.started); operation != RefreshChannels {
		t.Fatalf("expected queued channel refresh, got %s", operation)
	}
	if err := <-result; err != nil {
		t.Fatalf("channel refresh failed: %v", err)
	}
}

func waitForRefreshStart(t *testing.T, started <-chan string) string {
	t.Helper()
	select {
	case value := <-started:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for refresh start")
		return ""
	}
}

type blockingRefreshTarget struct {
	mu        sync.Mutex
	active    int
	maxActive int
	started   chan string
}

func (t *blockingRefreshTarget) maximumActive() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.maxActive
}

func (t *blockingRefreshTarget) run(ctx context.Context, settings config.Settings) error {
	t.mu.Lock()
	t.active++
	if t.active > t.maxActive {
		t.maxActive = t.active
	}
	t.mu.Unlock()
	t.started <- settings.DispatcharrURL

	if settings.DispatcharrURL == "https://old.example" {
		<-ctx.Done()
	}

	t.mu.Lock()
	t.active--
	t.mu.Unlock()
	return ctx.Err()
}

func (t *blockingRefreshTarget) SyncNow(ctx context.Context, settings config.Settings, _ int64) error {
	return t.run(ctx, settings)
}

func (t *blockingRefreshTarget) ForceSyncNow(ctx context.Context, settings config.Settings, _ int64) error {
	return t.run(ctx, settings)
}

func (t *blockingRefreshTarget) RefreshGuideOnlyNow(ctx context.Context, settings config.Settings, _ int64) error {
	return t.run(ctx, settings)
}

func (t *blockingRefreshTarget) RefreshChannelsNow(ctx context.Context, settings config.Settings, _ int64) error {
	return t.run(ctx, settings)
}

type countingRefreshTarget struct {
	catalogCalls int
	channelCalls int
}

type controlledRefreshTarget struct {
	started chan RefreshOperation
	release chan struct{}
}

func (t *controlledRefreshTarget) run(ctx context.Context, operation RefreshOperation) error {
	t.started <- operation
	select {
	case <-t.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *controlledRefreshTarget) SyncNow(ctx context.Context, _ config.Settings, _ int64) error {
	return t.run(ctx, RefreshCatalog)
}

func (t *controlledRefreshTarget) ForceSyncNow(ctx context.Context, _ config.Settings, _ int64) error {
	return t.run(ctx, RefreshForce)
}

func (t *controlledRefreshTarget) RefreshGuideOnlyNow(ctx context.Context, _ config.Settings, _ int64) error {
	return t.run(ctx, RefreshGuide)
}

func (t *controlledRefreshTarget) RefreshChannelsNow(ctx context.Context, _ config.Settings, _ int64) error {
	return t.run(ctx, RefreshChannels)
}

func waitForRefreshOperation(t *testing.T, started <-chan RefreshOperation) RefreshOperation {
	t.Helper()
	select {
	case operation := <-started:
		return operation
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for refresh operation")
		return ""
	}
}

func (t *countingRefreshTarget) SyncNow(context.Context, config.Settings, int64) error {
	t.catalogCalls++
	return nil
}

func (t *countingRefreshTarget) RefreshChannelsNow(context.Context, config.Settings, int64) error {
	t.channelCalls++
	return nil
}
