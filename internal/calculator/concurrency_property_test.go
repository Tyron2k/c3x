package calculator_test

// Concurrency property tests for the calculator engine. The program
// cache, expression evaluator, and pricing source are all touched
// from many goroutines once the caller fans out a multi-resource
// Estimate; these tests assert that no cross-resource state bleeds
// even under aggressive parallelism.
//
// The original bug that motivated these (engine.go:288 filter cache
// keyed only on `kind.attr_key`) was found by reading the code;
// these tests would have caught the same shape — different
// resources of the same kind getting tangled-up filter values — and
// they catch the next bug like it.

import (
	"context"
	"sync"
	"testing"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/pricing"
	"github.com/shopspring/decimal"
)

// TestParallelEstimatesOfSameKindDoNotTangle runs N=200 estimates of
// the same kind (aws_ebs_volume) with different `size` attribute
// values, in parallel. Each resource has its own expected cost; if
// any cache keys leak across resources, costs would shuffle and at
// least one resource would receive another's expected total.
func TestParallelEstimatesOfSameKindDoNotTangle(t *testing.T) {
	t.Parallel()

	stub := pricing.NewStub()
	stub.Set(pricing.Query{
		Provider:       "aws",
		Service:        "AmazonEC2",
		ProductFamily:  "Storage",
		Region:         "us-east-1",
		PurchaseOption: "on_demand",
		AttributeFilters: []pricing.KV{
			{Key: "volumeApiName", Value: "gp3"},
		},
	}, decimal.RequireFromString("0.08"))

	engine := newEngine(t, stub)
	region := "us-east-1"

	const N = 200
	type result struct {
		size  float64
		total decimal.Decimal
	}
	results := make(chan result, N)

	var wg sync.WaitGroup
	for i := 1; i <= N; i++ {
		wg.Add(1)
		go func(size float64) {
			defer wg.Done()
			res := domain.Resource{
				Ref:    domain.Reference{Kind: "aws_ebs_volume", Name: "vol"},
				Region: &region,
				Attributes: map[string]any{
					"type": "gp3",
					"size": size,
				},
			}
			est, err := engine.Estimate(context.Background(), []domain.Resource{res})
			if err != nil {
				t.Errorf("Estimate(size=%v): %v", size, err)
				return
			}
			if len(est.Costs) != 1 {
				t.Errorf("Estimate(size=%v): want 1 cost, got %d", size, len(est.Costs))
				return
			}
			results <- result{size: size, total: est.Costs[0].MonthlySubtotal}
		}(float64(i))
	}
	wg.Wait()
	close(results)

	// Each volume's cost should be size * $0.08 (the gp3 rate). If a
	// cache collision tangled them, at least one resource's total
	// would correspond to a *different* size's expected value.
	for r := range results {
		// Compare in decimal space to avoid float64 round-off
		// (size × 0.08 reflects float64 representational noise).
		expected := decimal.NewFromInt(int64(r.size)).Mul(decimal.RequireFromString("0.08"))
		if !r.total.Equal(expected) {
			t.Errorf("size %v: got %s, want %s — cache collision suspect",
				r.size, r.total, expected)
		}
	}
}

// TestParallelEstimatesAcrossKindsDoNotTangle takes the same idea
// but mixes resource kinds within a single Estimate batch, exercising
// the program cache's per-kind keying across goroutines.
func TestParallelEstimatesAcrossKindsDoNotTangle(t *testing.T) {
	t.Parallel()

	stub := pricing.NewStub()
	// gp3 EBS rate
	stub.Set(pricing.Query{
		Provider:       "aws",
		Service:        "AmazonEC2",
		ProductFamily:  "Storage",
		Region:         "us-east-1",
		PurchaseOption: "on_demand",
		AttributeFilters: []pricing.KV{
			{Key: "volumeApiName", Value: "gp3"},
		},
	}, decimal.RequireFromString("0.08"))
	// m5.xlarge instance rate
	stub.Set(pricing.Query{
		Provider:       "aws",
		Service:        "AmazonEC2",
		ProductFamily:  "Compute Instance",
		Region:         "us-east-1",
		PurchaseOption: "on_demand",
		AttributeFilters: []pricing.KV{
			{Key: "capacitystatus", Value: "Used"},
			{Key: "instanceType", Value: "m5.xlarge"},
			{Key: "operatingSystem", Value: "Linux"},
			{Key: "preInstalledSw", Value: "NA"},
			{Key: "tenancy", Value: "Shared"},
		},
	}, decimal.RequireFromString("0.192"))

	engine := newEngine(t, stub)
	region := "us-east-1"

	const N = 100
	resources := make([]domain.Resource, 0, N*2)
	for i := 1; i <= N; i++ {
		resources = append(resources,
			domain.Resource{
				Ref:    domain.Reference{Kind: "aws_ebs_volume", Name: "v"},
				Region: &region,
				Attributes: map[string]any{
					"type": "gp3",
					"size": float64(i * 10),
				},
			},
			domain.Resource{
				Ref:    domain.Reference{Kind: "aws_instance", Name: "i"},
				Region: &region,
				Attributes: map[string]any{
					"instance_type": "m5.xlarge",
				},
			},
		)
	}

	est, err := engine.Estimate(context.Background(), resources)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if len(est.Costs) != len(resources) {
		t.Fatalf("want %d cost rows, got %d", len(resources), len(est.Costs))
	}
	// Total: N volumes × i*10 × $0.08 + N instances × 730 × $0.192.
	wantVol := 0.0
	for i := 1; i <= N; i++ {
		wantVol += float64(i*10) * 0.08
	}
	wantInst := float64(N) * 730 * 0.192
	want := decimal.NewFromFloat(wantVol + wantInst)
	if !est.ProjectTotal.Equal(want.Round(2)) && !est.ProjectTotal.Round(2).Equal(want.Round(2)) {
		t.Errorf("ProjectTotal = %s, want %s", est.ProjectTotal, want.Round(2))
	}
}
