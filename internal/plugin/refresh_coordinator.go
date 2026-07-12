package plugin

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/config"
)

type RefreshOperation string

const (
	RefreshCatalog  RefreshOperation = "catalog"
	RefreshChannels RefreshOperation = "channels"
	RefreshGuide    RefreshOperation = "guide"
	RefreshForce    RefreshOperation = "force"
)

type RefreshState string

const (
	RefreshIdle      RefreshState = "idle"
	RefreshQueued    RefreshState = "queued"
	RefreshRunning   RefreshState = "running"
	RefreshSucceeded RefreshState = "succeeded"
	RefreshFailed    RefreshState = "failed"
	RefreshCanceled  RefreshState = "canceled"
)

type RefreshJob struct {
	ID              string           `json:"id,omitempty"`
	Operation       RefreshOperation `json:"operation,omitempty"`
	State           RefreshState     `json:"state"`
	ConfigKey       string           `json:"-"`
	StartedAtUnix   int64            `json:"startedAtUnix,omitempty"`
	CompletedAtUnix int64            `json:"completedAtUnix,omitempty"`
	Error           string           `json:"error,omitempty"`
}

type channelOnlySyncer interface {
	RefreshChannelsNow(ctx context.Context, settings config.Settings, nowUnix int64) error
}

type RefreshCoordinator struct {
	target catalogSyncer

	runMu  sync.Mutex
	mu     sync.Mutex
	job    RefreshJob
	run    *refreshExecution
	cancel context.CancelFunc
	closed bool
	seq    uint64
}

type refreshExecution struct {
	done chan struct{}
	mu   sync.Mutex
	err  error
}

func newRefreshExecution() *refreshExecution {
	return &refreshExecution{done: make(chan struct{})}
}

func (r *refreshExecution) complete(err error) {
	r.mu.Lock()
	r.err = err
	r.mu.Unlock()
	close(r.done)
}

func (r *refreshExecution) result() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

func NewRefreshCoordinator(target catalogSyncer) *RefreshCoordinator {
	return &RefreshCoordinator{target: target, job: RefreshJob{State: RefreshIdle}}
}

