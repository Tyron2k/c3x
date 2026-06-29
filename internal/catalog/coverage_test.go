package catalog

import "testing"

// mustBePriced is the canonical set of high-cost resource kinds that
// must always resolve to a real priced definition — never a free.toml
// entry, never missing. These are the resources whose cost dominates a
// typical bill; silently dropping one (a deleted TOML, or an accidental
// move into the free list) would make c3x *under*-report, which is far
// worse than over-reporting because it gives false confidence.
//
// This list is the durable guard behind the under-coverage audit: it
// runs offline on every CI push, so a coverage regression fails fast
// rather than surfacing as a quietly-wrong estimate in production.
var mustBePriced = []string{
	// AWS
	"aws_cognito_user_pool",
	"aws_vpc_endpoint",
	"aws_lambda_function",
	"aws_eks_cluster",
	"aws_ecs_service",
	"aws_fsx_lustre_file_system",
	"aws_globalaccelerator_accelerator",
	"aws_transfer_server",
	"aws_mq_broker",
	"aws_msk_cluster",
	"aws_redshift_cluster",
	"aws_opensearch_domain",
	"aws_nat_gateway",
	"aws_db_instance",
	"aws_rds_cluster",
	// Azure
	"azurerm_private_endpoint",
	"azurerm_kubernetes_cluster",
	"azurerm_postgresql_flexible_server",
	"azurerm_cosmosdb_account",
	"azurerm_application_gateway",
	"azurerm_firewall",
	"azurerm_linux_function_app",
	"azurerm_log_analytics_workspace",
	// GCP
	"google_container_cluster",
	"google_sql_database_instance",
	"google_compute_instance",
	"google_redis_instance",
	"google_filestore_instance",
	"google_dns_managed_zone",
}

// isPriced reports whether a Definition carries real billable structure
// rather than the synthesised free-kind sentinel (a single dimension
// with ID "free"; see loadFreeKinds). Inline-literal-rate resources
// (e.g. azurerm_linux_function_app at $0.20/1M) have no Mappings but a
// genuine dimension, so the sentinel shape — not len(Mappings) — is the
// correct discriminator.
func isPriced(def *Definition) bool {
	if def == nil {
		return false
	}
	if len(def.Dimensions) == 1 && def.Dimensions[0].ID == "free" {
		return false
	}
	return len(def.Dimensions) > 0
}

func TestMustBePricedKindsAreCovered(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	for _, kind := range mustBePriced {
		def := reg.Get(kind)
		if def == nil {
			t.Errorf("%s: not registered (deleted TOML or typo?) — must be priced", kind)
			continue
		}
		if !isPriced(def) {
			t.Errorf("%s: resolves to the free-kind sentinel — a costed resource must not be in free.toml", kind)
		}
		if def.Fixture == nil {
			t.Errorf("%s: missing [fixture] block — priced kinds must carry a regression snapshot", kind)
		}
	}
}
