package calculator_test

// Tests for the Estimate.Skipped surface — the calculator records
// when a resource was detected by the parser but couldn't be
// priced, so renderers (and the --show-skipped flag) can surface
// the gap rather than silently undercounting the total.

import (
	"context"
	"testing"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/pricing"
)

func TestSkippedRecordsUnsupportedKind(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, pricing.NewStub())
	region := "us-east-1"
	est, err := engine.Estimate(context.Background(), []domain.Resource{{
		Ref:    domain.Reference{Kind: "aws_does_not_exist", Name: "x"},
		Region: &region,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(est.Skipped) != 1 {
		t.Fatalf("expected 1 skipped resource, got %d", len(est.Skipped))
	}
	if est.Skipped[0].Resource.Kind != "aws_does_not_exist" {
		t.Errorf("wrong kind in skipped record: %+v", est.Skipped[0])
	}
	if est.Skipped[0].Reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestSkippedDoesNotRecordFreeShells(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, pricing.NewStub())
	region := "us-east-1"
	// aws_iam_role is a known FREE shell — must NOT appear as
	// skipped even though it produces $0.
	est, err := engine.Estimate(context.Background(), []domain.Resource{{
		Ref:    domain.Reference{Kind: "aws_iam_role", Name: "lambda-exec"},
		Region: &region,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(est.Skipped) != 0 {
		t.Errorf("free shell wrongly classified as skipped: %+v", est.Skipped)
	}
}
