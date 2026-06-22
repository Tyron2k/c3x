package terraform

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
)

// formatDiags renders an [hcl.Diagnostics] slice as a single string
// with the source position prefix on each line — `main.tf:14,5-12:
// Argument or block definition required`.
//
// Errors before this helper existed printed `diags.Error()` directly,
// which collapses the position into a generic prefix and loses the
// most useful detail Terraform users want when c3x rejects their
// config. This single seam fixes every parser callsite at once.
func formatDiags(diags hcl.Diagnostics) string {
	if !diags.HasErrors() {
		return diags.Error()
	}
	parts := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.Severity != hcl.DiagError {
			continue
		}
		parts = append(parts, formatDiag(d))
	}
	if len(parts) == 0 {
		return diags.Error()
	}
	return strings.Join(parts, "; ")
}

// formatDiag renders one diagnostic. The subject (where the error
// happened) is preferred over the context (the enclosing expression)
// because users care about the precise spot, not the surrounding
// construct.
func formatDiag(d *hcl.Diagnostic) string {
	rng := d.Subject
	if rng == nil {
		rng = d.Context
	}
	var pos string
	if rng != nil {
		pos = fmt.Sprintf("%s:%d,%d", rng.Filename, rng.Start.Line, rng.Start.Column)
		if rng.End.Line == rng.Start.Line && rng.End.Column > rng.Start.Column {
			pos = fmt.Sprintf("%s-%d", pos, rng.End.Column)
		}
	}
	msg := d.Summary
	if d.Detail != "" {
		msg = msg + ": " + d.Detail
	}
	if pos == "" {
		return msg
	}
	return pos + ": " + msg
}
