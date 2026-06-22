# CLI command reference

Every subcommand at a glance.

## `c3x estimate`

Parses an IaC source, computes the monthly cost, prints a
per-resource breakdown.

```bash
c3x estimate --path .                     # Terraform directory
c3x estimate --path plan.json             # terraform show -json output
c3x estimate --path template.yaml         # CloudFormation
c3x estimate --path . --format json       # machine output
c3x estimate --path . --currency EUR      # FX-converted via Frankfurter
c3x estimate --path . --budget 1000       # CI gate: exit 1 if > $1000/mo
c3x estimate --path . --show-skipped      # list parsed-but-unpriced resources
c3x estimate --path . --save-baseline base.json
c3x estimate --path . --what-if 'aws_instance.web.instance_type=t3.micro'
```

## `c3x diff`

Compares the current estimate against a saved baseline.

```bash
c3x diff --baseline base.json --path .
c3x diff --baseline base.json --path . --budget-delta 50
```

## `c3x recommend`

Surfaces cost-optimisation alternatives, including cross-resource
(tree) rules.

```bash
c3x recommend --path .
```

## `c3x comment <forge>`

Posts the markdown estimate as a PR / MR comment. Marker-tagged
so re-runs edit the same comment in place.

```bash
c3x comment github --path .
c3x comment gitlab --path .
c3x comment bitbucket --path .
c3x comment azuredevops --path .       # alias: ado
```

## `c3x policy eval`

Runs Rego policies against the estimate, using a standard Rego policy
shape so common rules port.

```bash
c3x policy eval --policy ./policies --estimate est.json
c3x policy eval --policy ./policies --baseline base.json --path .
```

## `c3x supported-resources`

Lists every kind c3x knows about plus its pricing status.

```bash
c3x supported-resources --provider aws --status static
c3x supported-resources --format json | jq '.resources[] | select(.status == "LIVE")'
```

## `c3x doctor`

Pre-flight checks (catalog, endpoint, cache, config). Usable as a
CI gate before running anything that depends on network.

```bash
c3x doctor
c3x doctor --quiet         # only print failures
```

## `c3x configure`

First-run setup wizard. Writes user-level config at
`~/.config/c3x/config.toml`.

```bash
c3x configure
c3x configure --non-interactive    # write platform defaults
```

## Common flags

| Flag | Default | Meaning |
|---|---|---|
| `--path` | `.` | Terraform / CFN / plan-JSON input |
| `--format` | `text` | Output format (text, markdown, json, junit, html, csv, sarif) |
| `--currency` | `USD` | Display currency; non-USD converts via Frankfurter |
| `--region` | _(provider's default)_ | Default region when the IaC doesn't declare one |
| `--var`, `--var-file` | _(empty)_ | Terraform variable overrides |
| `--usage` | _(empty)_ | Path to a `c3x-usage.yml` |
| `--what-if` | _(empty)_ | `kind.name.attr=value` override (repeatable) |
| `--offline` | `false` | Skip the network; use the offline stub (most resources $0) |
| `--no-cache` | `false` | Bypass the on-disk price cache |
| `--cache-path` | _(XDG)_ | SQLite cache file location |
| `--pricing-endpoint` | `https://pricing.c3x.dev/graphql` | GraphQL backend |
| `--budget` | `0` | Fail when project total exceeds this monthly threshold |
| `--show-skipped` | `false` | List resources parsed but not priced, with reason |
