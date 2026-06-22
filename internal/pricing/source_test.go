package pricing_test

import (
	"context"
	"testing"

	"github.com/c3xdev/c3x/internal/pricing"
	"github.com/shopspring/decimal"
)

func TestStubReturnsSetPrice(t *testing.T) {
	t.Parallel()

	s := pricing.NewStub()
	q := pricing.Query{
		Provider: "aws", Service: "AmazonEC2", Region: "us-east-1",
		AttributeFilters: []pricing.KV{{Key: "instanceType", Value: "m5.xlarge"}},
	}
	s.Set(q, decimal.RequireFromString("0.192"))

	got, src, err := s.Lookup(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(decimal.RequireFromString("0.192")) {
		t.Errorf("got %s, want 0.192", got)
	}
	if src != "stub" {
		t.Errorf("source = %q, want stub", src)
	}
}

func TestStubReturnsZeroOnMiss(t *testing.T) {
	t.Parallel()

	s := pricing.NewStub()
	got, _, err := s.Lookup(context.Background(), pricing.Query{Service: "Nope"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero, got %s", got)
	}
}

// TestStubKeyNormalisesFilterOrder pins the cross-Source invariant:
// two queries differing only by attribute-filter order share one key.
// MemoCache and DiskCache rely on this so a calculator that builds
// filters in a different order on a second invocation still hits the
// cache.
func TestStubKeyNormalisesFilterOrder(t *testing.T) {
	t.Parallel()

	s := pricing.NewStub()
	q1 := pricing.Query{AttributeFilters: []pricing.KV{
		{Key: "a", Value: "1"}, {Key: "b", Value: "2"},
	}}
	q2 := pricing.Query{AttributeFilters: []pricing.KV{
		{Key: "b", Value: "2"}, {Key: "a", Value: "1"},
	}}
	s.Set(q1, decimal.NewFromInt(99))
	got, _, _ := s.Lookup(context.Background(), q2)
	if !got.Equal(decimal.NewFromInt(99)) {
		t.Errorf("expected reordered filters to hit the same key; got %s", got)
	}
}
