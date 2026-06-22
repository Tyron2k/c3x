# GitHub Actions

A drop-in workflow that runs c3x on every PR and posts a cost
comment.

## Workflow

```yaml
name: c3x

on:
  pull_request:
    branches: [main]

permissions:
  contents: read
  pull-requests: write

jobs:
  estimate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: hashicorp/setup-terraform@v3
        with:
          terraform_version: 1.7.0

      - name: Terraform plan
        run: |
          terraform init
          terraform plan -out plan.tfplan
          terraform show -json plan.tfplan > plan.json

      - name: Install c3x
        run: |
          # When the release pipeline ships (ROADMAP item G) this
          # becomes a single `uses: c3xdev/c3x-action@v1`. Until
          # then, `go install` is the install path.
          go install github.com/c3xdev/c3x/cmd/c3x@latest
          echo "$(go env GOPATH)/bin" >> "$GITHUB_PATH"

      - name: Cost estimate
        run: c3x estimate --path plan.json --format markdown

      - name: PR comment
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: c3x comment github --path plan.json
```

## Gating merges on cost

Add a `--budget` to the estimate step to block merges:

```yaml
      - name: Cost gate
        run: c3x estimate --path plan.json --budget 1000
```

`c3x` exits 1 when the project total exceeds the threshold, which
fails the GitHub Actions job and blocks merge if "Require status
checks to pass" is enabled.

## Diff against `main`

For a "this PR adds $X/month" comment, save a baseline from main
in a separate workflow and compare:

```yaml
      - uses: actions/cache@v4
        with:
          path: base.json
          key: c3x-baseline-${{ github.base_ref }}

      - name: Diff
        run: |
          if [ -f base.json ]; then
            c3x diff --baseline base.json --path plan.json \
                     --budget-delta 50 --format markdown
          fi
```

## Auto-detect

When running inside `pull_request` events, `c3x comment github`
auto-detects everything from `GITHUB_REPOSITORY` and `GITHUB_REF`.
You only need to supply `GITHUB_TOKEN` (Actions sets it).
