package terraform_test

import (
	"path/filepath"
	"testing"

	"github.com/c3xdev/c3x/internal/parser/terraform"
)

// TestRealWorldCorpus parses every fixture under testdata/corpus/ and
// asserts they all produce a non-empty resource list. The corpus is
// hand-crafted to mirror patterns common in real Terraform monorepos:
// VPCs, EKS clusters, multi-cloud configs, and modules. It's our
// answer to "did you actually try this on something realistic?"
//
// Each entry's expectations live next to the .tf file as an inline
// minimum-resource-count so the test self-documents.
func TestRealWorldCorpus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dir      string
		minCount int
	}{
		{"vpc-stack", 9},             // 3 EIPs + 3 NAT + 1 ALB + 1 RDS + 3 EC2 (for_each over 3 keys)
		{"eks-cluster", 8},           // 1 cluster + 4 workers + 1 ALB + 2 S3 + 1 log group
		{"multi-cloud", 4},           // CloudFront + S3 + Postgres + GCS
		{"monorepo-with-modules", 2}, // DB + Cache via module expansion
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.dir, func(t *testing.T) {
			t.Parallel()
			path, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "corpus", tc.dir))
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := terraform.ParseDirectory(path, terraform.Options{})
			if err != nil {
				t.Fatalf("ParseDirectory %s: %v", tc.dir, err)
			}
			if len(parsed) < tc.minCount {
				t.Errorf("%s: expected at least %d resources, got %d", tc.dir, tc.minCount, len(parsed))
				for _, r := range parsed {
					t.Logf("  - %s", r.Ref.Label())
				}
			}
		})
	}
}
