package recommend

import (
	"strings"

	"github.com/c3xdev/c3x/internal/domain"
)

// GCPRules returns the default GCP-targeted rule set.
func GCPRules() []Rule {
	return []Rule{
		&GCPPDStandardToBalanced{},
		&GCPCloudSQLNonProdTier{},
		&GCPStorageBucketColdline{},
		&GCPUnusedStaticIP{},
		&GCPE2InsteadOfN1{},
		&GCPLoggingRetention{},
	}
}

// GCPTreeRules returns cross-resource GCP analyses.
func GCPTreeRules() []TreeRule {
	return []TreeRule{
		&GCPCommittedUseEligible{},
	}
}

// GCPCommittedUseEligible surfaces fleet-level Committed Use Discount
// opportunities. Heuristic: when three or more google_compute_instance
// resources share the same machine-type FAMILY (e.g. all `e2-*`, all
// `n2-*`), the workload is steady enough to justify a 1- or 3-year
// commitment. CUDs save ~25% (1yr) or ~52% (3yr) off list price for
// vCPU + memory consumed within the commitment.
//
// The proposal doesn't change resource attributes directly — it's a
// soft prompt rendered as a Recommendation with zero AttributeChanges
// per affected resource (which still routes through the engine's
// score path, but reports the fleet-level opportunity in the title).
type GCPCommittedUseEligible struct{}

func (GCPCommittedUseEligible) Name() string { return "gcp.compute.cud-eligible" }

func (GCPCommittedUseEligible) ProposeTree(resources []domain.Resource) []TreeProposal {
	// Bucket Compute instances by machine-family prefix (e2, n1, n2,
	// n2d, c2, ...). A family with ≥3 sustained members is a CUD
	// candidate; under that, the per-instance commitment minimums
	// usually don't pay back.
	byFamily := map[string][]domain.Resource{}
	for _, r := range resources {
		if r.Ref.Kind != "google_compute_instance" {
			continue
		}
		mt, _ := r.AttrString("machine_type")
		family := machineFamily(mt)
		if family == "" {
			continue
		}
		byFamily[family] = append(byFamily[family], r)
	}

	var props []TreeProposal
	for family, fleet := range byFamily {
		if len(fleet) < 3 {
			continue
		}
		// Build a multi-target proposal that doesn't change pricing
		// inputs (CUDs are a billing-side discount, not a config
		// change). The proposal carries a `cud_purchased` attribute
		// the calculator currently ignores — that's intentional, the
		// rule is a soft surface for the Recommendation panel rather
		// than an automatic savings claim. Engine drops the proposal
		// when net savings are zero, so the recommendation only
		// shows up if the catalog later starts honouring CUD
		// attributes.
		changes := map[domain.Reference]map[string]any{}
		for _, r := range fleet {
			changes[r.Ref] = map[string]any{"cud_purchased": true}
		}
		props = append(props, TreeProposal{
			PrimaryRef: fleet[0].Ref,
			Category:   "commitments",
			Title:      "Evaluate Committed Use Discount for " + family + " fleet",
			Description: "Three or more " + family + "-series instances detected. A 1-year resource-based " +
				"commitment for the aggregate vCPU + memory saves ~25%; 3-year saves ~52%. Confirm " +
				"steady-state utilisation before purchasing — commitments are non-refundable.",
			Changes: changes,
		})
	}
	return props
}

// machineFamily extracts the family prefix from a GCE machine type
// string. `e2-standard-4` → `e2`; `n2d-highmem-8` → `n2d`. Returns
// empty for an unrecognised shape.
func machineFamily(mt string) string {
	idx := strings.Index(mt, "-")
	if idx <= 0 {
		return ""
	}
	return mt[:idx]
}

// GCPPDStandardToBalanced suggests upgrading pd-standard volumes to
// pd-balanced — cheaper per IOPS for most workloads and on the
// deprecation path for pd-standard.
type GCPPDStandardToBalanced struct{}

func (GCPPDStandardToBalanced) Name() string { return "gcp.disk.pd-standard-to-balanced" }

