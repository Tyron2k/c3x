package recommend

import (
	"strings"

	"github.com/c3xdev/c3x/internal/domain"
)

// AzureRules returns the default Azure-targeted rule set. Tree rules
// live in AzureTreeRules.
func AzureRules() []Rule {
	return []Rule{
		&AzureBurstableForDev{},
		&AzureStorageCoolTier{},
		&AzurePostgresB1Right{},
		&AzureLogAnalyticsRetention{},
		&AzureRedisBasicForDev{},
		&AzureUnattachedDisk{},
	}
}

// AzureTreeRules returns cross-resource Azure analyses.
func AzureTreeRules() []TreeRule {
	return []TreeRule{
		&AzureOrphanedDisk{},
	}
}

// AzureOrphanedDisk flags managed disks that aren't referenced by any
// VM in the parsed configuration. The per-resource AzureUnattachedDisk
// rule is a soft prompt that fires on every disk above 100 GB; this
// tree rule is stricter — it only fires when no VM in the config
// names the disk via os_disk or data_disk blocks, so it has higher
// confidence the disk is genuinely orphaned.
type AzureOrphanedDisk struct{}

func (AzureOrphanedDisk) Name() string { return "azure.disk.orphaned" }

func (AzureOrphanedDisk) ProposeTree(resources []domain.Resource) []TreeProposal {
	// Build the set of disk names referenced by any VM. Azure HCL
	// surfaces these as either `os_disk.name` (the implicit OS disk,
	// which never shows up as its own azurerm_managed_disk) or
	// `data_disk.managed_disk_id` (which references the disk by
	// resource ID — the parser flattens those into a string).
	referenced := map[string]bool{}
	for _, r := range resources {
		if r.Ref.Kind != "azurerm_linux_virtual_machine" &&
			r.Ref.Kind != "azurerm_windows_virtual_machine" {
			continue
		}
		// data_disk references arrive in attribute keys shaped like
		// `data_disk.managed_disk_id`, `data_disk_0_managed_disk_id`,
		// etc. depending on how the parser flattens nested blocks.
		// Walk every attribute value and record any string that looks
		// like a disk reference.
		for k, v := range r.Attributes {
			if !strings.Contains(k, "data_disk") && !strings.Contains(k, "managed_disk") {
				continue
			}
			s, ok := v.(string)
			if !ok {
				continue
			}
			referenced[lastSegment(s)] = true
		}
	}

	var props []TreeProposal
	for _, r := range resources {
		if r.Ref.Kind != "azurerm_managed_disk" {
			continue
		}
		if referenced[r.Ref.Name] {
			continue
		}
		gb, _ := r.AttrInt("disk_size_gb")
		if gb == 0 {
			continue
		}
		props = append(props, TreeProposal{
			PrimaryRef: r.Ref,
			Category:   "unused-resources",
			Title:      "Delete orphaned managed disk",
			Description: "No VM in this configuration references this disk via os_disk or data_disk. " +
				"Managed disks bill regardless of attachment; snapshot then delete if unneeded.",
			Changes: map[domain.Reference]map[string]any{
				r.Ref: {"disk_size_gb": int64(0)},
			},
		})
	}
	return props
}

// lastSegment returns the trailing path/identifier component of an
// Azure resource ID — `/subscriptions/.../disks/data-1` → `data-1`.
// For non-paths it returns the input unchanged.
func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// AzureBurstableForDev suggests B-series VMs for non-prod where
// average CPU is low. Cuts cost ~50% vs the D-series at the price
// of lower sustained performance.
type AzureBurstableForDev struct{}

func (AzureBurstableForDev) Name() string { return "azure.vm.burstable-for-dev" }

func (AzureBurstableForDev) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "azurerm_linux_virtual_machine" &&
		r.Ref.Kind != "azurerm_windows_virtual_machine" {
		return nil
	}
	if !looksLikeNonProd(r.Ref.Name) {
		return nil
	}
	size, ok := r.AttrString("size")
	if !ok || !strings.HasPrefix(size, "Standard_") {
		return nil
	}
	suggested := mapAzureToBurstable(size)
	if suggested == "" || suggested == size {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Swap dev VM to a B-series burstable",
		Description: "B-series VMs cost ~50% less than D-series at the same vCPU count, with full " +
			"performance available in bursts. Suitable for dev/test/staging.",
		AttributeChanges: map[string]any{"size": suggested},
	}}
}

