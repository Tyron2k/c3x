package render

import (
	"fmt"

	"github.com/c3xdev/c3x/internal/domain"
)

// Render dispatches an Estimate to the right format-specific renderer.
// Callers that don't want to switch on Format themselves use this.
func Render(est domain.Estimate, f Format) (string, error) {
	switch f {
	case FormatText:
		return RenderText(est), nil
	case FormatMarkdown:
		return RenderMarkdown(est), nil
	case FormatJSON:
		return RenderJSON(est)
	case FormatJUnit:
		return RenderJUnit(est)
	case FormatHTML:
		return RenderHTML(est)
	case FormatCSV:
		return RenderCSV(est)
	case FormatSARIF:
		return RenderSARIF(est)
	default:
		return "", fmt.Errorf("unsupported format %v", f)
	}
}

// RenderDiff dispatches a Diff to the right format-specific renderer.
// Every format the estimate renderer supports works here too.
func RenderDiff(d domain.Diff, f Format) (string, error) {
	switch f {
	case FormatText:
		return RenderTextDiff(d), nil
	case FormatMarkdown:
		return RenderMarkdownDiff(d), nil
	case FormatJSON:
		return RenderJSONDiff(d)
	case FormatJUnit:
		return RenderJUnitDiff(d)
	case FormatHTML:
		return RenderHTMLDiff(d)
	case FormatCSV:
		return RenderCSVDiff(d)
	case FormatSARIF:
		return RenderSARIFDiff(d)
	default:
		return "", fmt.Errorf("unsupported format %v", f)
	}
}
