package main

// loadCatalogAuto is the estimate-path catalog loader: remote
// knowledge base first (pricing-api /catalog), disk cache second,
// embedded snapshot last. Introspection commands (supported-resources,
// doctor) intentionally keep using the embedded snapshot — they
// describe this binary; estimates describe the world.

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/c3xdev/c3x/internal/catalog"
	"github.com/c3xdev/c3x/internal/config"
)

func loadCatalogAuto(ctx context.Context, resolved config.Resolved) (*catalog.Registry, error) {
	cacheDir := ""
	if resolved.CachePath != "" {
		cacheDir = filepath.Dir(resolved.CachePath)
	} else if p, err := config.UserCachePath(); err == nil {
		cacheDir = filepath.Dir(p)
	}
	return catalog.LoadAuto(ctx, catalog.LoadOptions{
		Endpoint: catalogEndpointFrom(resolved.PricingEndpoint),
		CacheDir: cacheDir,
		Offline:  resolved.Offline,
	})
}

// catalogEndpointFrom derives the catalog URL from the pricing
// GraphQL endpoint so self-hosted users configure one host, not two:
// https://host/graphql → https://host/catalog.
func catalogEndpointFrom(pricingEndpoint string) string {
	if pricingEndpoint == "" {
		return catalog.DefaultCatalogEndpoint
	}
	if strings.HasSuffix(pricingEndpoint, "/graphql") {
		return strings.TrimSuffix(pricingEndpoint, "/graphql") + "/catalog"
	}
	return strings.TrimRight(pricingEndpoint, "/") + "/catalog"
}
