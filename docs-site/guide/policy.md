# Policy gates

c3x runs Rego policies against estimates and diffs, using a simple,
portable JSON data shape.

## Quick start

```bash
# Write a policy: deny when monthly total > $1000.
cat > budget.rego <<'EOF'
package c3x

deny[msg] {
  input.estimate.project_total > 1000
  msg := sprintf("project total $%v/mo exceeds $1000 budget",
                 [input.estimate.project_total])
}
EOF

# Evaluate it.
c3x estimate --path . --save-baseline est.json
c3x policy eval --policy budget.rego --estimate est.json
```

Exit code is 1 on any `deny[...]` match. Warnings (`warn[...]`)
print to stderr but don't fail.

## Data model

```jsonc
{
  "estimate": {
    "project_total":  148.16,
    "currency":       "USD",
    "generated_at":   "2026-06-09T10:00:00Z",
    "resources": [
      { "kind": "aws_instance",     "name": "web",  "monthly_cost": 140.16 },
      { "kind": "aws_ebs_volume",   "name": "data", "monthly_cost": 8.00 }
    ]
  },
  "diff": {                              // present only with --baseline
    "baseline_total": 50.00,
    "current_total":  148.16,
    "total_delta":    98.16,
    "resources": [
      { "kind": "aws_instance", "name": "web",
        "baseline": 0.0, "current": 140.16, "delta": 140.16 }
    ]
  }
}
```

Policies write rules under `package c3x`. Two outputs are
collected:

- `deny[msg]` — non-empty list fails the run.
- `warn[msg]` — printed to stderr; does not fail.

## Sample policies

The repo ships three runnable examples under `examples/policies/`:

```bash
# Block runs over an absolute budget.
c3x policy eval --policy examples/policies/budget.rego \
                --estimate est.json

# Warn on non-prod resources > $100/mo.
c3x policy eval --policy examples/policies/no_expensive_dev.rego \
                --estimate est.json

# Block PRs that add > $50/mo of cost.
c3x policy eval --policy examples/policies/diff_gate.rego \
                --baseline base.json --path .
```

## Composing with `--budget`

`c3x estimate --budget X` is a one-flag absolute gate. `c3x policy
eval` is the right tool when:

- Multiple thresholds apply (prod vs non-prod, per-team budgets).
- The rule is shape-dependent (no GPU on weekends, EBS gp3 only).
- The team already has a Rego ecosystem (Conftest, OPA in
  Kubernetes admission, etc.).
