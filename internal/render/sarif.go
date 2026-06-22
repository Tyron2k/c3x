package render

// SARIF v2.1.0 renderer. SARIF (Static Analysis Results Interchange
// Format) is what GitHub code scanning, GitLab security dashboards,
// and many SCM-integrated security tools consume.
//
// We model c3x results as SARIF "results" with severity derived
// from cost magnitude:
//
//   - cost < $10/mo:       note     (info, no action)
//   - $10 ≤ cost < $100:   warning
//   - cost ≥ $100/mo:      error
//
// Severity buckets are not policy — they're a default scale that
// makes the SARIF surface useful at-a-glance. Users with stricter
// thresholds should feed the JSON output to OPA (when D lands) for
// real policy gating.

import (
	"encoding/json"
	"fmt"

	"github.com/c3xdev/c3x/internal/domain"
)

// RenderSARIF produces a v2.1.0 SARIF document with one run
// containing one result per resource.
func RenderSARIF(est domain.Estimate) (string, error) {
	doc := sarifDoc{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{
			{
				Tool: sarifTool{
					Driver: sarifDriver{
						Name:           "c3x",
						InformationURI: "https://c3x.dev",
						Rules: []sarifRule{
							{
								ID: "cost.note", Name: "LowMonthlyCost",
								ShortDescription: sarifMessage{Text: "Low monthly cost"},
							},
							{
								ID: "cost.warning", Name: "MediumMonthlyCost",
								ShortDescription: sarifMessage{Text: "Medium monthly cost"},
							},
							{
								ID: "cost.error", Name: "HighMonthlyCost",
								ShortDescription: sarifMessage{Text: "High monthly cost"},
							},
						},
					},
				},
				Results: make([]sarifResult, 0, len(est.Costs)),
			},
		},
	}
	currency := est.Currency.Symbol()
	for _, c := range est.Costs {
		monthly, _ := c.MonthlySubtotal.Float64()
		level, rule := sarifLevelFor(monthly)
		doc.Runs[0].Results = append(doc.Runs[0].Results, sarifResult{
			RuleID: rule,
			Level:  level,
			Message: sarifMessage{Text: fmt.Sprintf("%s.%s: %s%s/mo",
				c.Resource.Kind, c.Resource.Name, currency, c.MonthlySubtotal.Round(2))},
			Properties: map[string]any{
				"kind":         c.Resource.Kind,
				"name":         c.Resource.Name,
				"monthly_cost": c.MonthlySubtotal.Round(2).String(),
				"currency":     est.Currency.String(),
			},
		})
	}
	buf, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(buf) + "\n", nil
}

func sarifLevelFor(monthly float64) (string, string) {
	switch {
	case monthly >= 100:
		return "error", "cost.error"
	case monthly >= 10:
		return "warning", "cost.warning"
	default:
		return "note", "cost.note"
	}
}

// SARIF schema subset — only the fields we populate. The official
// schema is large; we ship the minimum that validates.

type sarifDoc struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	ShortDescription sarifMessage `json:"shortDescription"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID     string         `json:"ruleId"`
	Level      string         `json:"level"`
	Message    sarifMessage   `json:"message"`
	Properties map[string]any `json:"properties,omitempty"`
}
