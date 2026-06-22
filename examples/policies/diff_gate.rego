# Diff gate: deny when the PR adds more than $50/mo of cost.
#
# Requires --baseline:
#   c3x policy eval --policy examples/policies/diff_gate.rego \
#                   --baseline base.json --path .

package c3x

deny[msg] {
    input.diff.total_delta > 50
    msg := sprintf("PR adds %v/mo (baseline %v → current %v); ask for review before merging",
                   [input.diff.total_delta, input.diff.baseline_total, input.diff.current_total])
}

warn[msg] {
    resource := input.diff.resources[_]
    resource.delta > 25
    msg := sprintf("resource %v.%v adds %v/mo on this PR",
                   [resource.kind, resource.name, resource.delta])
}
