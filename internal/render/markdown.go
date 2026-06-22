package render

import (
	"fmt"
	"strings"

	"github.com/c3xdev/c3x/internal/domain"
)

// RenderMarkdown formats an Estimate as PR-comment-ready markdown.
// `c3x comment <forge>` adds the c3x-marker / collapsible / cost-trend
// layout on top; this is the lossless baseline.
//
// The output is GitHub-flavored: tables for the line-item breakdown,
// fenced code for the project total, and emoji markers (📦 priced,
// ⚪ free, ⚠️ static-rate) so reviewers can scan visually.
func RenderMarkdown(est domain.Estimate) string {
	var b strings.Builder
	cur := est.Currency
	b.WriteString("## c3x estimate\n\n")
	if len(est.Costs) == 0 {
		b.WriteString("_No resources to estimate._\n")
		return b.String()
	}

	priced := 0
	for _, c := range est.Costs {
		if len(c.LineItems) == 0 {
			continue
		}
		priced++
		marker := "📦"
		if c.HasStaticRate() {
			marker = "⚠️"
		}
		fmt.Fprintf(&b, "### %s `%s`  —  %s%s/mo\n\n", marker, c.Resource.Label(),
			cur.Symbol(), c.MonthlySubtotal)
		b.WriteString("| Dimension | Quantity | Unit rate | Monthly | Source |\n")
		b.WriteString("|---|---:|---:|---:|---|\n")
		for _, li := range c.LineItems {
			fmt.Fprintf(&b, "| %s | %s %s | %s%s | %s%s | %s |\n",
				escapeMD(li.Description),
				li.Quantity, li.Unit,
				cur.Symbol(), li.UnitRate,
				cur.Symbol(), li.MonthlyCost,
				li.PriceSource)
		}
		b.WriteString("\n")
	}

	if priced == 0 {
		b.WriteString("_No resources priced (offline mode or unknown kinds)._\n")
		return b.String()
	}

	fmt.Fprintf(&b, "**Project total: %s%s/mo**\n", cur.Symbol(), est.ProjectTotal)
	return b.String()
}

// RenderMarkdownDiff formats a Diff for PR comments. Resources are
// grouped by change kind so reviewers see Added/Removed/Modified
// sections rather than a flat list. The Δ column carries an
// up/down/flat indicator — at a glance reviewers see which way the
// bill moves without parsing signed numbers.
func RenderMarkdownDiff(d domain.Diff) string {
	var b strings.Builder
	cur := d.Currency
	b.WriteString("## c3x diff\n\n")
	fmt.Fprintf(&b, "**Total: %s%s/mo → %s%s/mo  %s**\n\n",
		cur.Symbol(), d.BaselineTotal,
		cur.Symbol(), d.CurrentTotal,
		signedWithIndicator(cur.Symbol(), d.TotalDelta.String()))

	groups := map[domain.DeltaKind][]domain.ResourceDelta{}
	for _, r := range d.Resources {
		groups[r.Kind] = append(groups[r.Kind], r)
	}
	for _, kind := range []struct {
		k     domain.DeltaKind
		emoji string
		label string
	}{
		{domain.DeltaAdded, "🟢", "Added"},
		{domain.DeltaModified, "🟡", "Modified"},
		{domain.DeltaRemoved, "🔴", "Removed"},
	} {
		rs := groups[kind.k]
		if len(rs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s %s\n\n", kind.emoji, kind.label)
		b.WriteString("| Resource | Baseline | Current | Δ |\n")
		b.WriteString("|---|---:|---:|---:|\n")
		for _, r := range rs {
			fmt.Fprintf(&b, "| `%s` | %s%s | %s%s | %s |\n",
				r.Resource.Label(),
				cur.Symbol(), r.Baseline,
				cur.Symbol(), r.Current,
				signedWithIndicator(cur.Symbol(), r.Delta.String()))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// escapeMD escapes the pipe character so resource labels and
// descriptions don't break the table layout. We don't escape `_` or
// `*` because GitHub renders them harmlessly inside table cells.
func escapeMD(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}
