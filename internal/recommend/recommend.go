// Package recommend contains c3x's cost-optimisation rule engine.
//
// A Rule inspects one parsed resource and optionally proposes a
// concrete alternative — a modified attribute set the engine
// re-estimates to compute savings. Rules are deliberately small and
// stateless; the engine composes them per resource and emits a
// Recommendation only when the alternative is cheaper.
//
// Recommendations are user-facing data, so each carries a category,
// a one-line title, and a longer description suitable for PR comments
// or terminal rendering. The package is a sibling to calculator (not
// a layer above) because it re-runs calculator.Estimate against the
// hypothetical resource — calling it would mean a circular import the
// other way.
package recommend

import (
	"context"

	"github.com/c3xdev/c3x/internal/calculator"
	"github.com/c3xdev/c3x/internal/domain"
	"github.com/shopspring/decimal"
)

// Recommendation describes one suggested change to a resource.
//
// CurrentCost / SuggestedCost are the engine-computed monthly costs
// for the baseline and proposed states; Savings is their difference
// rounded to 2dp.
type Recommendation struct {
	Resource      domain.Reference
	Category      string
	Title         string
	Description   string
	CurrentCost   decimal.Decimal
	SuggestedCost decimal.Decimal
	Savings       decimal.Decimal
	Currency      domain.Currency
}

// Rule is the contract every per-resource rule implements. Rules
// inspect one resource at a time and propose alternative attribute
// sets the engine then scores.
//
// Each alternative is a complete attribute swap, not a delta — two
// rules touching the same resource never accidentally compound.
type Rule interface {
	Name() string
	Propose(r domain.Resource) []Proposal
}

// TreeRule is the contract for rules that need to see every parsed
// resource to make a decision — NAT-gateway consolidation, idle ALB
// detection, fleet-wide commitment recommendations.
//
// TreeRules return [TreeProposal]s, each binding to a specific
// resource. The engine scores them with the same machinery as
// per-resource proposals.
type TreeRule interface {
	Name() string
	ProposeTree(resources []domain.Resource) []TreeProposal
}

// TreeProposal is the cross-resource analogue of [Proposal]. Unlike
// the per-resource case, a TreeProposal can modify many resources at
// once — `Changes` maps each affected reference to its attribute
// overrides. The engine re-estimates the whole modified set so
// savings reflect any cost shift across the boundary (e.g. NAT
// consolidation: remove NAT processing cost + add inter-AZ data-
// transfer cost on the surviving NAT).
//
// PrimaryRef names the resource the recommendation is "about" for
// display purposes — typically the one being removed or right-sized.
type TreeProposal struct {
	PrimaryRef  domain.Reference
	Category    string
	Title       string
	Description string
	Changes     map[domain.Reference]map[string]any
}

// Proposal is one alternative configuration a Rule wants the engine
// to score. AttributeChanges replaces the values for the listed
// attribute keys; everything else carries over from the source
// resource.
type Proposal struct {
	Category         string
	Title            string
	Description      string
	AttributeChanges map[string]any
}

// Engine runs rules against parsed resources and returns the
// recommendations whose alternative is cheaper than the baseline.
//
// We always re-estimate using the same calculator the user's
// `c3x estimate` would invoke, so the savings math is consistent
// across surfaces.
type Engine struct {
	rules     []Rule
	treeRules []TreeRule
	calc      *calculator.Engine
}

// New constructs an Engine with per-resource rules. Tree rules are
// added via [Engine.RegisterTreeRule].
func New(calc *calculator.Engine, rules ...Rule) *Engine {
	return &Engine{rules: rules, calc: calc}
}

// RegisterTreeRule adds a cross-resource rule to the engine.
// Returns the engine so registrations chain.
func (e *Engine) RegisterTreeRule(r TreeRule) *Engine {
	e.treeRules = append(e.treeRules, r)
	return e
}

// Recommend walks every resource through every rule, scoring each
// proposal against the live engine. The slice returned is sorted by
// savings (largest first) so renderers can show the highest-impact
// suggestion at the top.
func (e *Engine) Recommend(ctx context.Context, resources []domain.Resource) ([]Recommendation, error) {
	baseline, err := e.calc.Estimate(ctx, resources)
	if err != nil {
		return nil, err
	}
	baselineCosts := indexCosts(baseline)

	var out []Recommendation

	// Per-resource rules.
	resourcesByRef := indexResources(resources)
	for _, r := range resources {
		base := baselineCosts[r.Ref]
		for _, rule := range e.rules {
			for _, p := range rule.Propose(r) {
				rec, ok := e.score(ctx, r, p.AttributeChanges, base, p.Category, p.Title, p.Description, baseline.Currency)
				if ok {
					out = append(out, rec)
				}
			}
		}
	}

	// Tree rules can modify many resources per proposal — savings are
	// computed against the WHOLE modified set so that a change which
	// removes one line item while adding another (e.g. NAT
	// consolidation: drop NAT charges, add inter-AZ transfer) reports
	// the net delta, not just the gross removal.
	for _, rule := range e.treeRules {
		for _, p := range rule.ProposeTree(resources) {
			rec, ok := e.scoreTree(ctx, resources, resourcesByRef, baselineCosts, p, baseline.Currency)
			if ok {
				out = append(out, rec)
			}
		}
	}

	sortBySavings(out)
	return out, nil
}

