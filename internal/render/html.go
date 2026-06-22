package render

// Single-file HTML report renderer. The output is a complete,
// self-contained HTML document with inline CSS so users can attach
// it to a Jira ticket / Slack message / email without external
// asset paths breaking.
//
// Design choices:
//   - inline CSS, no JS — works in restricted iframe sandboxes (e.g.
//     GitLab Pages preview), email clients, and stripped browsers.
//   - per-resource <details> blocks so the per-dimension breakdown
//     collapses by default (the body stays scannable for 100+
//     resource projects).
//   - cost colour scale shaded by magnitude: low cost = green-ish,
//     high cost = amber/red.
//
// We use html/template (not text/template) for autoescape — any
// resource name with `<` or `&` is rendered safely.

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/shopspring/decimal"
)

// RenderHTML produces a complete HTML document representing the
// estimate. Output is a `<!doctype html>` page with inline styling,
// suitable for direct viewing or attachment.
func RenderHTML(est domain.Estimate) (string, error) {
	ceiling := decimal.Zero
	for _, c := range est.Costs {
		if c.MonthlySubtotal.GreaterThan(ceiling) {
			ceiling = c.MonthlySubtotal
		}
	}

	data := htmlData{
		Currency:      est.Currency.Symbol(),
		ProjectTotal:  est.ProjectTotal.Round(2).String(),
		ResourceCount: len(est.Costs),
		Costs:         make([]htmlCost, 0, len(est.Costs)),
	}
	if !est.GeneratedAt.IsZero() {
		data.GeneratedAt = est.GeneratedAt.UTC().Format("2006-01-02 15:04:05 UTC")
	}
	for _, c := range est.Costs {
		row := htmlCost{
			Kind:     c.Resource.Kind,
			Name:     c.Resource.Name,
			Subtotal: c.MonthlySubtotal.Round(2).String(),
			Shade:    shadeFor(c.MonthlySubtotal, ceiling),
			IsZero:   c.MonthlySubtotal.IsZero(),
			Items:    make([]htmlLineItem, 0, len(c.LineItems)),
		}
		for _, li := range c.LineItems {
			row.Items = append(row.Items, htmlLineItem{
				Label:    li.Description,
				Quantity: li.Quantity.String(),
				Unit:     li.Unit,
				Rate:     li.UnitRate.String(),
				Cost:     li.MonthlyCost.Round(2).String(),
				Source:   string(li.PriceSource),
			})
		}
		data.Costs = append(data.Costs, row)
	}

	var buf bytes.Buffer
	if err := htmlTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute HTML template: %w", err)
	}
	return buf.String(), nil
}

// shadeFor returns one of 5 CSS class buckets based on this row's
// fraction of the max. Buckets keep the output's colour palette
// predictable (low: green, high: red) without needing the user to
// supply an absolute scale.
func shadeFor(cost, ceiling decimal.Decimal) string {
	if ceiling.IsZero() {
		return "shade-0"
	}
	frac, _ := cost.Div(ceiling).Float64()
	switch {
	case frac >= 0.8:
		return "shade-4"
	case frac >= 0.6:
		return "shade-3"
	case frac >= 0.4:
		return "shade-2"
	case frac >= 0.2:
		return "shade-1"
	default:
		return "shade-0"
	}
}

type htmlData struct {
	Currency      string
	ProjectTotal  string
	ResourceCount int
	GeneratedAt   string
	Costs         []htmlCost
}

type htmlCost struct {
	Kind     string
	Name     string
	Subtotal string
	Shade    string
	IsZero   bool
	Items    []htmlLineItem
}

type htmlLineItem struct {
	Label    string
	Quantity string
	Unit     string
	Rate     string
	Cost     string
	Source   string
}

// htmlTmpl is the inline document. Kept inside the package so we
// don't carry around a runtime asset path; the trade-off is the
// template can't be hot-swapped without a rebuild, which is fine
// for the report format that doesn't change per user.
var htmlTmpl = template.Must(template.New("c3x").Parse(`<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><title>c3x cost estimate</title>
<style>
  :root { color-scheme: light dark; --fg:#1c1c1c; --bg:#fbfbfb; --muted:#666; --border:#e2e2e2;
          --shade-0:#e6f4ea; --shade-1:#fff4d6; --shade-2:#ffe7b0; --shade-3:#ffcfa3; --shade-4:#ffb3b3; }
  @media (prefers-color-scheme: dark) {
    :root { --fg:#eee; --bg:#171717; --muted:#999; --border:#333;
            --shade-0:#1f3326; --shade-1:#3a3622; --shade-2:#3a3022; --shade-3:#3a261d; --shade-4:#3a1e1e; }
  }
  body { font: 14px/1.5 -apple-system, system-ui, "Segoe UI", sans-serif; color:var(--fg); background:var(--bg); margin:0; padding:2rem; }
  h1 { margin:0 0 0.25rem; font-size:1.5rem; }
  .summary { color:var(--muted); margin-bottom:1.5rem; }
  .total { font-size:2rem; font-weight:600; color:var(--fg); }
  table { width:100%; border-collapse:collapse; }
  th, td { text-align:left; padding:6px 8px; border-bottom:1px solid var(--border); }
  th { font-size:0.85rem; color:var(--muted); font-weight:500; }
  td.cost { text-align:right; font-variant-numeric:tabular-nums; }
  details { margin:0; }
  summary { cursor:pointer; padding:8px; border-radius:4px; list-style:none; }
  summary::-webkit-details-marker { display:none; }
  summary::before { content:"▸ "; color:var(--muted); }
  details[open] summary::before { content:"▾ "; }
  .shade-0 { background:var(--shade-0); }
  .shade-1 { background:var(--shade-1); }
  .shade-2 { background:var(--shade-2); }
  .shade-3 { background:var(--shade-3); }
  .shade-4 { background:var(--shade-4); }
  .zero { opacity:0.5; }
  .breakdown { margin: 4px 8px 12px 28px; }
  .breakdown th, .breakdown td { padding:4px 8px; font-size:0.85rem; }
  .source { color:var(--muted); font-size:0.75rem; padding:1px 4px; border:1px solid var(--border); border-radius:3px; }
</style>
</head><body>

<h1>c3x cost estimate</h1>
<p class="summary">
  {{.ResourceCount}} resources
  {{if .GeneratedAt}} · generated {{.GeneratedAt}}{{end}}
</p>
<p class="total">{{.Currency}}{{.ProjectTotal}} / month</p>

<table>
<thead>
<tr><th>Resource</th><th></th><th>Kind</th><th class="cost">Monthly</th></tr>
</thead>
<tbody>
{{range .Costs}}
<tr class="{{.Shade}}{{if .IsZero}} zero{{end}}">
<td colspan="4">
<details>
<summary>{{.Kind}}.{{.Name}} — {{$.Currency}}{{.Subtotal}}</summary>
<table class="breakdown">
<thead><tr><th>Dimension</th><th>Quantity</th><th>Unit</th><th class="cost">Rate</th><th class="cost">Cost</th><th>Source</th></tr></thead>
<tbody>
{{range .Items}}
<tr>
  <td>{{.Label}}</td>
  <td>{{.Quantity}}</td>
  <td>{{.Unit}}</td>
  <td class="cost">{{$.Currency}}{{.Rate}}</td>
  <td class="cost">{{$.Currency}}{{.Cost}}</td>
  <td><span class="source">{{.Source}}</span></td>
</tr>
{{end}}
</tbody>
</table>
</details>
</td>
</tr>
{{end}}
</tbody>
</table>

</body></html>
`))
