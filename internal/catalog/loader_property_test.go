package catalog_test

// Loader-validation property tests. These guard against classes of
// bugs that would silently corrupt estimates at runtime:
//
//   - Duplicate dimension IDs within a single kind would collide on
//     the engine's program cache (`qty:<kind>.<dim_id>`), with the
//     second compile() returning the first dimension's compiled
//     expression — a hidden mis-estimate.
//
//   - `price("name")` referencing an undeclared mapping silently
//     resolves to a $0 rate (the upstream returns "no matched
//     products"); the resource's monthly subtotal looks valid but
//     a real line item is missing.
//
//   - Missing required fields (kind, dimension id/quantity/rate)
//     would cause runtime panics or silent skips.
//
// Each test constructs a minimal in-memory catalog filesystem with
// the bad TOML and asserts Load fails loudly.

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/c3xdev/c3x/internal/catalog"
)

func TestLoaderRejectsDuplicateDimensionID(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"aws/aws_test.toml": &fstest.MapFile{
			Data: []byte(`
kind         = "aws_test"
display_name = "Test"
provider     = "aws"

[[dimensions]]
id       = "duped"
quantity = "1"
rate     = "0.10"

[[dimensions]]
id       = "duped"
quantity = "2"
rate     = "0.20"
`),
		},
	}
	_, err := catalog.LoadFromFS(fsys, ".")
	if err == nil {
		t.Fatal("expected duplicate-dimension-id error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate dimension id") {
		t.Errorf("expected 'duplicate dimension id' in error, got: %v", err)
	}
}

func TestLoaderRejectsUnknownPriceLookup(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"aws/aws_test.toml": &fstest.MapFile{
			Data: []byte(`
kind         = "aws_test"
display_name = "Test"
provider     = "aws"

[mappings.real_mapping]
service           = "AmazonEC2"
product_family    = "Compute Instance"
attribute_filters = []

[[dimensions]]
id       = "ghost"
quantity = "1"
rate     = 'price("nonexistent_mapping")'
`),
		},
	}
	_, err := catalog.LoadFromFS(fsys, ".")
	if err == nil {
		t.Fatal("expected unknown-mapping error, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent_mapping") {
		t.Errorf("expected 'nonexistent_mapping' in error, got: %v", err)
	}
}

func TestLoaderAcceptsValidPriceLookup(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"aws/aws_test.toml": &fstest.MapFile{
			Data: []byte(`
kind         = "aws_test"
display_name = "Test"
provider     = "aws"

[mappings.compute]
service           = "AmazonEC2"
product_family    = "Compute Instance"
attribute_filters = []

[[dimensions]]
id       = "hours"
quantity = "1"
rate     = 'price("compute")'
`),
		},
	}
	if _, err := catalog.LoadFromFS(fsys, "."); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLoaderRejectsMissingFields(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"missing-kind": `display_name = "X"
provider = "aws"`,
		"missing-provider": `kind = "aws_test"
display_name = "X"`,
		"invalid-provider": `kind = "aws_test"
display_name = "X"
provider = "foobar"`,
		"missing-dim-id": `kind = "aws_test"
display_name = "X"
provider = "aws"

[[dimensions]]
quantity = "1"
rate = "0"`,
		"missing-quantity": `kind = "aws_test"
display_name = "X"
provider = "aws"

[[dimensions]]
id = "a"
rate = "0"`,
		"missing-rate": `kind = "aws_test"
display_name = "X"
provider = "aws"

[[dimensions]]
id = "a"
quantity = "1"`,
	}
	for name, body := range cases {
		body := body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fsys := fstest.MapFS{"aws/aws_test.toml": &fstest.MapFile{Data: []byte(body)}}
			if _, err := catalog.LoadFromFS(fsys, "."); err == nil {
				t.Errorf("expected error for %s, got nil", name)
			}
		})
	}
}

// TestEveryEmbeddedDefinitionHasUniqueDimensions sweeps the live
// catalog. Belt-and-braces on top of the loader check: if a TOML
// snuck in with a duplicate-id, this fails the test even before
// LoadFromFS rejects it (covers both directions).
func TestEveryEmbeddedDefinitionHasUniqueDimensions(t *testing.T) {
	reg, err := catalog.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, kind := range reg.Kinds() {
		def := reg.Get(kind)
		seen := make(map[string]struct{}, len(def.Dimensions))
		for _, dim := range def.Dimensions {
			if _, dup := seen[dim.ID]; dup {
				t.Errorf("%s: duplicate dimension id %q", kind, dim.ID)
			}
			seen[dim.ID] = struct{}{}
		}
	}
}
