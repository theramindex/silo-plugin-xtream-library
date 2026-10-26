package plugin

import (
	"context"
	"sync"
)

// sportsFlightGroup collapses concurrent loads of the same key into one call,
// a minimal in-package singleflight for the ESPN caches.
type sportsFlightGroup struct {
	mu    sync.Mutex
	calls map[string]*sportsFlight
}

type sportsFlight struct {
	done  chan struct{}
	value any
	err   error
}

// do shares one bounded load per key. Each caller may stop waiting without
// canceling the shared fetch that can still populate the cache for other users.
func (group *sportsFlightGroup) do(ctx context.Context, key string, load func() (any, error)) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	group.mu.Lock()
	call, exists := group.calls[key]
	if !exists {
		if group.calls == nil {
			group.calls = map[string]*sportsFlight{}
		}
		call = &sportsFlight{done: make(chan struct{})}
		group.calls[key] = call
		go func() {
			call.value, call.err = load()
			group.mu.Lock()
			delete(group.calls, key)
			group.mu.Unlock()
			close(call.done)
		}()
	}
	group.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-call.done:
		return call.value, call.err
	}
}