func mapAzureToBurstable(d string) string {
	switch d {
	case "Standard_D2s_v3", "Standard_D2s_v4", "Standard_D2s_v5":
		return "Standard_B2s_v2"
	case "Standard_D4s_v3", "Standard_D4s_v4", "Standard_D4s_v5":
		return "Standard_B4s_v2"
	case "Standard_D8s_v3", "Standard_D8s_v4", "Standard_D8s_v5":
		return "Standard_B8s_v2"
	}
	return ""
}

// AzureStorageCoolTier flags blob storage accounts above 500 GB and
// suggests the Cool access tier for infrequently-accessed data.
type AzureStorageCoolTier struct{}

func (AzureStorageCoolTier) Name() string { return "azure.storage.cool-tier" }

func (AzureStorageCoolTier) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "azurerm_storage_account" {
		return nil
	}
	gb, ok := r.AttrInt("monthly_storage_gb")
	if !ok || gb < 500 {
		return nil
	}
	return []Proposal{{
		Category: "lifecycle",
		Title:    "Move infrequently-accessed blobs to Cool tier",
		Description: "Cool tier is ~50% cheaper per GB than Hot. Profile access patterns first — Cool " +
			"charges per-operation more, so frequently-read data is more expensive.",
		AttributeChanges: map[string]any{},
	}}
}

// AzurePostgresB1Right suggests Burstable tier for non-prod
// Postgres Flexible Server deployments.
type AzurePostgresB1Right struct{}

func (AzurePostgresB1Right) Name() string { return "azure.postgres.b-tier" }

func (AzurePostgresB1Right) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "azurerm_postgresql_flexible_server" {
		return nil
	}
	sku, _ := r.AttrString("sku_name")
	if !strings.HasPrefix(sku, "GP_") {
		return nil
	}
	if !looksLikeNonProd(r.Ref.Name) {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Move non-prod Postgres to Burstable (B) tier",
		Description: "Postgres Flexible Server's Burstable tier is ~70% cheaper than General Purpose. " +
			"Suitable for dev/staging with intermittent traffic.",
		AttributeChanges: map[string]any{"sku_name": "B_Standard_B1ms"},
	}}
}

// AzureLogAnalyticsRetention flags workspaces ingesting > 10 GB/mo.
type AzureLogAnalyticsRetention struct{}

func (AzureLogAnalyticsRetention) Name() string { return "azure.log-analytics.retention" }

func (AzureLogAnalyticsRetention) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "azurerm_log_analytics_workspace" {
		return nil
	}
	gb, ok := r.AttrInt("monthly_retained_gb")
	if !ok || gb < 100 {
		return nil
	}
	suggested := gb / 2
	return []Proposal{{
		Category: "lifecycle",
		Title:    "Cap Log Analytics retention",
		Description: "Workspaces retain logs at $0.10/GB-month after the included 31 days. An explicit " +
			"retention_in_days commensurate with your debugging window cuts the bill.",
		AttributeChanges: map[string]any{"monthly_retained_gb": suggested},
	}}
}

// AzureRedisBasicForDev suggests the Basic tier for non-prod Redis.
type AzureRedisBasicForDev struct{}

func (AzureRedisBasicForDev) Name() string { return "azure.redis.basic-non-prod" }

func (AzureRedisBasicForDev) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "azurerm_redis_cache" {
		return nil
	}
	sku, _ := r.AttrString("sku_name")
	if sku == "Basic" {
		return nil
	}
	if !looksLikeNonProd(r.Ref.Name) {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Use Basic Redis tier in non-prod",
		Description: "Standard tier adds replication for HA at roughly 2× the Basic price. Dev/staging " +
			"tolerates the reduced availability.",
		AttributeChanges: map[string]any{"sku_name": "Basic"},
	}}
}

// AzureUnattachedDisk is a soft prompt for stand-alone managed disks.
type AzureUnattachedDisk struct{}

func (AzureUnattachedDisk) Name() string { return "azure.disk.unattached-audit" }

func (AzureUnattachedDisk) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "azurerm_managed_disk" {
		return nil
	}
	gb, ok := r.AttrInt("disk_size_gb")
	if !ok || gb < 100 {
		return nil
	}
	return []Proposal{{
		Category: "unused-resources",
		Title:    "Audit managed disk for orphaned status",
		Description: "Managed disks bill regardless of attachment. Confirm the disk is referenced by " +
			"an active VM; otherwise snapshot + delete.",
		AttributeChanges: map[string]any{},
	}}
}
