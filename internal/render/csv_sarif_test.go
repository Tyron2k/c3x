package render_test

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/render"
	"github.com/shopspring/decimal"
)

func sampleCSVSarifEstimate() domain.Estimate {
	return domain.Estimate{
		Currency:     domain.CurrencyUSD,
		ProjectTotal: decimal.RequireFromString("148.16"),
		Costs: []domain.Cost{
			{
				Resource:        domain.Reference{Kind: "aws_instance", Name: "web"},
				MonthlySubtotal: decimal.RequireFromString("140.16"),
				LineItems: []domain.LineItem{
					{
						Description: "Instance hours", Quantity: decimal.RequireFromString("730"),
						UnitRate:    decimal.RequireFromString("0.192"),
						MonthlyCost: decimal.RequireFromString("140.16"),
						Unit:        "hours", PriceSource: "live",
					},
				},
			},
			{
				Resource:        domain.Reference{Kind: "aws_ebs_volume", Name: "data"},
				MonthlySubtotal: decimal.RequireFromString("8.00"),
				LineItems: []domain.LineItem{
					{
						Description: "Storage", Quantity: decimal.RequireFromString("100"),
						UnitRate:    decimal.RequireFromString("0.08"),
						MonthlyCost: decimal.RequireFromString("8.00"),
						Unit:        "GB-month", PriceSource: "live",
					},
				},
			},
		},
	}
}

func TestRenderCSVParsesAsCSV(t *testing.T) {
	t.Parallel()
	got, err := render.RenderCSV(sampleCSVSarifEstimate())
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(strings.NewReader(got))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("re-parse CSV: %v\n%s", err, got)
	}
	// Header + 2 line items + trailing TOTAL row = 4 rows.
	if len(rows) != 4 {
		t.Errorf("row count = %d, want 4\n%s", len(rows), got)
	}
	if rows[0][0] != "resource" {
		t.Errorf("header[0] = %q, want resource", rows[0][0])
	}
	// TOTAL row at the tail.
	tail := rows[len(rows)-1]
	if tail[2] != "TOTAL" || tail[6] != "148.16" {
		t.Errorf("trailing TOTAL row wrong: %v", tail)
	}
}

func TestRenderCSVEmitsRowForResourceWithNoLineItems(t *testing.T) {
	t.Parallel()
	est := domain.Estimate{
		Currency:     domain.CurrencyUSD,
		ProjectTotal: decimal.Zero,
		Costs: []domain.Cost{{
			Resource:        domain.Reference{Kind: "aws_iam_role", Name: "free"},
			MonthlySubtotal: decimal.Zero,
		}},
	}
	got, _ := render.RenderCSV(est)
	r := csv.NewReader(strings.NewReader(got))
	rows, _ := r.ReadAll()
	if len(rows) < 3 {
		t.Errorf("expected header + free-row + TOTAL, got %d rows", len(rows))
	}
}

func TestRenderSARIFProducesValidDocument(t *testing.T) {
	t.Parallel()
	got, err := render.RenderSARIF(sampleCSVSarifEstimate())
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("re-parse SARIF JSON: %v\n%s", err, got)
	}
	if v, _ := doc["version"].(string); v != "2.1.0" {
		t.Errorf("version = %v, want 2.1.0", v)
	}
	runs, _ := doc["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	results, _ := runs[0].(map[string]any)["results"].([]any)
	if len(results) != 2 {
		t.Errorf("results = %d, want 2", len(results))
	}
}

func TestSARIFSeverityBucketing(t *testing.T) {
	t.Parallel()
	est := domain.Estimate{
		Currency:     domain.CurrencyUSD,
		ProjectTotal: decimal.RequireFromString("305"),
		Costs: []domain.Cost{
			{
				Resource:        domain.Reference{Kind: "k", Name: "expensive"},
				MonthlySubtotal: decimal.RequireFromString("200"),
			},
			{
				Resource:        domain.Reference{Kind: "k", Name: "medium"},
				MonthlySubtotal: decimal.RequireFromString("50"),
			},
			{
				Resource:        domain.Reference{Kind: "k", Name: "cheap"},
				MonthlySubtotal: decimal.RequireFromString("2"),
			},
		},
	}
	got, _ := render.RenderSARIF(est)
	wantContains := []string{`"level": "error"`, `"level": "warning"`, `"level": "note"`}
	for _, w := range wantContains {
		if !strings.Contains(got, w) {
			t.Errorf("expected %q in SARIF output", w)
		}
	}
}

func TestParseFormatAcceptsCSVAndSARIF(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"csv", "sarif"} {
		if _, err := render.ParseFormat(in); err != nil {
			t.Errorf("ParseFormat(%q): %v", in, err)
		}
	}
}
