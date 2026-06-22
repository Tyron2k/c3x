package render

import (
	"fmt"
	"strings"

	"github.com/c3xdev/c3x/internal/domain"
)

// RenderText formats an Estimate as a terminal-friendly breakdown. The
// layout uses box-drawing characters for visual structure; callers
// targeting plain ASCII should use [FormatJSON] instead.
//
// Static-rate items get a `(static)` annotation so users see at a
// glance which line items don't track upstream price changes.
func RenderText(est domain.Estimate) string {
	if len(est.Costs) == 0 {
		return "c3x: no resources to estimate.\n"
	}
	var b strings.Builder
	cur := est.Currency
	fmt.Fprintf(&b, "── c3x estimate · %s ─────────────────────────────────────────\n\n", cur)

	priced := 0
	for _, c := range est.Costs {
		if len(c.LineItems) == 0 {
			continue
		}
		priced++
		label := c.Resource.Label()
		annot := ""
		if c.HasStaticRate() {
			annot = "  (some line items use static rates)"
		}
		fmt.Fprintf(&b, "  %s%s\n", label, annot)
		for _, li := range c.LineItems {
			src := ""
			if li.PriceSource == domain.PriceSourceStatic {
				src = " static"
			}
			fmt.Fprintf(&b, "    %s\n", li.Description)
			fmt.Fprintf(&b, "      %s %s × %s%s = %s%s/mo%s\n",
				li.Quantity, li.Unit,
				cur.Symbol(), li.UnitRate,
				cur.Symbol(), li.MonthlyCost,
				src)
		}
		fmt.Fprintf(&b, "    %s subtotal: %s%s/mo\n\n", label, cur.Symbol(), c.MonthlySubtotal)
	}

	if priced == 0 {
		fmt.Fprintf(&b, "  %d resources parsed; none priced.\n", len(est.Costs))
		fmt.Fprintln(&b, "  This usually means --offline, an unknown resource kind,")
		fmt.Fprintln(&b, "  or that pricing.c3x.dev returned no matching products.")
		return b.String()
	}

	b.WriteString("  ────────────────────────────────────────────────────────────\n")
	fmt.Fprintf(&b, "  PROJECT TOTAL: %s%s/mo\n", cur.Symbol(), est.ProjectTotal)
	return b.String()
}

// RenderTextDiff formats a Diff in the same visual idiom as
// [RenderText] but with +/- markers and a delta column.
func RenderTextDiff(d domain.Diff) string {
	var b strings.Builder
	cur := d.Currency
	fmt.Fprintf(&b, "── c3x diff · %s ─────────────────────────────────────────────\n\n", cur)
	for _, r := range d.Resources {
		marker := deltaMarker(r.Kind)
		fmt.Fprintf(&b, "  %s %s\n", marker, r.Resource.Label())
		fmt.Fprintf(&b, "      baseline: %s%s/mo   current: %s%s/mo   Δ %s%s\n",
			cur.Symbol(), r.Baseline,
			cur.Symbol(), r.Current,
			signed(cur.Symbol(), r.Delta.String()),
			"")
	}
	b.WriteString("\n  ────────────────────────────────────────────────────────────\n")
	fmt.Fprintf(&b, "  TOTAL: %s%s/mo  →  %s%s/mo   (Δ %s)\n",
		cur.Symbol(), d.BaselineTotal,
		cur.Symbol(), d.CurrentTotal,
		signed(cur.Symbol(), d.TotalDelta.String()))
	return b.String()
}

func deltaMarker(k domain.DeltaKind) string {
	switch k {
	case domain.DeltaAdded:
		return "+"
	case domain.DeltaRemoved:
		return "-"
	case domain.DeltaModified:
		return "~"
	default:
		return " "
	}
}

// signed renders a signed delta with the currency symbol attached to
// the numeric portion. Negative numbers already carry their `-` from
// shopspring/decimal so we don't double-print.
func signed(sym, val string) string {
	if strings.HasPrefix(val, "-") {
		return "-" + sym + strings.TrimPrefix(val, "-")
	}
	return "+" + sym + val
}

// signedWithIndicator returns a signed value prefixed with an
// up/down/flat arrow. Used in PR-comment markdown so reviewers see
// the direction of change at a glance. Pure zero (no change)
// renders as a dash.
func signedWithIndicator(sym, val string) string {
	switch {
	case strings.HasPrefix(val, "-"):
		return "🔻 -" + sym + strings.TrimPrefix(val, "-")
	case val == "0" || val == "0.00":
		return "—"
	default:
		return "🔺 +" + sym + val
	}
}
