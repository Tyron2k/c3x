package render_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/render"
	"github.com/shopspring/decimal"
)

func sampleEstimate() domain.Estimate {
	return domain.NewEstimate([]domain.Cost{
		{
			Resource: domain.Reference{Kind: "aws_instance", Name: "web"},
			LineItems: []domain.LineItem{
				{
					Dimension: "compute_hours", Description: "Instance usage (m5.xlarge)",
					Unit: "hours", Quantity: dec("730"), UnitRate: dec("0.192"),
					MonthlyCost: dec("140.16"), PriceSource: domain.PriceSourceLive,
				},
			},
			MonthlySubtotal: dec("140.16"), Currency: domain.CurrencyUSD,
		},
		{
			Resource: domain.Reference{Kind: "aws_eip", Name: "main"},
			LineItems: []domain.LineItem{
				{
					Dimension: "ip_hours", Description: "Public IPv4 address-hour",
					Unit: "hours", Quantity: dec("730"), UnitRate: dec("0.005"),
					MonthlyCost: dec("3.65"), PriceSource: domain.PriceSourceStatic,
				},
			},
			MonthlySubtotal: dec("3.65"), Currency: domain.CurrencyUSD,
		},
	}, domain.CurrencyUSD, time.Unix(1700000000, 0).UTC())
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestParseFormat(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in      string
		want    render.Format
		wantErr bool
	}{
		{"", render.FormatText, false},
		{"text", render.FormatText, false},
		{"markdown", render.FormatMarkdown, false},
		{"md", render.FormatMarkdown, false},
		{"json", render.FormatJSON, false},
		{"yaml", 0, true},
	} {
		got, err := render.ParseFormat(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseFormat(%q) err=%v want=%v", tc.in, err, tc.wantErr)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("ParseFormat(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestRenderTextHighlightsStaticRates(t *testing.T) {
	t.Parallel()
	out := render.RenderText(sampleEstimate())
	if !strings.Contains(out, "PROJECT TOTAL") {
		t.Errorf("missing total line:\n%s", out)
	}
	if !strings.Contains(out, "$143.81/mo") {
		t.Errorf("expected total $143.81/mo, got:\n%s", out)
	}
	if !strings.Contains(out, "static") {
		t.Errorf("expected static-rate annotation:\n%s", out)
	}
}

func TestRenderMarkdownProducesGitHubFlavoredTables(t *testing.T) {
	t.Parallel()
	out := render.RenderMarkdown(sampleEstimate())
	// Sanity: starts with the heading and uses the markdown table syntax.
	if !strings.HasPrefix(out, "## c3x estimate") {
		t.Errorf("missing heading:\n%s", out)
	}
	if !strings.Contains(out, "|---|---:|---:|---:|---|") {
		t.Errorf("table delimiter missing:\n%s", out)
	}
	if !strings.Contains(out, "**Project total: $143.81/mo**") {
		t.Errorf("total line not in markdown:\n%s", out)
	}
}

func TestRenderJSONIsParseableAndPreservesPrecision(t *testing.T) {
	t.Parallel()
	raw, err := render.RenderJSON(sampleEstimate())
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if parsed["project_total"] != "143.81" {
		t.Errorf("project_total = %v, want \"143.81\"", parsed["project_total"])
	}
	if parsed["currency"] != "USD" {
		t.Errorf("currency = %v, want USD", parsed["currency"])
	}
	costs, _ := parsed["costs"].([]any)
	if len(costs) != 2 {
		t.Fatalf("expected 2 costs, got %d", len(costs))
	}
}

func TestRenderEmptyEstimate(t *testing.T) {
	t.Parallel()
	empty := domain.Estimate{Currency: domain.CurrencyUSD}
	if got := render.RenderText(empty); !strings.Contains(got, "no resources") {
		t.Errorf("empty text render:\n%s", got)
	}
	if got := render.RenderMarkdown(empty); !strings.Contains(got, "No resources") {
		t.Errorf("empty markdown render:\n%s", got)
	}
}

func TestDispatchRoutesToRightRenderer(t *testing.T) {
	t.Parallel()
	for _, f := range []render.Format{render.FormatText, render.FormatMarkdown, render.FormatJSON} {
		got, err := render.Render(sampleEstimate(), f)
		if err != nil {
			t.Fatalf("Render(%v): %v", f, err)
		}
		if got == "" {
			t.Errorf("Render(%v) produced empty output", f)
		}
	}
}

// TestEstimateJSONRoundTrip pins the on-disk baseline contract: any
// estimate we wrote yesterday must load identically today. Without
// this, `c3x diff` becomes fragile across c3x version bumps.
func TestEstimateJSONRoundTrip(t *testing.T) {
	t.Parallel()
	orig := sampleEstimate()
	raw, err := render.RenderJSON(orig)
	if err != nil {
		t.Fatal(err)
	}
	got, err := render.DecodeEstimate([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !got.ProjectTotal.Equal(orig.ProjectTotal) {
		t.Errorf("ProjectTotal: got %s want %s", got.ProjectTotal, orig.ProjectTotal)
	}
	if got.Currency != orig.Currency {
		t.Errorf("Currency: got %v want %v", got.Currency, orig.Currency)
	}
	if len(got.Costs) != len(orig.Costs) {
		t.Fatalf("Costs len: got %d want %d", len(got.Costs), len(orig.Costs))
	}
	for i := range got.Costs {
		if got.Costs[i].Resource != orig.Costs[i].Resource {
			t.Errorf("Costs[%d].Resource diverged", i)
		}
		if !got.Costs[i].MonthlySubtotal.Equal(orig.Costs[i].MonthlySubtotal) {
			t.Errorf("Costs[%d].MonthlySubtotal diverged", i)
		}
	}
}

func TestDiffRenderText(t *testing.T) {
	t.Parallel()
	d := domain.ComputeDiff(
		domain.Estimate{
			Costs: []domain.Cost{{
				Resource:        domain.Reference{Kind: "aws_instance", Name: "kept"},
				MonthlySubtotal: dec("10.00"),
			}, {
				Resource:        domain.Reference{Kind: "aws_instance", Name: "gone"},
				MonthlySubtotal: dec("5.00"),
			}},
			ProjectTotal: dec("15.00"),
			Currency:     domain.CurrencyUSD,
		},
		domain.Estimate{
			Costs: []domain.Cost{{
				Resource:        domain.Reference{Kind: "aws_instance", Name: "kept"},
				MonthlySubtotal: dec("12.00"),
			}, {
				Resource:        domain.Reference{Kind: "aws_instance", Name: "new"},
				MonthlySubtotal: dec("7.00"),
			}},
			ProjectTotal: dec("19.00"),
			Currency:     domain.CurrencyUSD,
		},
	)
	out := render.RenderTextDiff(d)
	if !strings.Contains(out, "+") || !strings.Contains(out, "-") || !strings.Contains(out, "~") {
		t.Errorf("expected +/-/~ markers:\n%s", out)
	}
	if !strings.Contains(out, "+$4") {
		t.Errorf("expected +$4 delta:\n%s", out)
	}
}
