package plugin

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
)

const accountLeaseTTL = 90 * time.Second

// errAccountPoolFull means every playback account is at its connection limit.
var errAccountPoolFull = errors.New("all playback accounts are at their connection limit")

type accountLease struct {
	sourceID  string
	accountID string
	seenAt    time.Time
}

type accountPool struct {
	mu       sync.Mutex
	now      func() time.Time
	leases   map[string]accountLease
	cooldown map[string]time.Time
}

func newAccountPool(now func() time.Time) *accountPool {
	if now == nil {
		now = time.Now
	}
	return &accountPool{now: now, leases: map[string]accountLease{}}
}

func (p *accountPool) Lease(sessionID string, source config.XtreamSource) (config.XtreamAccount, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.prune(now)
	accounts := source.EffectivePlaybackAccounts()
	if len(accounts) == 0 {
		return config.XtreamAccount{}, fmt.Errorf("provider %q has no compatible playback accounts", source.Name)
	}
	if existing, ok := p.leases[sessionID]; ok && existing.sourceID == source.ID {
		for _, account := range accounts {
			if account.ID == existing.accountID {
				existing.seenAt = now
				p.leases[sessionID] = existing
				return account, nil
			}
		}
	}
	usage := map[string]int{}
	for _, lease := range p.leases {
		if lease.sourceID == source.ID {
			usage[lease.accountID]++
		}
	}
	selected := -1
	for index, account := range accounts {
		if now.Before(p.cooldown[source.ID+"/"+account.ID]) {
			continue
		}
		if account.ConnectionLimit > 0 && usage[account.ID] >= account.ConnectionLimit {
			continue
		}
		if selected < 0 || usage[account.ID] < usage[accounts[selected].ID] {
			selected = index
		}
	}
	if selected < 0 {
		return config.XtreamAccount{}, fmt.Errorf("provider %q: %w", source.Name, errAccountPoolFull)
	}
	account := accounts[selected]
	p.leases[sessionID] = accountLease{sourceID: source.ID, accountID: account.ID, seenAt: now}
	return account, nil
}

// Touch renews a lease and reports whether the session still holds one.
func (p *accountPool) Touch(sessionID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.prune(now)
	lease, ok := p.leases[sessionID]
	if ok {
		lease.seenAt = now
		p.leases[sessionID] = lease
	}
	return ok
}

func (p *accountPool) Release(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.leases, sessionID)
}

func (p *accountPool) Usage(sourceID string) map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(p.now())
	result := map[string]int{}
	for _, lease := range p.leases {
		if lease.sourceID == sourceID {
			result[lease.accountID]++
		}
	}
	return result
}

func (p *accountPool) prune(now time.Time) {
	for id, until := range p.cooldown {
		if !now.Before(until) {
			delete(p.cooldown, id)
		}
	}
	for sessionID, lease := range p.leases {
		if now.Sub(lease.seenAt) > accountLeaseTTL {
			delete(p.leases, sessionID)
		}
	}
}

func (p *accountPool) Reject(session string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if lease, ok := p.leases[session]; ok {
		if p.cooldown == nil {
			p.cooldown = map[string]time.Time{}
		}
		p.cooldown[lease.sourceID+"/"+lease.accountID] = p.now().Add(15 * time.Second)
		delete(p.leases, session)
	}
}
