package pricing_test

// Pricing source benchmarks. The memo cache wraps slower sources
// (disk, HTTP) so the lookup latency of repeated identical queries
// should be sub-microsecond — anything more is wasted on the
// estimator's hot path.

import (
	"context"
	"testing"

	"github.com/c3xdev/c3x/internal/pricing"
	"github.com/shopspring/decimal"
)

func BenchmarkStubLookup(b *testing.B) {
	stub := pricing.NewStub()
	q := pricing.Query{
		Provider:       "aws",
		Service:        "AmazonEC2",
		ProductFamily:  "Compute Instance",
		Region:         "us-east-1",
		PurchaseOption: "on_demand",
		AttributeFilters: []pricing.KV{
			{Key: "instanceType", Value: "m5.xlarge"},
		},
	}
	stub.Set(q, decimal.RequireFromString("0.192"))
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := stub.Lookup(ctx, q); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMemoCacheHit measures the in-process memo cache's
// happy-path latency. Real workloads hit this thousands of times
// per estimate; anything above ~500 ns is a tell-tale of
// over-allocation in the key builder.
func BenchmarkMemoCacheHit(b *testing.B) {
	stub := pricing.NewStub()
	q := pricing.Query{
		Provider:       "aws",
		Service:        "AmazonEC2",
		ProductFamily:  "Compute Instance",
		Region:         "us-east-1",
		PurchaseOption: "on_demand",
		AttributeFilters: []pricing.KV{
			{Key: "instanceType", Value: "m5.xlarge"},
		},
	}
	stub.Set(q, decimal.RequireFromString("0.192"))

	mc := pricing.NewMemoCache(stub)
	ctx := context.Background()
	// Warm the cache.
	if _, _, err := mc.Lookup(ctx, q); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := mc.Lookup(ctx, q); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCacheKey exercises just the key-builder; it should be
// allocation-light because it's invoked at every cache check.
func BenchmarkCacheKey(b *testing.B) {
	q := pricing.Query{
		Provider:       "aws",
		Service:        "AmazonEC2",
		ProductFamily:  "Compute Instance",
		Region:         "us-east-1",
		PurchaseOption: "on_demand",
		AttributeFilters: []pricing.KV{
			{Key: "instanceType", Value: "m5.xlarge"},
			{Key: "operatingSystem", Value: "Linux"},
			{Key: "tenancy", Value: "Shared"},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pricing.CacheKey(q)
	}
}
