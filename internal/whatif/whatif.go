// Package whatif applies `kind.name.attr=value` CLI overrides to
// parsed resources. The overrides land between parser and calculator
// — after the IaC source is resolved into [domain.Resource]s but
// before the calculator evaluates dimensions — so users can ask
// "what if this aws_instance was m6i.large?" without editing the .tf.
//
// Overrides participate in the same precedence chain as everything
// else: parser defaults < usage file < --what-if. The whatif layer
// is applied last so it always wins.
package whatif

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/c3xdev/c3x/internal/domain"
)

// Override is one parsed override directive. It binds an attribute on
// a specific resource (`Kind.Name.Attr`) to a typed value.
type Override struct {
	Kind  string
	Name  string
	Attr  string
	Value any
}

// Parse turns a slice of `--what-if kind.name.attr=value` strings
// into [Override]s. Values are type-coerced: bool first, then int,
// then float, then string. The strict type-precedence order keeps
// `true` from accidentally becoming the string "true".
func Parse(raws []string) ([]Override, error) {
	out := make([]Override, 0, len(raws))
	for _, raw := range raws {
		o, err := parseOne(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func parseOne(raw string) (Override, error) {
	eq := strings.Index(raw, "=")
	if eq < 0 {
		return Override{}, fmt.Errorf("--what-if expects kind.name.attr=value, got %q", raw)
	}
	lhs, rhs := raw[:eq], raw[eq+1:]
	// Split lhs on the LAST dot so attr never accidentally consumes
	// dots that belong to `module.X.module.Y.kind.name`. Walk right to
	// left and require at least three segments: kind, name, attr.
	parts := strings.Split(lhs, ".")
	if len(parts) < 3 {
		return Override{}, fmt.Errorf("--what-if lhs %q: need kind.name.attr (>=3 segments)", lhs)
	}
	attr := parts[len(parts)-1]
	// Name is everything between the kind prefix and the trailing attr;
	// when there's no module prefix this is just the resource name.
	// We treat the first segment as kind even for module-prefixed
	// addresses — the calculator never sees the module portion on
	// the kind side of Resource.Ref.
	kind := parts[0]
	name := strings.Join(parts[1:len(parts)-1], ".")
	return Override{
		Kind:  kind,
		Name:  name,
		Attr:  attr,
		Value: coerce(rhs),
	}, nil
}

// coerce narrows a raw value string into the strongest Go type that
// fits. Booleans win first so `true` doesn't accidentally become a
// string; the catalog's `pick(cond, a, b)` would silently break if
// `default(monitored, false)` started returning a string.
func coerce(v string) any {
	switch v {
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.ParseInt(v, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	// Strip surrounding quotes so `--what-if 'x.y.z="abc"'` doesn't
	// produce the string `"abc"` with literal quotes.
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[0] == v[len(v)-1] {
		return v[1 : len(v)-1]
	}
	return v
}

// Apply mutates `resources` in place, layering every Override onto
// the matching resource. Returns the list of overrides that didn't
// match any resource so the caller can surface a warning rather than
// silently dropping them.
func Apply(resources []domain.Resource, overrides []Override) (unmatched []Override) {
	for _, o := range overrides {
		matched := false
		for i, r := range resources {
			if r.Ref.Kind == o.Kind && r.Ref.Name == o.Name {
				if resources[i].Attributes == nil {
					resources[i].Attributes = map[string]any{}
				}
				resources[i].Attributes[o.Attr] = o.Value
				matched = true
			}
		}
		if !matched {
			unmatched = append(unmatched, o)
		}
	}
	return unmatched
}