func (GCPPDStandardToBalanced) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "google_compute_disk" {
		return nil
	}
	t, _ := r.AttrString("type")
	if t != "pd-standard" {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Switch pd-standard to pd-balanced",
		Description: "pd-balanced delivers ~3× the IOPS-per-GB of pd-standard at similar capacity " +
			"cost, and pd-standard is on the Google deprecation path.",
		AttributeChanges: map[string]any{"type": "pd-balanced"},
	}}
}

// GCPCloudSQLNonProdTier suggests db-f1-micro for non-prod Cloud SQL.
type GCPCloudSQLNonProdTier struct{}

func (GCPCloudSQLNonProdTier) Name() string { return "gcp.cloudsql.non-prod-tier" }

func (GCPCloudSQLNonProdTier) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "google_sql_database_instance" {
		return nil
	}
	tier, _ := r.AttrString("tier")
	if strings.HasPrefix(tier, "db-f1") || strings.HasPrefix(tier, "db-g1") {
		return nil
	}
	if !looksLikeNonProd(r.Ref.Name) {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Drop non-prod Cloud SQL to db-f1-micro",
		Description: "db-f1-micro is ~10× cheaper than the smallest Standard tier and adequate for " +
			"dev/test workloads with light query volume.",
		AttributeChanges: map[string]any{"tier": "db-f1-micro"},
	}}
}

// GCPStorageBucketColdline flags storage buckets above 1 TB.
type GCPStorageBucketColdline struct{}

func (GCPStorageBucketColdline) Name() string { return "gcp.storage.coldline" }

func (GCPStorageBucketColdline) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "google_storage_bucket" {
		return nil
	}
	gb, ok := r.AttrInt("monthly_storage_gb")
	if !ok || gb < 1000 {
		return nil
	}
	return []Proposal{{
		Category: "lifecycle",
		Title:    "Add a Coldline lifecycle rule for cold objects",
		Description: "Buckets > 1 TB usually have a long tail of cold objects. Coldline class is ~75% " +
			"cheaper per GB than Standard with 90-day minimum storage duration.",
		AttributeChanges: map[string]any{},
	}}
}

// GCPUnusedStaticIP flags reserved external IP addresses.
type GCPUnusedStaticIP struct{}

func (GCPUnusedStaticIP) Name() string { return "gcp.ip.unused" }

func (GCPUnusedStaticIP) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "google_compute_address" {
		return nil
	}
	inUse, ok := r.AttrBool("in_use")
	if ok && inUse {
		return nil
	}
	return []Proposal{{
		Category: "unused-resources",
		Title:    "Release unused static IP",
		Description: "Reserved IPs bill regardless of attachment. If this address isn't actively " +
			"serving traffic, release it.",
		AttributeChanges: map[string]any{},
	}}
}

// GCPE2InsteadOfN1 suggests moving N1 family instances to E2.
type GCPE2InsteadOfN1 struct{}

func (GCPE2InsteadOfN1) Name() string { return "gcp.compute.e2-instead-of-n1" }

func (GCPE2InsteadOfN1) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "google_compute_instance" {
		return nil
	}
	mt, _ := r.AttrString("machine_type")
	if !strings.HasPrefix(mt, "n1-") {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Migrate N1 to E2 for general-purpose workloads",
		Description: "E2 machine types deliver similar performance at ~30% lower cost for general " +
			"workloads. N1 remains preferred for sole-tenant or GPU-attached use cases.",
		AttributeChanges: map[string]any{"machine_type": "e2-standard-2"},
	}}
}

// GCPLoggingRetention surfaces ingestion-volume optimisation for
// Cloud Logging buckets.
type GCPLoggingRetention struct{}

func (GCPLoggingRetention) Name() string { return "gcp.logging.retention" }

func (GCPLoggingRetention) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "google_logging_project_bucket" {
		return nil
	}
	gib, ok := r.AttrInt("monthly_ingestion_gib")
	if !ok || gib < 50 {
		return nil
	}
	return []Proposal{{
		Category: "lifecycle",
		Title:    "Cap Cloud Logging ingestion",
		Description: "Default retention is 30 days; extending bucket retention raises monthly cost in " +
			"proportion to ingested volume. Long-term needs belong in a Storage bucket export sink.",
		AttributeChanges: map[string]any{"monthly_ingestion_gib": gib / 2},
	}}
}
