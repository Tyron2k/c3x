// Package catalog owns the declarative resource definitions. Each file
// under `resources/<provider>/<kind>.toml` describes how to price one
// IaC resource type: which upstream products to query, which attributes
// drive the filter, and which billable dimensions contribute line items.
//
// The catalog is loaded once at startup into a Registry that the
// calculator queries by Kind. Catalog files are data, not code; adding
// a new resource is a TOML file and a verifier-harness entry — never a
// new Go function.
package catalog

// Definition is one TOML file's parsed contents. The struct shape is
// stable across versions because it's the public schema for catalog
// authors; field additions must be backward-compatible.
type Definition struct {
	// Kind is the IaC resource type (e.g. "aws_instance"). Catalog files
	// are keyed by Kind and one file must define exactly one Kind.
	Kind string `toml:"kind"`

	// DisplayName is the human-friendly label rendered in breakdowns.
	DisplayName string `toml:"display_name"`

	// Provider is "aws", "azure", or "gcp". Used to dispatch the
	// per-cloud purchaseOption default and other small variations.
	Provider string `toml:"provider"`

	// Mappings is the set of upstream-catalog filter recipes referenced
	// by dimensions via `price("<name>")`. A mapping resolves to a single
	// per-unit rate when looked up against the pricing source.
	Mappings map[string]PriceMapping `toml:"mappings"`

	// Dimensions are the billable rows that contribute to the resource's
	// monthly cost. Each dimension produces zero or one LineItem.
	Dimensions []DimensionSpec `toml:"dimensions"`

	// Fixture is the canonical test resource for this kind, used by the
	// verifier harness and snapshot tests. Each TOML carries its own
	// fixture so it's impossible to add a kind without exercising it,
	// and so the expected monthly cost serves as a regression sentinel
	// — a silent rate change in the upstream catalogue surfaces as a
	// failed snapshot rather than a passing-but-wrong estimate.
	Fixture *Fixture `toml:"fixture"`
}

// Fixture is the snapshot test contract for a kind: a representative
// set of attributes plus the expected monthly cost in USD.
//
// LIVE rates can drift with the upstream catalogue, so the verifier
// applies a tolerance band (±5% by default) when comparing the
// computed cost to ExpectedMonthlyCost. STATIC rates are exact —
// inline literals don't move.
//
// Region defaults to the per-provider canonical region (us-east-1 /
// eastus / us-central1) unless explicitly set.
type Fixture struct {
	Attributes          map[string]any `toml:"attributes"`
	ExpectedMonthlyCost float64        `toml:"expected_monthly_cost"`
	Region              string         `toml:"region"`
	// Tolerance is the fractional band around ExpectedMonthlyCost the
	// verifier accepts. 0 means "use the default" (5%); explicit 0.0
	// means "exact match" via the sentinel below.
	Tolerance float64 `toml:"tolerance"`
	// Exact, when true, demands the computed cost equal
	// ExpectedMonthlyCost to the cent. Used for STATIC rates that
	// won't drift.
	Exact bool `toml:"exact"`
	// LastVerified is the date (YYYY-MM-DD) the snapshot value was
	// validated against the vendor's published pricing page. The
	// verifier emits [STALE] when this falls beyond the freshness
	// window so STATIC rates don't silently rot.
	LastVerified string `toml:"last_verified"`
}

// PriceMapping is a named pointer into the upstream pricing catalogue.
// Authors compose Mappings so multiple dimensions can share the same
// filter shape (e.g. an EC2 instance referenced by both a compute and a
// data-out dimension).
type PriceMapping struct {
	// Service is the upstream catalogue's service code
	// (e.g. "AmazonEC2", "Azure Database for PostgreSQL").
	Service string `toml:"service"`

	// ProductFamily narrows the lookup within Service. Some providers
	// don't use this and may have it empty; the loader treats empty as
	// "any product family".
	ProductFamily string `toml:"product_family"`

	// Region overrides the resource's default region for this lookup.
	// Use "global" to omit the region filter entirely — required for
	// services that aren't priced per-region (CloudFront, GKE control
	// plane, Cloud Run, Log Analytics).
	Region string `toml:"region"`

	// PurchaseOption optionally filters the prices sub-field. Empty
	// means the per-provider default (on_demand for AWS, Consumption
	// for Azure, OnDemand for GCP).
	//
	// Set to the sentinel "any" when the upstream's price entries
	// don't carry a purchaseOption label and the default filter would
	// match zero rows. The HTTP source then omits the filter clause
	// entirely so every priced entry is considered.
	PurchaseOption string `toml:"purchase_option"`

	// Unit optionally filters prices by the upstream unit string. Used
	// when a product exposes multiple units (e.g. "GB" vs "GB-Month").
	Unit string `toml:"unit"`

	// AttributeFilters narrow the lookup further. Each entry is either
	// a static literal or an expression evaluated against the parsed
	// resource's attributes; see AttributeFilter.
	AttributeFilters []AttributeFilter `toml:"attribute_filters"`
}

// AttributeFilter is one upstream-attribute constraint. Catalog authors
// write either `{ key = "x", const = "y" }` (literal) or
// `{ key = "x", expr = "default(foo, \"bar\")" }` (expression).
//
// The two fields are mutually exclusive: the loader rejects entries
// that set both, and the calculator treats a non-empty Expr as the
// authoritative value if both are set anyway.
type AttributeFilter struct {
	Key string `toml:"key"`

	// Literal is the static value (TOML key: `const`, renamed because
	// `const` is a reserved word in Go).
	Literal string `toml:"const"`

	// Expr is an expression evaluated against the resource's attributes
	// at lookup time.
	Expr string `toml:"expr"`
}

// DimensionSpec describes one billable row in a resource's breakdown.
// At calculation time the engine evaluates Quantity and Rate against
// the resource's attributes and the active pricing source.
type DimensionSpec struct {
	// ID is the stable internal identifier (e.g. "compute_hours").
	ID string `toml:"id"`

	// Label is the user-facing description rendered in breakdowns.
	Label string `toml:"label"`

	// Unit is the displayed unit string ("hours", "GB-month", …).
	Unit string `toml:"unit"`

	// Quantity is an expression returning the resolved per-month
	// quantity. Stdlib helpers: monthly_hours(), default(x, fallback).
	Quantity string `toml:"quantity"`

	// Rate is an expression returning the per-unit rate. Most catalog
	// files use `price("<mapping>")`; some fall back to inline literals
	// (e.g. "0.005") when the upstream catalog doesn't expose the meter.
	// Inline literals are flagged as PriceSourceStatic in the LineItem
	// so the verifier and renderer can surface them distinctly.
	Rate string `toml:"rate"`

	// When is an optional predicate. If set and evaluates to false, the
	// dimension is skipped (no LineItem). Used for tier branching
	// (e.g. NAT gateway data only billed when traffic > 0).
	When string `toml:"when"`

	// Constants are exposed under the same names in the dimension's
	// expression scope. Useful for one-shot magic numbers that don't
	// belong in the resource attributes.
	Constants map[string]any `toml:"constants"`
}
