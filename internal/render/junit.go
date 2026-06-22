package render

// JUnit-XML renderer. The output schema follows the de-facto JUnit5
// shape that GitLab, CircleCI, Jenkins, BuildKite, and most other
// CI dashboards parse:
//
//	<testsuites name="c3x" tests="N" failures="F" time="0">
//	  <testsuite name="c3x.estimate" tests="N" failures="F">
//	    <testcase classname="aws_instance" name="web" time="0">
//	      <!-- nothing or <failure> -->
//	    </testcase>
//	    ...
//	  </testsuite>
//	</testsuites>
//
// Each resource is one <testcase>. The default classification is
// passing; if the per-resource subtotal exceeds a per-resource
// budget (carried separately in the Estimate.Metadata map, populated
// by `c3x diff --budget`), the case is marked <failure>.
//
// The renderer is deliberately conservative on what it surfaces —
// the JUnit format is heavy enough without including the full cost
// breakdown for every test case. The cost is in the <testcase>'s
// `time` attribute (rounded to 2dp dollars) so downstream tooling
// that graphs trends gets a single number per resource.

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/shopspring/decimal"
)

// RenderJUnit writes the Estimate as a JUnit-XML report. The third
// argument is an optional per-resource budget map (resource Ref →
// max-monthly-USD). A nil map means "no failures, just report".
func RenderJUnit(est domain.Estimate) (string, error) {
	return RenderJUnitWithBudget(est, nil)
}

// RenderJUnitWithBudget is the budget-aware variant. Resources
// exceeding their budget produce a <failure> element so CI gates can
// surface them.
func RenderJUnitWithBudget(est domain.Estimate, budget map[string]decimal.Decimal) (string, error) {
	suites := junitTestsuites{
		Name:  "c3x",
		Tests: len(est.Costs),
		Time:  "0",
		Suites: []junitTestsuite{
			{
				Name:       "c3x.estimate",
				Tests:      len(est.Costs),
				Time:       "0",
				Cases:      make([]junitTestcase, 0, len(est.Costs)),
				Properties: properties(est),
			},
		},
	}
	failures := 0
	for _, c := range est.Costs {
		tc := junitTestcase{
			Classname: c.Resource.Kind,
			Name:      c.Resource.Name,
			Time:      c.MonthlySubtotal.Round(2).String(),
		}
		if limit, ok := budget[refKey(c.Resource)]; ok && c.MonthlySubtotal.GreaterThan(limit) {
			tc.Failure = &junitFailure{
				Type:    "budget-exceeded",
				Message: fmt.Sprintf("monthly cost $%s exceeds budget $%s", c.MonthlySubtotal, limit),
			}
			failures++
		}
		suites.Suites[0].Cases = append(suites.Suites[0].Cases, tc)
	}
	suites.Failures = failures
	suites.Suites[0].Failures = failures

	out, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal JUnit XML: %w", err)
	}
	return xml.Header + string(out) + "\n", nil
}

// junitTestsuites is the root element. Wraps one or more <testsuite>
// blocks; we always emit exactly one.
type junitTestsuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Name     string           `xml:"name,attr"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Time     string           `xml:"time,attr"`
	Suites   []junitTestsuite `xml:"testsuite"`
}

type junitTestsuite struct {
	Name       string           `xml:"name,attr"`
	Tests      int              `xml:"tests,attr"`
	Failures   int              `xml:"failures,attr"`
	Time       string           `xml:"time,attr"`
	Properties *junitProperties `xml:"properties,omitempty"`
	Cases      []junitTestcase  `xml:"testcase"`
}

type junitTestcase struct {
	Classname string        `xml:"classname,attr"`
	Name      string        `xml:"name,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Type    string `xml:"type,attr"`
	Message string `xml:"message,attr"`
}

type junitProperties struct {
	Props []junitProperty `xml:"property"`
}

type junitProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// properties carries the run-level metadata in a JUnit-native shape.
// CI dashboards that show test details render properties verbatim,
// which is the cheapest place to surface the project total + the
// generation timestamp.
func properties(est domain.Estimate) *junitProperties {
	props := []junitProperty{
		{Name: "c3x.project_total_usd", Value: est.ProjectTotal.Round(2).String()},
		{Name: "c3x.currency", Value: est.Currency.String()},
		{Name: "c3x.resource_count", Value: fmt.Sprintf("%d", len(est.Costs))},
	}
	if !est.GeneratedAt.IsZero() {
		props = append(props, junitProperty{
			Name:  "c3x.generated_at",
			Value: est.GeneratedAt.UTC().Format(time.RFC3339),
		})
	}
	return &junitProperties{Props: props}
}

// refKey is the same key shape callers use to populate the budget
// map: "<kind>.<name>". We do not URL-encode because Terraform refs
// are ASCII by construction.
func refKey(r domain.Reference) string {
	var b strings.Builder
	b.WriteString(r.Kind)
	b.WriteByte('.')
	b.WriteString(r.Name)
	return b.String()
}
