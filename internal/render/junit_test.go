package render_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/render"
	"github.com/shopspring/decimal"
)

func TestRenderJUnitProducesValidXML(t *testing.T) {
	t.Parallel()

	est := domain.Estimate{
		Currency:     domain.CurrencyUSD,
		ProjectTotal: decimal.RequireFromString("140.16"),
		Costs: []domain.Cost{
			{
				Resource:        domain.Reference{Kind: "aws_instance", Name: "web"},
				MonthlySubtotal: decimal.RequireFromString("140.16"),
			},
			{
				Resource:        domain.Reference{Kind: "aws_ebs_volume", Name: "data"},
				MonthlySubtotal: decimal.RequireFromString("8.00"),
			},
		},
	}
	got, err := render.RenderJUnit(est)
	if err != nil {
		t.Fatal(err)
	}
	// Must round-trip through xml.Unmarshal to be valid.
	var doc struct {
		XMLName  xml.Name `xml:"testsuites"`
		Name     string   `xml:"name,attr"`
		Tests    int      `xml:"tests,attr"`
		Failures int      `xml:"failures,attr"`
	}
	if err := xml.Unmarshal([]byte(strings.TrimPrefix(got, xml.Header)), &doc); err != nil {
		t.Fatalf("unmarshal: %v\nXML:\n%s", err, got)
	}
	if doc.Name != "c3x" {
		t.Errorf("Name = %q, want c3x", doc.Name)
	}
	if doc.Tests != 2 {
		t.Errorf("Tests = %d, want 2", doc.Tests)
	}
	if doc.Failures != 0 {
		t.Errorf("Failures = %d, want 0 (no budget set)", doc.Failures)
	}
	if !strings.Contains(got, `classname="aws_instance" name="web"`) {
		t.Errorf("missing expected testcase line:\n%s", got)
	}
	if !strings.Contains(got, `c3x.project_total_usd`) {
		t.Errorf("missing project total property:\n%s", got)
	}
}

func TestRenderJUnitMarksBudgetExceededAsFailure(t *testing.T) {
	t.Parallel()

	est := domain.Estimate{
		Currency:     domain.CurrencyUSD,
		ProjectTotal: decimal.RequireFromString("200.00"),
		Costs: []domain.Cost{
			{
				Resource:        domain.Reference{Kind: "aws_instance", Name: "expensive"},
				MonthlySubtotal: decimal.RequireFromString("200.00"),
			},
			{
				Resource:        domain.Reference{Kind: "aws_ebs_volume", Name: "ok"},
				MonthlySubtotal: decimal.RequireFromString("8.00"),
			},
		},
	}
	budgets := map[string]decimal.Decimal{
		"aws_instance.expensive": decimal.RequireFromString("50.00"),
		"aws_ebs_volume.ok":      decimal.RequireFromString("50.00"),
	}
	got, err := render.RenderJUnitWithBudget(est, budgets)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `<failure type="budget-exceeded"`) {
		t.Errorf("expected <failure> on over-budget resource:\n%s", got)
	}
	if !strings.Contains(got, `failures="1"`) {
		t.Errorf("expected failures=1 at the suite level:\n%s", got)
	}
}

func TestParseFormatAcceptsJUnit(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"junit", "junit-xml", "JUnit"} {
		f, err := render.ParseFormat(in)
		if err != nil {
			t.Errorf("ParseFormat(%q) errored: %v", in, err)
			continue
		}
		if f != render.FormatJUnit {
			t.Errorf("ParseFormat(%q) = %v, want FormatJUnit", in, f)
		}
	}
}

func TestDispatchRoutesJUnit(t *testing.T) {
	t.Parallel()
	est := domain.Estimate{
		Currency:     domain.CurrencyUSD,
		ProjectTotal: decimal.Zero,
		Costs: []domain.Cost{{
			Resource:        domain.Reference{Kind: "aws_iam_role", Name: "x"},
			MonthlySubtotal: decimal.Zero,
		}},
	}
	got, err := render.Render(est, render.FormatJUnit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, xml.Header) {
		t.Errorf("expected XML header prefix")
	}
}
