package plugin

import (
	"testing"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
)

func TestAccountPoolLeasesLeastUsedCompatibleAccountWithinLimit(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	pool := newAccountPool(func() time.Time { return now })
	source := config.XtreamSource{ID: "frost", Accounts: []config.XtreamAccount{
		{ID: "a", Username: "first", Password: "one", Enabled: true, Compatible: true, ConnectionLimit: 1},
		{ID: "b", Username: "second", Password: "two", Enabled: true, Compatible: true, ConnectionLimit: 2},
	}}
	first, err := pool.Lease("session-1", source)
	if err != nil || first.ID != "a" {
		t.Fatalf("first lease: account=%+v err=%v", first, err)
	}
	second, err := pool.Lease("session-2", source)
	if err != nil || second.ID != "b" {
		t.Fatalf("second lease: account=%+v err=%v", second, err)
	}
	sticky, err := pool.Lease("session-1", source)
	if err != nil || sticky.ID != "a" {
		t.Fatalf("session must remain sticky: account=%+v err=%v", sticky, err)
	}
	third, err := pool.Lease("session-3", source)
	if err != nil || third.ID != "b" {
		t.Fatalf("third lease should use remaining capacity: account=%+v err=%v", third, err)
	}
	if _, err := pool.Lease("session-4", source); err == nil {
		t.Fatal("expected pool exhaustion after configured connection limits")
	}
	pool.Release("session-1")
	reused, err := pool.Lease("session-4", source)
	if err != nil || reused.ID != "a" {
		t.Fatalf("released capacity should be reusable: account=%+v err=%v", reused, err)
	}
}

func TestAccountPoolExpiresAbandonedSessionLeases(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	pool := newAccountPool(func() time.Time { return now })
	source := config.XtreamSource{ID: "frost", Accounts: []config.XtreamAccount{{
		ID: "only", Username: "viewer", Password: "secret", Enabled: true, Compatible: true, ConnectionLimit: 1,
	}}}
	if _, err := pool.Lease("abandoned", source); err != nil {
		t.Fatalf("initial lease: %v", err)
	}
	now = now.Add(accountLeaseTTL + time.Second)
	if account, err := pool.Lease("replacement", source); err != nil || account.ID != "only" {
		t.Fatalf("expired lease should release capacity: account=%+v err=%v", account, err)
	}
}
