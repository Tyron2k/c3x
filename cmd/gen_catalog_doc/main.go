// Command gen_catalog_doc renders the catalog as a single markdown
// page grouped by provider, with per-kind status. Output goes to
// docs/catalog.md so the auto-generated page stays in the repo and
// renders on the docs site.
//
// Run from the repo root:
//
//	go run ./cmd/gen_catalog_doc > docs/catalog.md
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/c3xdev/c3x/internal/catalog"
)

func main() {
	reg, err := catalog.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load catalog: %v\n", err)
		os.Exit(1)
	}

	byProvider := map[string][]*catalog.Definition{}
	for _, kind := range reg.Kinds() {
		def := reg.Get(kind)
		if def == nil {
			continue
		}
		byProvider[def.Provider] = append(byProvider[def.Provider], def)
	}

	out := os.Stdout
	fmt.Fprintln(out, "# Supported resources")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Auto-generated from `resources/<provider>/*.toml`. Run")
	fmt.Fprintln(out, "`go run ./cmd/gen_catalog_doc > docs/catalog.md` to refresh.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Status meaning:")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "- **LIVE** — priced against `pricing.c3x.dev`; tracks vendor changes.")
	fmt.Fprintln(out, "- **STATIC** — inline rate (upstream catalog doesn't expose the meter).")
	fmt.Fprintln(out, "  See `docs/upstream-gaps.md` for the per-resource explanation.")
	fmt.Fprintln(out, "- **FREE** — no per-resource charge (parent-billed, structural, IAM).")
	fmt.Fprintln(out)

	type counts struct{ live, static, free int }
	totals := counts{}
	providerCounts := map[string]counts{}

	for _, provider := range []string{"aws", "azure", "gcp"} {
		defs := byProvider[provider]
		if len(defs) == 0 {
			continue
		}
		sort.Slice(defs, func(i, j int) bool { return defs[i].Kind < defs[j].Kind })

		fmt.Fprintf(out, "## %s (%d resources)\n\n", strings.ToUpper(provider), len(defs))
		fmt.Fprintln(out, "| Kind | Status | Display name |")
		fmt.Fprintln(out, "|---|---|---|")
		pc := counts{}
		for _, def := range defs {
			status := statusFor(reg, def)
			switch status {
			case "LIVE":
				pc.live++
				totals.live++
			case "STATIC":
				pc.static++
				totals.static++
			case "FREE":
				pc.free++
				totals.free++
			}
			fmt.Fprintf(out, "| `%s` | %s | %s |\n", def.Kind, status, escapePipes(def.DisplayName))
		}
		providerCounts[provider] = pc
		fmt.Fprintln(out)
		fmt.Fprintf(out, "**%s totals:** %d LIVE · %d STATIC · %d FREE\n\n",
			strings.ToUpper(provider), pc.live, pc.static, pc.free)
	}

	fmt.Fprintln(out, "---")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "**Grand total: %d resources.** %d LIVE · %d STATIC · %d FREE.\n",
		totals.live+totals.static+totals.free, totals.live, totals.static, totals.free)
}

func statusFor(reg *catalog.Registry, def *catalog.Definition) string {
	if catalog.IsLegitimatelyFree(def.Kind) || reg.IsFreeShell(def.Kind) {
		return "FREE"
	}
	if reg.HasStaticRate(def.Kind) {
		return "STATIC"
	}
	return "LIVE"
}

func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
