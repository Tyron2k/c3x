package pricing

// DefaultPurchaseOption returns the purchaseOption filter applied when a
// catalog mapping doesn't set one explicitly. The values are the labels
// the upstream catalog uses per provider. This is the single source of
// truth shared by the live query path (calculator) and the offline sync
// so a warmed cache is keyed identically to a live lookup.
func DefaultPurchaseOption(provider string) string {
	switch provider {
	case "aws":
		return "on_demand"
	case "azure":
		return "Consumption"
	case "gcp":
		return "OnDemand"
	default:
		return ""
	}
}

// ResolvePurchaseOption returns the effective purchaseOption for a
// mapping: the mapping's own value when set, otherwise the provider
// default. The "any" sentinel is preserved (it means "this product
// carries no purchaseOption label"; the query omits the filter).
func ResolvePurchaseOption(provider, mappingPO string) string {
	if mappingPO != "" {
		return mappingPO
	}
	return DefaultPurchaseOption(provider)
}
