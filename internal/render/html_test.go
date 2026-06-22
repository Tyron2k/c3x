package render_test

import (
	"strings"
	"testing"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/render"
	"github.com/shopspring/decimal"
)

func TestRenderHTMLProducesCompleteDocument(t *testing.T) {
	t.Parallel()
	est := domain.Estimate{
		Currency:     domain.CurrencyUSD,
		ProjectTotal: decimal.RequireFromString("140.16"),
		Costs: []domain.Cost{
			{
				Resource:        domain.Reference{Kind: "aws_instance", Name: "web"},
				MonthlySubtotal: decimal.RequireFromString("140.16"),
				LineItems: []domain.LineItem{
					{
						Description: "Instance hours", Quantity: decimal.RequireFromString("730"),
						UnitRate: decimal.RequireFromString("0.192"), MonthlyCost: decimal.RequireFromString("140.16"),
						Unit: "hours", PriceSource: "live",
					},
				},
			},
		},
	}
	got, err := render.RenderHTML(est)
	if err != nil {
		t.Fatal(err)
	}
	for _, must := range []string{
		"<!doctype html>",
		"<title>c3x cost estimate</title>",
		"$140.16",
		"aws_instance.web",
		"Instance hours",
		"</html>",
	} {
		if !strings.Contains(got, must) {
			t.Errorf("missing %q in output", must)
		}
	}
}

func TestRenderHTMLEscapesUserContent(t *testing.T) {
	t.Parallel()
	// Resource name with HTML special chars must be autoescaped.
	est := domain.Estimate{
		Currency:     domain.CurrencyUSD,
		ProjectTotal: decimal.Zero,
		Costs: []domain.Cost{
			{
				Resource:        domain.Reference{Kind: "aws_instance", Name: "<script>x</script>"},
				MonthlySubtotal: decimal.Zero,
			},
		},
	}
	got, err := render.RenderHTML(est)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "<script>x</script>") {
		t.Error("user content was not escaped — XSS surface")
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected escaped form: %s", got)
	}
}

func TestRenderHTMLAppliesCostShading(t *testing.T) {
	t.Parallel()
	est := domain.Estimate{
		Currency:     domain.CurrencyUSD,
		ProjectTotal: decimal.RequireFromString("100"),
		Costs: []domain.Cost{
			{
				Resource:        domain.Reference{Kind: "aws_instance", Name: "big"},
				MonthlySubtotal: decimal.RequireFromString("100"),
			}, // shade-4
			{
				Resource:        domain.Reference{Kind: "aws_instance", Name: "tiny"},
				MonthlySubtotal: decimal.RequireFromString("1"),
			}, // shade-0
		},
	}
	got, _ := render.RenderHTML(est)
	if !strings.Contains(got, "shade-4") || !strings.Contains(got, "shade-0") {
		t.Errorf("expected both shade-4 and shade-0 classes in output")
	}
}

func TestParseFormatAcceptsHTML(t *testing.T) {
	t.Parallel()
	f, err := render.ParseFormat("html")
	if err != nil {
		t.Fatal(err)
	}
	if f != render.FormatHTML {
		t.Errorf("got %v, want FormatHTML", f)
	}
}