// scoreTree re-estimates the modified resource set described by a
// TreeProposal and emits a Recommendation when the net delta is a
// saving. The baseline against which we compare is the sum of the
// modified resources' baseline costs — anything outside the proposal
// is unchanged on both sides and cancels.
func (e *Engine) scoreTree(
	ctx context.Context,
	all []domain.Resource,
	byRef map[domain.Reference]domain.Resource,
	baselineCosts map[domain.Reference]domain.Cost,
	p TreeProposal,
	currency domain.Currency,
) (Recommendation, bool) {
	if len(p.Changes) == 0 {
		return Recommendation{}, false
	}

	baselineSum := decimal.Zero
	alts := make([]domain.Resource, 0, len(p.Changes))
	for ref, changes := range p.Changes {
		orig, ok := byRef[ref]
		if !ok {
			return Recommendation{}, false
		}
		baselineSum = baselineSum.Add(baselineCosts[ref].MonthlySubtotal)
		alts = append(alts, applyProposal(orig, Proposal{AttributeChanges: changes}))
	}

	altEst, err := e.calc.Estimate(ctx, alts)
	if err != nil {
		return Recommendation{}, false
	}
	suggested := decimal.Zero
	for _, c := range altEst.Costs {
		suggested = suggested.Add(c.MonthlySubtotal)
	}

	savings := baselineSum.Sub(suggested).Round(2)
	if savings.LessThanOrEqual(decimal.Zero) {
		return Recommendation{}, false
	}
	primary := p.PrimaryRef
	if primary == (domain.Reference{}) {
		// Fall back to first changed ref so downstream renderers still
		// have a resource to print.
		for ref := range p.Changes {
			primary = ref
			break
		}
	}
	_ = all
	return Recommendation{
		Resource:      primary,
		Category:      p.Category,
		Title:         p.Title,
		Description:   p.Description,
		CurrentCost:   baselineSum,
		SuggestedCost: suggested,
		Savings:       savings,
		Currency:      currency,
	}, true
}

// score re-estimates a single alternative resource and produces a
// Recommendation when savings are positive. Common path for both
// per-resource and tree-rule proposals.
func (e *Engine) score(
	ctx context.Context,
	r domain.Resource,
	changes map[string]any,
	base domain.Cost,
	category, title, description string,
	currency domain.Currency,
) (Recommendation, bool) {
	alt := applyProposal(r, Proposal{AttributeChanges: changes})
	altEst, err := e.calc.Estimate(ctx, []domain.Resource{alt})
	if err != nil {
		return Recommendation{}, false
	}
	suggested := decimal.Zero
	if len(altEst.Costs) > 0 {
		suggested = altEst.Costs[0].MonthlySubtotal
	}
	savings := base.MonthlySubtotal.Sub(suggested).Round(2)
	if savings.LessThanOrEqual(decimal.Zero) {
		return Recommendation{}, false
	}
	return Recommendation{
		Resource:      r.Ref,
		Category:      category,
		Title:         title,
		Description:   description,
		CurrentCost:   base.MonthlySubtotal,
		SuggestedCost: suggested,
		Savings:       savings,
		Currency:      currency,
	}, true
}

// indexResources groups parsed resources by reference for O(1) lookup
// when a TreeProposal binds to a specific Ref.
func indexResources(rs []domain.Resource) map[domain.Reference]domain.Resource {
	out := make(map[domain.Reference]domain.Resource, len(rs))
	for _, r := range rs {
		out[r.Ref] = r
	}
	return out
}

// applyProposal builds a copy of r with the proposal's attribute
// changes applied. We deep-copy the Attributes map because the rules
// must not mutate the caller's resource slice.
func applyProposal(r domain.Resource, p Proposal) domain.Resource {
	attrs := make(map[string]any, len(r.Attributes)+len(p.AttributeChanges))
	for k, v := range r.Attributes {
		attrs[k] = v
	}
	for k, v := range p.AttributeChanges {
		attrs[k] = v
	}
	return domain.Resource{Ref: r.Ref, Attributes: attrs, Region: r.Region}
}

// indexCosts returns a map keyed by reference so applyProposal/
// Recommend can look up baseline costs in O(1) per proposal.
func indexCosts(est domain.Estimate) map[domain.Reference]domain.Cost {
	out := make(map[domain.Reference]domain.Cost, len(est.Costs))
	for _, c := range est.Costs {
		out[c.Resource] = c
	}
	return out
}

func sortBySavings(rs []Recommendation) {
	// Insertion sort — small N, want stable order on ties.
	for i := 1; i < len(rs); i++ {
		j := i
		for j > 0 && rs[j].Savings.GreaterThan(rs[j-1].Savings) {
			rs[j], rs[j-1] = rs[j-1], rs[j]
			j--
		}
	}
}
