package adapters

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrDuplicateLoanRequest is returned by the loan adapter's dedupe gate when
// the same user submits the same (payout_method, principal_amount) tuple
// within the dedupe window. The USSD layer can present a friendly retry
// message instead of double-disbursing.
var ErrDuplicateLoanRequest = errors.New("duplicate loan request within dedupe window")

// dedupeGate is a small in-memory TTL set used to suppress accidental
// double-submits from USSD retries or carrier replays. The window is short
// (seconds) — anything beyond that is a genuinely new request.
//
// In-memory is intentional: dedupe is a best-effort guard against the
// fast-path replay case, not a strong invariant. A persistent guard would
// belong on the loans table as a unique index or in Redis; until USSD
// retries cause real pain that's premature.
type dedupeGate struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
}

func newDedupeGate(ttl time.Duration) *dedupeGate {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &dedupeGate{seen: make(map[string]time.Time), ttl: ttl}
}

// check reserves the key if it hasn't been seen within the TTL. Returns
// true when the caller may proceed; false when the request is a duplicate.
// Expired entries are swept on each call to keep the map bounded.
func (g *dedupeGate) check(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	for k, t := range g.seen {
		if now.Sub(t) > g.ttl {
			delete(g.seen, k)
		}
	}

	if _, exists := g.seen[key]; exists {
		return false
	}
	g.seen[key] = now
	return true
}

// dedupeKey is the canonical key shape: a user can't have two identical
// requests in flight at once, but they can have two different ones (e.g.
// different amount or different payout method).
func dedupeKey(userID, payoutMethod string, principalStroops int64) string {
	return fmt.Sprintf("%s|%s|%d", userID, payoutMethod, principalStroops)
}
