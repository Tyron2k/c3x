package render_test

// Tests for the diff-format matrix completion: JUnit, HTML, CSV,
// SARIF renderers over a domain.Diff. Shape assertions only — the
// styling is free to evolve, the data contract is not.

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/render"
	"github.com/shopspring/decimal"
)

func sampleDiff() domain.Diff {
	base := domain.NewEstimate([]domain.Cost{
		{
			Resource:        domain.Reference{Kind: "aws_instance", Name: "web"},
			MonthlySubtotal: decimal.RequireFromString("100"), Currency: domain.CurrencyUSD,
		},
		{
			Resource:        domain.Reference{Kind: "aws_s3_bucket", Name: "logs"},
			MonthlySubtotal: decimal.RequireFromString("5"), Currency: domain.CurrencyUSD,
		},
	}, domain.CurrencyUSD, time.Now())
	cur := domain.NewEstimate([]domain.Cost{
		{
			Resource:        domain.Reference{Kind: "aws_instance", Name: "web"},
			MonthlySubtotal: decimal.RequireFromString("250"), Currency: domain.CurrencyUSD,
		}, // modified +150
		{
			Resource:        domain.Reference{Kind: "aws_lambda_function", Name: "fn"},
			MonthlySubtotal: decimal.RequireFromString("3"), Currency: domain.CurrencyUSD,
		}, // added
	}, domain.CurrencyUSD, time.Now())
	return domain.ComputeDiff(base, cur)
}

func TestRenderJUnitDiffShape(t *testing.T) {
	t.Parallel()
	out, err := render.RenderJUnitDiff(sampleDiff())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		XMLName xml.Name `xml:"testsuites"`
		Suites  []struct {
			Name  string `xml:"name,attr"`
			Cases []struct {
				Classname string `xml:"classname,attr"`
				Name      string `xml:"name,attr"`
				Time      string `xml:"time,attr"`
			} `xml:"testcase"`
		} `xml:"testsuite"`
	}
	if err := xml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid XML: %v\n%s", err, out)
	}
	if len(doc.Suites) != 1 || len(doc.Suites[0].Cases) != 3 {
		t.Fatalf("expected 1 suite with 3 cases, got %+v", doc.Suites)
	}
	if !strings.Contains(out, "[modified]") || !strings.Contains(out, "[added]") || !strings.Contains(out, "[removed]") {
		t.Errorf("expected change kinds annotated in case names:\n%s", out)
	}
}

func TestRenderCSVDiffShape(t *testing.T) {
	t.Parallel()
	out, err := render.RenderCSVDiff(sampleDiff())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// header + 3 deltas + TOTAL
	if len(lines) != 5 {
		t.Fatalf("expected 5 CSV lines, got %d:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "resource,kind,change,baseline,current,delta,currency") {
		t.Errorf("header mismatch: %q", lines[0])
	}
	if !strings.HasPrefix(lines[len(lines)-1], "PROJECT,") {
		t.Errorf("expected trailing TOTAL row, got %q", lines[len(lines)-1])
	}
	if !strings.Contains(out, "aws_instance.web,aws_instance,modified,100,250,150,USD") {
		t.Errorf("modified row missing or malformed:\n%s", out)
	}
}

func TestRenderSARIFDiffSeverities(t *testing.T) {
	t.Parallel()
	out, err := render.RenderSARIFDiff(sampleDiff())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version string `json:"version"`
		Runs    []struct {
			Results []struct {
				RuleID string `json:"ruleId"`
				Level  string `json:"level"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if doc.Version != "2.1.0" || len(doc.Runs) != 1 {
		t.Fatalf("unexpected SARIF envelope: %+v", doc)
	}
	byLevel := map[string]int{}
	for _, r := range doc.Runs[0].Results {
		byLevel[r.Level]++
	}
	// +150 instance → error; +3 lambda → warning; -5 removed bucket → note.
	if byLevel["error"] != 1 || byLevel["warning"] != 1 || byLevel["note"] != 1 {
		t.Errorf("severity distribution = %v, want error:1 warning:1 note:1", byLevel)
	}
}

func TestRenderHTMLDiffEscapesAndRenders(t *testing.T) {
	t.Parallel()
	d := sampleDiff()
	// Hostile resource name must be HTML-escaped by the template.
	d.Resources[0].Resource.Name = `<script>alert(1)</script>`
	out, err := render.RenderHTMLDiff(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Error("resource name was not HTML-escaped")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("expected escaped form of the hostile name")
	}
	if !strings.Contains(out, "c3x cost diff") {
		t.Error("missing report title")
	}
}

func TestRenderDiffDispatchAllFormats(t *testing.T) {
	t.Parallel()
	d := sampleDiff()
	for _, f := range []render.Format{
		render.FormatText, render.FormatMarkdown, render.FormatJSON,
		render.FormatJUnit, render.FormatHTML, render.FormatCSV, render.FormatSARIF,
	} {
		out, err := render.RenderDiff(d, f)
		if err != nil {
			t.Errorf("RenderDiff(%v): %v", f, err)
			continue
		}
		if out == "" {
			t.Errorf("RenderDiff(%v) produced empty output", f)
		}
	}
}
