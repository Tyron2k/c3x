package policy_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/policy"
	"github.com/shopspring/decimal"
)

func sampleEstimate() domain.Estimate {
	return domain.Estimate{
		Currency:     domain.CurrencyUSD,
		ProjectTotal: decimal.RequireFromString("1500.00"),
		GeneratedAt:  time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
		Costs: []domain.Cost{
			{
				Resource:        domain.Reference{Kind: "aws_instance", Name: "dev-api"},
				MonthlySubtotal: decimal.RequireFromString("250.00"),
			},
			{
				Resource:        domain.Reference{Kind: "aws_instance", Name: "prod-api"},
				MonthlySubtotal: decimal.RequireFromString("1250.00"),
			},
		},
	}
}

func TestEvalDenyOnOverBudget(t *testing.T) {
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "budget.rego")
	_ = os.WriteFile(policyFile, []byte(`
package c3x

deny[msg] {
  input.estimate.project_total > 1000
  msg := sprintf("project total $%v exceeds $1000 budget",
                 [input.estimate.project_total])
}
`), 0o644)

	res, err := policy.Eval(context.Background(), policyFile,
		policy.BuildInput(sampleEstimate(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasDenials() {
		t.Errorf("expected denials, got none\n%+v", res)
	}
	if !strings.Contains(res.Denials[0], "exceeds") {
		t.Errorf("denial message wrong: %v", res.Denials)
	}
}

func TestEvalWarningPassesWithoutFailing(t *testing.T) {
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "warn.rego")
	_ = os.WriteFile(policyFile, []byte(`
package c3x

warn[msg] {
  resource := input.estimate.resources[_]
  contains(resource.name, "dev")
  resource.monthly_cost > 100
  msg := sprintf("non-prod %v costs $%v", [resource.name, resource.monthly_cost])
}
`), 0o644)

	res, err := policy.Eval(context.Background(), policyFile,
		policy.BuildInput(sampleEstimate(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if res.HasDenials() {
		t.Errorf("warn rule should not deny: %+v", res)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(res.Warnings))
	}
	if !strings.Contains(res.Warnings[0], "dev-api") {
		t.Errorf("warning didn't reference the dev resource: %v", res.Warnings)
	}
}

func TestEvalLoadsDirectoryOfPolicies(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.rego"), []byte(`
package c3x

deny[msg] {
  input.estimate.project_total > 100
  msg := "a-policy fired"
}
`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.rego"), []byte(`
package c3x

warn[msg] {
  input.estimate.project_total > 100
  msg := "b-policy fired"
}
`), 0o644)

	res, err := policy.Eval(context.Background(), dir,
		policy.BuildInput(sampleEstimate(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasDenials() {
		t.Error("expected the a.rego deny to fire")
	}
	if len(res.Warnings) != 1 {
		t.Errorf("expected 1 warn from b.rego, got %v", res.Warnings)
	}
}

func TestEvalDiffInput(t *testing.T) {
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "diff.rego")
	_ = os.WriteFile(policyFile, []byte(`
package c3x

deny[msg] {
  input.diff.total_delta > 50
  msg := "PR adds too much"
}
`), 0o644)

	baseline := sampleEstimate()
	baseline.ProjectTotal = decimal.RequireFromString("1000.00")
	current := sampleEstimate() // 1500 - 1000 delta = 500
	diff := domain.ComputeDiff(baseline, current)

	res, err := policy.Eval(context.Background(), policyFile,
		policy.BuildInput(current, &diff))
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasDenials() {
		t.Error("expected deny on diff delta > 50")
	}
}

func TestEvalNoPoliciesFound(t *testing.T) {
	dir := t.TempDir() // empty
	_, err := policy.Eval(context.Background(), dir,
		policy.BuildInput(sampleEstimate(), nil))
	if err == nil {
		t.Error("expected error when policy dir has no .rego files")
	}
}
