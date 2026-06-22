package usage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/usage"
)

func write(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "c3x-usage.yml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadEmptyPathReturnsEmptyFile(t *testing.T) {
	t.Parallel()
	f, err := usage.Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.ResourceUsage) != 0 {
		t.Errorf("expected empty file, got %+v", f)
	}
}

func TestLoadRejectsUnknownVersion(t *testing.T) {
	t.Parallel()
	path := write(t, t.TempDir(), `version: "99"`)
	_, err := usage.Load(path)
	if err == nil {
		t.Fatal("expected error for unknown version")
	}
}

func TestApplyMergesPerResourceAttributes(t *testing.T) {
	t.Parallel()
	path := write(t, t.TempDir(), `
version: "0.1"
resource_usage:
  aws_lambda_function.api:
    monthly_requests: 1500000
    average_duration_ms: 250
  aws_s3_bucket.data:
    standard_storage_gb: 500
`)
	f, err := usage.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	resources := []domain.Resource{
		{Ref: domain.Reference{Kind: "aws_lambda_function", Name: "api"}},
		{Ref: domain.Reference{Kind: "aws_s3_bucket", Name: "data"}},
		{Ref: domain.Reference{Kind: "aws_db_instance", Name: "other"}},
	}
	unmatched := usage.Apply(resources, f)
	if len(unmatched) != 0 {
		t.Errorf("expected no unmatched, got %v", unmatched)
	}
	if resources[0].Attributes["monthly_requests"] != 1500000 {
		t.Errorf("lambda monthly_requests = %v", resources[0].Attributes["monthly_requests"])
	}
	if resources[1].Attributes["standard_storage_gb"] != 500 {
		t.Errorf("s3 storage = %v", resources[1].Attributes["standard_storage_gb"])
	}
	if len(resources[2].Attributes) != 0 {
		t.Errorf("unrelated resource was modified: %v", resources[2].Attributes)
	}
}

func TestApplyReportsUnmatchedEntries(t *testing.T) {
	t.Parallel()
	path := write(t, t.TempDir(), `
version: "0.1"
resource_usage:
  aws_lambda_function.api:
    monthly_requests: 1000
  aws_typo_resource.x:
    foo: bar
`)
	f, _ := usage.Load(path)
	resources := []domain.Resource{
		{Ref: domain.Reference{Kind: "aws_lambda_function", Name: "api"}},
	}
	unmatched := usage.Apply(resources, f)
	if len(unmatched) != 1 || unmatched[0] != "aws_typo_resource.x" {
		t.Errorf("expected typo to be unmatched, got %v", unmatched)
	}
}

func TestApplyDefaultsPerKind(t *testing.T) {
	t.Parallel()
	path := write(t, t.TempDir(), `
version: "0.1"
defaults:
  aws_nat_gateway:
    monthly_data_processed_gb: 100
resource_usage:
  aws_nat_gateway.heavy:
    monthly_data_processed_gb: 500
`)
	f, _ := usage.Load(path)
	resources := []domain.Resource{
		{Ref: domain.Reference{Kind: "aws_nat_gateway", Name: "default"}},
		{Ref: domain.Reference{Kind: "aws_nat_gateway", Name: "heavy"}},
	}
	usage.Apply(resources, f)
	if resources[0].Attributes["monthly_data_processed_gb"] != 100 {
		t.Errorf("default not applied: %v", resources[0].Attributes)
	}
	if resources[1].Attributes["monthly_data_processed_gb"] != 500 {
		t.Errorf("explicit value should override default: %v", resources[1].Attributes)
	}
}

func TestApplyOverridesParsedAttributes(t *testing.T) {
	t.Parallel()
	path := write(t, t.TempDir(), `
version: "0.1"
resource_usage:
  aws_lambda_function.api:
    monthly_requests: 999
`)
	f, _ := usage.Load(path)
	resources := []domain.Resource{{
		Ref:        domain.Reference{Kind: "aws_lambda_function", Name: "api"},
		Attributes: map[string]any{"monthly_requests": 1},
	}}
	usage.Apply(resources, f)
	if resources[0].Attributes["monthly_requests"] != 999 {
		t.Errorf("expected usage to override parsed, got %v",
			resources[0].Attributes["monthly_requests"])
	}
}
