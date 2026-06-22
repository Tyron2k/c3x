package pricing_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/c3xdev/c3x/internal/pricing"
	"github.com/shopspring/decimal"
)

// counter is a Source that returns the same fixed price but counts
// invocations. Used to prove the memo layer absorbs duplicate hits.
type counter struct {
	hits  int64
	price decimal.Decimal
}

func (c *counter) Lookup(_ context.Context, _ pricing.Query) (decimal.Decimal, string, error) {
	atomic.AddInt64(&c.hits, 1)
	return c.price, "live", nil
}

func TestMemoCacheServesRepeatedLookupsFromCache(t *testing.T) {
	t.Parallel()

	inner := &counter{price: decimal.RequireFromString("0.192")}
	cache := pricing.NewMemoCache(inner)

	q := pricing.Query{Provider: "aws", Service: "AmazonEC2", Region: "us-east-1"}
	for i := 0; i < 5; i++ {
		v, _, err := cache.Lookup(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		if !v.Equal(decimal.RequireFromString("0.192")) {
			t.Errorf("got %s", v)
		}
	}
	if got := atomic.LoadInt64(&inner.hits); got != 1 {
		t.Errorf("inner Source called %d times, want 1", got)
	}
	if cache.Size() != 1 {
		t.Errorf("Size = %d, want 1", cache.Size())
	}
}

func TestMemoCacheKeysIgnoreFilterOrder(t *testing.T) {
	t.Parallel()

	inner := &counter{price: decimal.RequireFromString("1.00")}
	cache := pricing.NewMemoCache(inner)

	q1 := pricing.Query{AttributeFilters: []pricing.KV{
		{Key: "a", Value: "1"}, {Key: "b", Value: "2"},
	}}
	q2 := pricing.Query{AttributeFilters: []pricing.KV{
		{Key: "b", Value: "2"}, {Key: "a", Value: "1"},
	}}
	if _, _, err := cache.Lookup(context.Background(), q1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cache.Lookup(context.Background(), q2); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(&inner.hits); got != 1 {
		t.Errorf("reordered filters caused %d backend hits, want 1", got)
	}
}
