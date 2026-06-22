package pricing_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c3xdev/c3x/internal/pricing"
	"github.com/shopspring/decimal"
)

func openCache(t *testing.T, inner pricing.Source, ttl time.Duration, now func() time.Time) *pricing.DiskCache {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c3x.db")
	opts := []pricing.DiskCacheOption{}
	if ttl > 0 {
		opts = append(opts, pricing.WithTTL(ttl))
	}
	if now != nil {
		opts = append(opts, pricing.WithClock(now))
	}
	c, err := pricing.OpenDiskCache(path, inner, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestDiskCacheStoresAndServesHits(t *testing.T) {
	t.Parallel()

	inner := &counter{price: decimal.RequireFromString("0.192")}
	c := openCache(t, inner, time.Hour, nil)

	q := pricing.Query{Provider: "aws", Service: "AmazonEC2", Region: "us-east-1"}
	for i := 0; i < 3; i++ {
		v, _, err := c.Lookup(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		if !v.Equal(decimal.RequireFromString("0.192")) {
			t.Errorf("got %s", v)
		}
	}
	if got := atomic.LoadInt64(&inner.hits); got != 1 {
		t.Errorf("inner Source called %d times across 3 lookups, want 1", got)
	}
}

func TestDiskCacheStaleEntriesRefresh(t *testing.T) {
	t.Parallel()

	// Time travel: the clock advances 24 hours between the first and
	// second lookups, with TTL = 1 hour, so the cache treats the
	// second call as a miss.
	t0 := time.Unix(1_000_000, 0)
	clock := t0
	inner := &counter{price: decimal.RequireFromString("0.192")}
	c := openCache(t, inner, time.Hour, func() time.Time { return clock })

	q := pricing.Query{Provider: "aws", Service: "AmazonEC2"}
	if _, _, err := c.Lookup(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	clock = t0.Add(24 * time.Hour)
	if _, _, err := c.Lookup(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(&inner.hits); got != 2 {
		t.Errorf("inner Source called %d times, want 2 (stale refresh)", got)
	}
}

func TestDiskCacheStats(t *testing.T) {
	t.Parallel()

	t0 := time.Unix(1_000_000, 0)
	clock := t0
	inner := &counter{price: decimal.RequireFromString("1")}
	c := openCache(t, inner, time.Hour, func() time.Time { return clock })

	// Two entries: one fresh, one we'll age out.
	_, _, _ = c.Lookup(context.Background(), pricing.Query{Service: "A"})
	_, _, _ = c.Lookup(context.Background(), pricing.Query{Service: "B"})
	clock = t0.Add(2 * time.Hour)

	s, err := c.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 2 || s.Live != 0 || s.Stale != 2 {
		t.Errorf("stats wrong: %+v", s)
	}
}

func TestDiskCacheClear(t *testing.T) {
	t.Parallel()

	inner := &counter{price: decimal.RequireFromString("1")}
	c := openCache(t, inner, time.Hour, nil)

	_, _, _ = c.Lookup(context.Background(), pricing.Query{Service: "A"})
	_, _, _ = c.Lookup(context.Background(), pricing.Query{Service: "B"})

	n, err := c.Clear()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("Clear returned %d, want 2", n)
	}
	s, _ := c.Stats()
	if s.Total != 0 {
		t.Errorf("Total after clear = %d, want 0", s.Total)
	}
}

func TestBuildChainOfflineReturnsStub(t *testing.T) {
	t.Parallel()
	s, err := pricing.BuildChain(pricing.ChainOptions{Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*pricing.Stub); !ok {
		t.Errorf("expected *Stub, got %T", s)
	}
}

func TestBuildChainNoCacheBypassesDisk(t *testing.T) {
	t.Parallel()
	s, err := pricing.BuildChain(pricing.ChainOptions{NoCache: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*pricing.MemoCache); !ok {
		t.Errorf("expected MemoCache wrapping HTTPSource, got %T", s)
	}
}
