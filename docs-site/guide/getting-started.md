# Getting started

c3x is a single Go binary. Install, point it at a Terraform
directory, get a per-resource cost breakdown.

## Install

::: code-group

```bash [go install]
go install github.com/c3xdev/c3x/cmd/c3x@latest
```

```bash [binary release]
# Once release tooling is wired (ROADMAP item G), pre-built
# binaries will live at https://github.com/c3xdev/c3x/releases.
# Until then, `go install` is the canonical install path.
```

```bash [Homebrew]
# Pending the release tooling (ROADMAP G.3):
brew install c3xdev/c3x/c3x
```

```bash [Docker]
docker pull ghcr.io/c3xdev/c3x:latest
```

:::

## Verify the install

```bash
c3x --version
c3x doctor
```

`c3x doctor` runs four checks (catalog loads, pricing endpoint
reachable, cache writable, config resolves) and exits non-zero if
any fail.

## Your first estimate

Point c3x at any directory containing `.tf` files:

```bash
c3x estimate --path ./my-terraform
```

Example output (truncated):

```
$  c3x estimate --path testdata/corpus/eks-cluster

c3x estimate
============

📦 aws_eks_cluster.main                                  $73.00/mo
   Control plane (EKS classic)  730 hours × $0.10 = $73.00 [static]

📦 aws_instance.worker[0]                                $140.16/mo
   Instance usage (Linux, m5.xlarge, on-demand)
                                730 hours × $0.192 = $140.16 [live]
   ...

Project total: $487.16/mo
```

Each line item shows quantity × unit rate = monthly cost, plus a
source tag — `[live]` came from the upstream pricing API, `[static]`
came from an inline TOML literal (the upstream doesn't expose that
SKU).

## Saving a baseline

The estimate JSON form is the input to `c3x diff`:

```bash
c3x estimate --path . --save-baseline base.json
```

After changes, compare against the baseline:

```bash
c3x diff --baseline base.json --path .
```

This is the CI gate flow — combine with `--budget-delta 50` to
block PRs that add more than $50/month.

## Next steps

- [Your first estimate](./first-estimate) — walk through the
  output format and the `--what-if` overrides.
- [Usage file](./usage-file) — supply runtime quantities the
  parser can't infer (request counts, log volumes).
- [CI integration](/integrations/) — pick the right forge guide.
