package main

// End-to-end golden tests for `c3x estimate`. Each test runs the
// CLI against a fixture under testdata/corpus/ in --offline mode
// (deterministic — no live HTTP) and compares the markdown output
// to a checked-in expected file under testdata/golden/.
//
// Golden files capture the SHAPE of output: resources detected,
// dimensions surfaced, rendering format. They protect against
// pipeline-level regressions (a parser silently dropping a resource,
// a calculator skipping a dimension, a renderer changing column
// order). The numeric costs are mostly $0 in offline mode because
// the stub doesn't seed live rates; STATIC rates (EIP, EKS, …) do
// appear, which catches inline-rate regressions too.
//
// Updating the goldens: run with -update to regenerate, then commit
// the diff:
//   go test ./cmd/c3x/... -run TestEstimateGolden -update

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGoldens = flag.Bool("update", false, "regenerate golden files in testdata/golden/")

func TestEstimateGolden(t *testing.T) {
	// Each subtest covers one fixture directory.
	cases := []string{
		"eks-cluster",
		"monorepo-with-modules",
		"multi-cloud",
		"vpc-stack",
	}
	for _, name := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())

			repoRoot, err := projectRoot()
			if err != nil {
				t.Fatalf("projectRoot: %v", err)
			}
			corpus := filepath.Join(repoRoot, "testdata", "corpus", name)
			golden := filepath.Join(repoRoot, "testdata", "golden", name+".md")

			cmd := newRootCmd()
			out := &bytes.Buffer{}
			cmd.SetOut(out)
			cmd.SetErr(out)
			cmd.SetArgs([]string{
				"estimate",
				"--offline",
				"--path", corpus,
				"--format", "markdown",
			})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("estimate failed: %v\nOutput:\n%s", err, out.String())
			}
			got := out.String()

			if *updateGoldens {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("updated %s", golden)
				return
			}

			wantBytes, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with -update to create)", golden, err)
			}
			want := string(wantBytes)
			if got != want {
				t.Errorf("golden mismatch for %s.\n=== got ===\n%s\n=== want ===\n%s\n=== diff (first 60 lines) ===\n%s",
					name, got, want, firstDiff(got, want))
			}
		})
	}
}

// projectRoot walks up from the cwd until it finds go.mod. Tests
// run from cmd/c3x but reference testdata/ at the repo root.
func projectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// firstDiff returns the first ~30 lines around the divergence point
// so the test failure message points to where the goldens drifted.
func firstDiff(got, want string) string {
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(want, "\n")
	n := len(gotLines)
	if len(wantLines) < n {
		n = len(wantLines)
	}
	for i := 0; i < n; i++ {
		if gotLines[i] != wantLines[i] {
			lo := i - 2
			if lo < 0 {
				lo = 0
			}
			hi := i + 10
			if hi > n {
				hi = n
			}
			var b strings.Builder
			for j := lo; j < hi; j++ {
				prefix := "  "
				if j == i {
					prefix = "* "
				}
				b.WriteString(prefix)
				b.WriteString(gotLines[j])
				b.WriteString("\n")
				if j == i {
					b.WriteString("  expected: ")
					b.WriteString(wantLines[j])
					b.WriteString("\n")
				}
			}
			return b.String()
		}
	}
	return "(no line-level diff found)"
}
