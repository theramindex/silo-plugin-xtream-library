package plugin

import (
	"sync"
)

// sportsFlightGroup collapses concurrent loads of the same key into one call,
// a minimal in-package singleflight for the ESPN stats cache.
type sportsFlightGroup struct {
	mu    sync.Mutex
	calls map[string]*sportsFlight
}

type sportsFlight struct {
	done  chan struct{}
	value any
	err   error
}

// do runs load once per key at a time; callers that arrive while it runs wait
// for and share its result.
func (group *sportsFlightGroup) do(key string, load func() (any, error)) (any, error) {
	group.mu.Lock()
	if call, ok := group.calls[key]; ok {
		group.mu.Unlock()
		<-call.done
		return call.value, call.err
	}
	if group.calls == nil {
		group.calls = map[string]*sportsFlight{}
	}
	call := &sportsFlight{done: make(chan struct{})}
	group.calls[key] = call
	group.mu.Unlock()

	defer func() {
		group.mu.Lock()
		delete(group.calls, key)
		group.mu.Unlock()
		close(call.done)
	}()
	call.value, call.err = load()
	return call.value, call.err
}
