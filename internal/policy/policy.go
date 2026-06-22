// Package policy runs Rego policies against a c3x Estimate.
//
// The policy data model is intentionally narrow so policies stay
// portable — it is a JSON representation of resources + total +
// (optionally) a diff vs baseline. The shape we feed Rego:
//
//	{
//	  "estimate": {
//	    "project_total":  148.16,
//	    "currency":       "USD",
//	    "generated_at":   "2026-06-09T10:00:00Z",
//	    "resources":      [ { "kind", "name", "monthly_cost" }, ... ]
//	  },
//	  "diff": {                            // optional, present only with --baseline
//	    "baseline_total": 50.00,
//	    "current_total":  148.16,
//	    "total_delta":    98.16,
//	    "resources":      [ { "kind", "name", "baseline", "current", "delta" }, ... ]
//	  }
//	}
//
// Policies write rules under the `c3x` package. Two outputs are
// recognised:
//
//	deny[msg]          — list of strings; non-empty denies the run
//	warn[msg]          — list of strings; printed but doesn't fail
//
// The Result struct exposes both lists separately so callers can
// gate (deny → non-zero exit) and surface (warn → stderr line).
package policy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/shopspring/decimal"
)

// Input is the JSON-shaped value Rego evaluates against. Built from
// an Estimate plus an optional Diff.
type Input struct {
	Estimate *estimateInput `json:"estimate"`
	Diff     *diffInput     `json:"diff,omitempty"`
}

type estimateInput struct {
	ProjectTotal float64         `json:"project_total"`
	Currency     string          `json:"currency"`
	GeneratedAt  string          `json:"generated_at,omitempty"`
	Resources    []resourceInput `json:"resources"`
}

type resourceInput struct {
	Kind        string  `json:"kind"`
	Name        string  `json:"name"`
	MonthlyCost float64 `json:"monthly_cost"`
}

type diffInput struct {
	BaselineTotal float64             `json:"baseline_total"`
	CurrentTotal  float64             `json:"current_total"`
	TotalDelta    float64             `json:"total_delta"`
	Resources     []resourceDiffInput `json:"resources"`
}

type resourceDiffInput struct {
	Kind     string  `json:"kind"`
	Name     string  `json:"name"`
	Baseline float64 `json:"baseline"`
	Current  float64 `json:"current"`
	Delta    float64 `json:"delta"`
}

// Result carries the outcome of one Eval call.
type Result struct {
	Denials  []string
	Warnings []string
}

// HasDenials reports whether the policy emitted any deny[...] rules.
// CLI callers exit non-zero in that case.
func (r Result) HasDenials() bool { return len(r.Denials) > 0 }

// Eval loads every `.rego` file under `policyPath` (file or
// directory) and evaluates them against the supplied input. The
// `c3x.deny` and `c3x.warn` rule sets are queried; results are
// flattened into the Result's slices.
func Eval(ctx context.Context, policyPath string, input Input) (Result, error) {
	if policyPath == "" {
		return Result{}, errors.New("policy: empty path")
	}
	policies, err := loadPolicies(policyPath)
	if err != nil {
		return Result{}, err
	}
	if len(policies) == 0 {
		return Result{}, fmt.Errorf("policy: no .rego files found under %q", policyPath)
	}

	res := Result{}
	for _, ruleName := range []string{"deny", "warn"} {
		query := "data.c3x." + ruleName
		opts := []func(*rego.Rego){
			rego.Query(query),
			rego.Input(input),
			// Accept classic `deny[msg] { ... }` syntax (Rego v0):
			// that's the widely-used classic form, so existing policy
			// bundles load unchanged.
			// Policies written in the newer `contains/if` form also
			// parse — v0 is a superset accepted by the v1 engine
			// when the version is pinned here.
			rego.SetRegoVersion(ast.RegoV0),
		}
		for _, p := range policies {
			opts = append(opts, rego.Module(p.path, p.source))
		}
		r := rego.New(opts...)
		rs, err := r.Eval(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("policy: eval %s: %w", query, err)
		}
		messages := collectStringMessages(rs)
		sort.Strings(messages)
		switch ruleName {
		case "deny":
			res.Denials = append(res.Denials, messages...)
		case "warn":
			res.Warnings = append(res.Warnings, messages...)
		}
	}
	return res, nil
}

// BuildInput converts the domain types into the Rego-friendly shape.
// Diff is optional; pass nil to evaluate the estimate alone.
func BuildInput(est domain.Estimate, diff *domain.Diff) Input {
	in := Input{
		Estimate: &estimateInput{
			ProjectTotal: decToFloat(est.ProjectTotal),
			Currency:     est.Currency.String(),
			Resources:    make([]resourceInput, 0, len(est.Costs)),
		},
	}
	if !est.GeneratedAt.IsZero() {
		in.Estimate.GeneratedAt = est.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	for _, c := range est.Costs {
		in.Estimate.Resources = append(in.Estimate.Resources, resourceInput{
			Kind:        c.Resource.Kind,
			Name:        c.Resource.Name,
			MonthlyCost: decToFloat(c.MonthlySubtotal),
		})
	}
	if diff != nil {
		d := &diffInput{
			BaselineTotal: decToFloat(diff.BaselineTotal),
			CurrentTotal:  decToFloat(diff.CurrentTotal),
			TotalDelta:    decToFloat(diff.TotalDelta),
			Resources:     make([]resourceDiffInput, 0, len(diff.Resources)),
		}
		for _, r := range diff.Resources {
			d.Resources = append(d.Resources, resourceDiffInput{
				Kind:     r.Resource.Kind,
				Name:     r.Resource.Name,
				Baseline: decToFloat(r.Baseline),
				Current:  decToFloat(r.Current),
				Delta:    decToFloat(r.Delta),
			})
		}
		in.Diff = d
	}
	return in
}

type loadedPolicy struct {
	path   string
	source string
}

// loadPolicies walks the given path; if it's a file, returns it
// alone, if a directory, returns every .rego under it.
func loadPolicies(root string) ([]loadedPolicy, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("policy path: %w", err)
	}
	var out []loadedPolicy
	if !info.IsDir() {
		data, err := os.ReadFile(root)
		if err != nil {
			return nil, err
		}
		return []loadedPolicy{{path: root, source: string(data)}}, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".rego") {
			continue
		}
		full := filepath.Join(root, e.Name())
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", full, err)
		}
		out = append(out, loadedPolicy{path: full, source: string(data)})
	}
	return out, nil
}

func collectStringMessages(rs rego.ResultSet) []string {
	var out []string
	for _, r := range rs {
		for _, expr := range r.Expressions {
			switch v := expr.Value.(type) {
			case []any:
				for _, item := range v {
					if s, ok := item.(string); ok {
						out = append(out, s)
					}
				}
			case string:
				out = append(out, v)
			}
		}
	}
	return out
}

func decToFloat(d decimal.Decimal) float64 {
	f, _ := d.Float64()
	return f
}
