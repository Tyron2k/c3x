package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/c3xdev/c3x/resources"
	"github.com/pelletier/go-toml/v2"
)

// Load reads every `*.toml` file from the bundled catalog FS, parses
// each into a Definition, and returns the populated Registry.
//
// LoadFromFS does the same for an arbitrary fs.FS, which lets callers
// override the embedded catalog (e.g. `c3x estimate --resources ./my-catalog`).
//
// Validation rules enforced here:
//   - Kind must be set and unique across the whole tree.
//   - Provider must be one of aws|azure|gcp.
//   - Each AttributeFilter has either Literal or Expr, not both.
//   - Each DimensionSpec has Quantity and Rate set.
//
// Errors include the offending file path so authors get actionable
// diagnostics rather than a generic TOML parse error.
func Load() (*Registry, error) {
	return LoadFromFS(resources.FS, ".")
}

// LoadFromFS reads the catalog from the given filesystem, treating
// `root` as the base directory.
func LoadFromFS(fsys fs.FS, root string) (*Registry, error) {
	reg := newRegistry()
	walker := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking %s: %w", path, err)
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".toml") {
			return nil
		}
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		// free.toml is the bulk free-kinds list: one line per
		// no-charge resource kind instead of a full definition file
		// each. Synthesised into zero-cost Definitions here so the
		// rest of the engine never knows the difference.
		if filepath.Base(path) == "free.toml" {
			return registerFreeKinds(reg, raw, path)
		}
		var def Definition
		if err := toml.Unmarshal(raw, &def); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if err := validate(&def, path); err != nil {
			return err
		}
		if err := reg.add(&def); err != nil {
			return fmt.Errorf("register %s: %w", path, err)
		}
		return nil
	}
	if err := fs.WalkDir(fsys, root, walker); err != nil {
		return nil, err
	}
	if reg.Len() == 0 {
		return nil, errors.New("catalog is empty (no TOMLs found)")
	}
	return reg, nil
}

func validate(def *Definition, path string) error {
	if def.Kind == "" {
		return fmt.Errorf("%s: missing required field `kind`", path)
	}
	// The provider field must match the directory the TOML lives in
	// (resources/<provider>/<kind>.toml; the embedded FS strips the
	// "resources/" prefix, so compare path components, not substrings).
	if def.Provider != "" {
		matched := false
		for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(path)), "/") {
			if part == def.Provider {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s: provider %q does not match its directory", path, def.Provider)
		}
	}
	switch def.Provider {
	case "aws", "azure", "gcp":
	default:
		return fmt.Errorf("%s: unsupported provider %q (want aws|azure|gcp)", path, def.Provider)
	}
	for name, m := range def.Mappings {
		for i, af := range m.AttributeFilters {
			if af.Key == "" {
				return fmt.Errorf("%s: mapping %q filter[%d]: empty key", path, name, i)
			}
			if af.Literal != "" && af.Expr != "" {
				return fmt.Errorf("%s: mapping %q filter[%d] %q: both `const` and `expr` set",
					path, name, i, af.Key)
			}
		}
	}
	seenDimIDs := make(map[string]struct{}, len(def.Dimensions))
	for i, dim := range def.Dimensions {
		if dim.ID == "" {
			return fmt.Errorf("%s: dimension[%d]: missing `id`", path, i)
		}
		if _, dup := seenDimIDs[dim.ID]; dup {
			// Two dimensions with the same ID would collide on the
			// engine's program cache (keyed `qty:<kind>.<dim_id>`),
			// silently overwriting each other's compiled expressions.
			// Fail loud at load time.
			return fmt.Errorf("%s: duplicate dimension id %q (each dimension must have a unique id)",
				path, dim.ID)
		}
		seenDimIDs[dim.ID] = struct{}{}
		if dim.Quantity == "" {
			return fmt.Errorf("%s: dimension %q: missing `quantity`", path, dim.ID)
		}
		if dim.Rate == "" {
			return fmt.Errorf("%s: dimension %q: missing `rate`", path, dim.ID)
		}
	}
	// Validate that every price("name") in dimensions references a
	// mapping that actually exists. A typo in the dimension's rate
	// otherwise silently resolves to a $0 rate at runtime (the
	// lookup returns "no priced products matched").
	for _, dim := range def.Dimensions {
		for _, mappingName := range priceLookupsIn(dim.Rate) {
			if _, ok := def.Mappings[mappingName]; !ok {
				return fmt.Errorf("%s: dimension %q references price(%q) but mapping is not declared",
					path, dim.ID, mappingName)
			}
		}
	}
	return nil
}

// priceLookupsIn extracts every `"name"` argument passed to
// price(...) in an expression source. Lightweight parser; we don't
// need full expression parsing because we only look for the literal
// pattern.
func priceLookupsIn(src string) []string {
	var out []string
	for i := 0; i < len(src); i++ {
		// Look for `price("...")`.
		if i+7 > len(src) || src[i:i+7] != "price(\"" {
			continue
		}
		j := i + 7
		for j < len(src) && src[j] != '"' {
			j++
		}
		if j >= len(src) {
			break
		}
		out = append(out, src[i+7:j])
		i = j
	}
	return out
}

// freeKindsFile is the schema of a provider's free.toml: a flat
// kind → reason map. The reason surfaces in `supported-resources`
// and documentation; insisting on one keeps the list reviewable —
// every entry must say WHY the resource is free.
type freeKindsFile struct {
	Provider string            `toml:"provider"`
	Kinds    map[string]string `toml:"kinds"`
}

func registerFreeKinds(reg *Registry, raw []byte, path string) error {
	var f freeKindsFile
	if err := toml.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	switch f.Provider {
	case "aws", "azure", "gcp":
	default:
		return fmt.Errorf("%s: provider %q must be aws|azure|gcp", path, f.Provider)
	}
	for kind, reason := range f.Kinds {
		if reason == "" {
			return fmt.Errorf("%s: kind %q has no reason — every free entry must say why", path, kind)
		}
		def := &Definition{
			Kind:        kind,
			DisplayName: kind,
			Provider:    f.Provider,
			Dimensions: []DimensionSpec{{
				ID: "free", Label: "Free resource", Unit: "n/a",
				Quantity: "0", Rate: "0",
			}},
			Fixture: &Fixture{ExpectedMonthlyCost: 0, Exact: true},
		}
		if err := validate(def, path); err != nil {
			return err
		}
		if err := reg.add(def); err != nil {
			return fmt.Errorf("register %s (%s): %w", kind, path, err)
		}
	}
	return nil
}
