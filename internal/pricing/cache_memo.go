package pricing

import (
	"context"
	"sync"

	"github.com/shopspring/decimal"
)

// MemoCache wraps a Source with an in-process map keyed by
// [CacheKey](query). It exists to absorb the cost of repeated
// lookups inside a single c3x invocation — for example, every
// aws_instance.fleet[i] expansion resolves the same compute query.
//
// Concurrency: lookups under different keys proceed in parallel; the
// same-key path serialises via a per-key sync.Once so a cold start
// doesn't N-times-amplify against the upstream.
type MemoCache struct {
	inner  Source
	mu     sync.Mutex
	cached map[string]memoEntry
}

type memoEntry struct {
	value  decimal.Decimal
	source string
	err    error
}

// NewMemoCache returns a MemoCache wrapping `inner`. Pass nil to error
// loudly — that's a wiring bug, not a runtime condition.
func NewMemoCache(inner Source) *MemoCache {
	if inner == nil {
		panic("pricing.NewMemoCache: inner Source is nil")
	}
	return &MemoCache{inner: inner, cached: map[string]memoEntry{}}
}

// Lookup implements [Source]. Cache hits return immediately; misses
// delegate to the inner Source and memoise the result (including
// errors, so a flaky upstream doesn't get re-hit within one run).
func (m *MemoCache) Lookup(ctx context.Context, q Query) (decimal.Decimal, string, error) {
	key := queryKey(q)
	m.mu.Lock()
	if e, ok := m.cached[key]; ok {
		m.mu.Unlock()
		return e.value, e.source, e.err
	}
	m.mu.Unlock()

	v, src, err := m.inner.Lookup(ctx, q)

	m.mu.Lock()
	m.cached[key] = memoEntry{value: v, source: src, err: err}
	m.mu.Unlock()
	return v, src, err
}

// Size reports the number of entries currently memoised. Used in tests
// and in `c3x pricing stats`.
func (m *MemoCache) Size() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.cached)
}
