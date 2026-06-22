package terraform_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/c3xdev/c3x/internal/parser/terraform"
)

// FuzzParseDirectory throws arbitrary byte sequences at the parser as
// if they were main.tf and asserts that we never panic. Any input
// either parses, returns a wrapped error, or returns no resources —
// what we never accept is a goroutine death that takes the CLI down.
//
// Run locally with `go test -fuzz=FuzzParseDirectory -fuzztime=30s
// ./internal/parser/terraform/`. CI runs the seed corpus only.
func FuzzParseDirectory(f *testing.F) {
	// Seeds: representative shapes that exercised parser branches in
	// integration tests. The fuzzer mutates around these.
	seeds := []string{
		`provider "aws" { region = "us-east-1" }`,
		`variable "x" { default = "v" }`,
		`resource "aws_instance" "x" { instance_type = var.t }`,
		`locals { x = format("%s-%d", "a", 1) }`,
		`module "m" { source = "./m" }`,
		``,
		`resource "" "" {}`,
		`# only a comment`,
		`provider "aws" { region = var.region }`,
		`resource "aws_instance" "x" { count = length([1,2,3]) }`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), raw, 0o644); err != nil {
			t.Skip(err)
		}
		// We don't care about the return values — we only care that the
		// call doesn't panic. A wrapped error or empty result is fine.
		// Offline: the fuzzer can synthesise registry/git module
		// sources; resolution must never leave the process.
		_, _ = terraform.ParseDirectory(dir, terraform.Options{Offline: true})
	})
}
