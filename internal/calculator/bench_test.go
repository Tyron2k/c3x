package calculator_test

// Engine benchmarks. Run with:
//
//	go test ./internal/calculator/... -bench=. -benchmem -run='^$'
//
// Targets:
//   - BenchmarkEstimateSingle: per-resource cost (price-source stub
//     used so we measure compute, not network)
//   - BenchmarkEstimateBatch:  100 resources of mixed kinds; the
//     hot loop that runs on a real project
//   - BenchmarkProgramCacheHit: the expression program cache must
//     not be allocating in the common case (every dimension's qty
//     and rate is compiled once and then re-used)

import (
	"context"
	"testing"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/pricing"
	"github.com/shopspring/decimal"
)

func BenchmarkEstimateSingle(b *testing.B) {
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

	engine := newEngine(b, stub)
	region := "us-east-1"
	res := domain.Resource{
		Ref:    domain.Reference{Kind: "aws_ebs_volume", Name: "bench"},
		Region: &region,
		Attributes: map[string]any{
			"type": "gp3",
			"size": float64(100),
		},
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Estimate(ctx, []domain.Resource{res}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEstimateBatch(b *testing.B) {
	stub := pricing.NewStub()
	// Seed gp3 + m5.xlarge so the batch produces real numbers.
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

	engine := newEngine(b, stub)
	region := "us-east-1"

	resources := make([]domain.Resource, 0, 100)
	for i := 0; i < 50; i++ {
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
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Estimate(ctx, resources); err != nil {
			b.Fatal(err)
		}
	}
}
