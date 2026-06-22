# Budget policy: deny when the project total exceeds $1000/mo.
#
# Run:
#   c3x estimate --path . --save-baseline est.json
#   c3x policy eval --policy examples/policies/budget.rego --estimate est.json

package c3x

deny[msg] {
    input.estimate.project_total > 1000
    msg := sprintf("project total %v %v/mo exceeds budget $1000/mo",
                   [input.estimate.currency, input.estimate.project_total])
}
