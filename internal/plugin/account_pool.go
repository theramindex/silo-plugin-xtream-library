package plugin

import (
	"fmt"
	"sync"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
)

const accountLeaseTTL = 90 * time.Second

type accountLease struct {
	sourceID  string
	accountID string
	seenAt    time.Time
}

type accountPool struct {
	mu     sync.Mutex
	now    func() time.Time
	leases map[string]accountLease
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
		if account.ConnectionLimit > 0 && usage[account.ID] >= account.ConnectionLimit {
			continue
		}
		if selected < 0 || usage[account.ID] < usage[accounts[selected].ID] {
			selected = index
		}
	}
	if selected < 0 {
		return config.XtreamAccount{}, fmt.Errorf("all playback accounts for %q are at their connection limit", source.Name)
	}
	account := accounts[selected]
	p.leases[sessionID] = accountLease{sourceID: source.ID, accountID: account.ID, seenAt: now}
	return account, nil
}

func (p *accountPool) Touch(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if lease, ok := p.leases[sessionID]; ok {
		lease.seenAt = p.now()
		p.leases[sessionID] = lease
	}
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
	for sessionID, lease := range p.leases {
		if now.Sub(lease.seenAt) > accountLeaseTTL {
			delete(p.leases, sessionID)
		}
	}
}