func (c *RefreshCoordinator) Start(operation RefreshOperation, settings config.Settings) (RefreshJob, bool) {
	if c == nil || c.target == nil {
		return RefreshJob{State: RefreshFailed, Error: "catalog sync is not available"}, false
	}

	configKey := config.CatalogCacheKey(settings)
	c.mu.Lock()
	if c.closed {
		job := c.job
		c.mu.Unlock()
		return job, false
	}
	if isActiveRefresh(c.job.State) && c.job.ConfigKey == configKey {
		if refreshOperationCovers(c.job.Operation, operation) || refreshOperationPriority(c.job.Operation) >= refreshOperationPriority(operation) {
			job := c.job
			c.mu.Unlock()
			return job, false
		}
	}
	if c.cancel != nil {
		c.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	job, run := c.nextJobLocked(operation, configKey)
	c.cancel = cancel
	c.mu.Unlock()

	go c.execute(ctx, cancel, job, run, settings, time.Now().Unix())
	return job, true
}

func refreshOperationPriority(operation RefreshOperation) int {
	switch operation {
	case RefreshForce:
		return 4
	case RefreshCatalog:
		return 3
	case RefreshChannels:
		return 2
	case RefreshGuide:
		return 1
	default:
		return 0
	}
}

func (c *RefreshCoordinator) Run(ctx context.Context, operation RefreshOperation, settings config.Settings, nowUnix int64) error {
	if c == nil || c.target == nil {
		return fmt.Errorf("catalog sync is not available")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	configKey := config.CatalogCacheKey(settings)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("refresh coordinator is closed")
	}
	if isActiveRefresh(c.job.State) && c.job.ConfigKey == configKey {
		activeOperation := c.job.Operation
		run := c.run
		c.mu.Unlock()
		if err := c.waitFor(ctx, run); err != nil {
			return err
		}
		if refreshOperationCovers(activeOperation, operation) {
			return nil
		}
		return c.Run(ctx, operation, settings, nowUnix)
	}
	if c.cancel != nil {
		c.cancel()
	}
	runCtx, cancel := context.WithCancel(ctx)
	job, run := c.nextJobLocked(operation, configKey)
	c.cancel = cancel
	c.mu.Unlock()

	c.execute(runCtx, cancel, job, run, settings, nowUnix)
	return run.result()
}

func (c *RefreshCoordinator) Status() RefreshJob {
	if c == nil {
		return RefreshJob{State: RefreshIdle}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.job
}

func (c *RefreshCoordinator) Wait(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	run := c.run
	c.mu.Unlock()
	return c.waitFor(ctx, run)
}

func (c *RefreshCoordinator) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	run := c.run
	c.mu.Unlock()
	if run != nil {
		<-run.done
	}
}

func (c *RefreshCoordinator) nextJobLocked(operation RefreshOperation, configKey string) (RefreshJob, *refreshExecution) {
	job := RefreshJob{
		ID:        strconv.FormatUint(atomic.AddUint64(&c.seq, 1), 10),
		Operation: operation,
		State:     RefreshQueued,
		ConfigKey: configKey,
	}
	run := newRefreshExecution()
	c.job = job
	c.run = run
	return job, run
}

func (c *RefreshCoordinator) execute(ctx context.Context, cancel context.CancelFunc, job RefreshJob, run *refreshExecution, settings config.Settings, nowUnix int64) {
	defer cancel()

	c.runMu.Lock()
	defer c.runMu.Unlock()
	if err := ctx.Err(); err != nil {
		c.finish(job.ID, RefreshCanceled, err)
		run.complete(err)
		return
	}
	c.markRunning(job.ID)
	err := c.runOperation(ctx, job.Operation, settings, nowUnix)
	if err == nil {
		c.finish(job.ID, RefreshSucceeded, nil)
		run.complete(nil)
		return
	}
	if ctx.Err() != nil {
		c.finish(job.ID, RefreshCanceled, ctx.Err())
		run.complete(ctx.Err())
		return
	}
	c.finish(job.ID, RefreshFailed, err)
	run.complete(err)
}

func (c *RefreshCoordinator) runOperation(ctx context.Context, operation RefreshOperation, settings config.Settings, nowUnix int64) error {
	switch operation {
	case RefreshChannels:
		if target, ok := c.target.(channelOnlySyncer); ok {
			return target.RefreshChannelsNow(ctx, settings, nowUnix)
		}
	case RefreshGuide:
		if target, ok := c.target.(guideOnlySyncer); ok {
			return target.RefreshGuideOnlyNow(ctx, settings, nowUnix)
		}
	case RefreshForce:
		if target, ok := c.target.(forceCatalogSyncer); ok {
			return target.ForceSyncNow(ctx, settings, nowUnix)
		}
	}
	return c.target.SyncNow(ctx, settings, nowUnix)
}

func (c *RefreshCoordinator) markRunning(jobID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.job.ID != jobID {
		return
	}
	c.job.State = RefreshRunning
	c.job.StartedAtUnix = time.Now().Unix()
}

func (c *RefreshCoordinator) finish(jobID string, state RefreshState, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.job.ID != jobID {
		return
	}
	c.job.State = state
	c.job.CompletedAtUnix = time.Now().Unix()
	if err != nil {
		c.job.Error = err.Error()
	} else {
		c.job.Error = ""
	}
	c.cancel = nil
}

func (c *RefreshCoordinator) waitFor(ctx context.Context, run *refreshExecution) error {
	if run == nil {
		return nil
	}
	select {
	case <-run.done:
		return run.result()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func isActiveRefresh(state RefreshState) bool {
	return state == RefreshQueued || state == RefreshRunning
}

func refreshOperationCovers(active RefreshOperation, requested RefreshOperation) bool {
	if active == requested || active == RefreshForce {
		return true
	}
	return active == RefreshCatalog && requested != RefreshForce
}
