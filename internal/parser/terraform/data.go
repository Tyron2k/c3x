package terraform

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"
)

// collectDataBlocks pre-scans every `data "kind" "name" { ... }` block
// and builds a `data` cty.Object whose shape mirrors Terraform's own
// scope, so `data.aws_ami.ubuntu.id` traversals resolve at evaluation
// time. Because c3x doesn't actually call AWS/Azure/GCP APIs to
// materialise data sources, every attribute resolves to a synthetic
// placeholder string (`"data.aws_ami.ubuntu.<attr>"`).
//
// The placeholder is generated lazily per traversal by overriding the
// object's element accessor with a map that returns a string for any
// key. We approximate this in cty by pre-populating `id` (the most
// commonly referenced attribute) plus any static literal attributes
// the data block declares in its body. Anything else still falls
// through to evaluator-level "unknown attribute" handling.
func collectDataBlocks(sources []sourceFile) cty.Value {
	byKind := map[string]map[string]cty.Value{}

	for _, src := range sources {
		for _, block := range src.Body.Blocks {
			if block.Type != "data" || len(block.Labels) < 2 {
				continue
			}
			kind := block.Labels[0]
			name := block.Labels[1]

			fields := map[string]cty.Value{
				"id": cty.StringVal(fmt.Sprintf("data.%s.%s.id", kind, name)),
			}
			ctx := buildEvalContext(cty.EmptyObjectVal, cty.EmptyObjectVal, cty.EmptyObjectVal, nil)
			for _, attr := range block.Body.Attributes {
				val, diags := attr.Expr.Value(ctx)
				if diags.HasErrors() {
					continue
				}
				fields[attr.Name] = val
			}

			if byKind[kind] == nil {
				byKind[kind] = map[string]cty.Value{}
			}
			byKind[kind][name] = cty.ObjectVal(fields)
		}
	}

	if len(byKind) == 0 {
		return cty.EmptyObjectVal
	}
	top := map[string]cty.Value{}
	for kind, names := range byKind {
		top[kind] = cty.ObjectVal(names)
	}
	return cty.ObjectVal(top)
}
