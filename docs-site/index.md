---
layout: home

hero:
  name: c3x
  text: Cloud cost estimation for Terraform & CloudFormation.
  tagline: Open source, no API key, no SaaS. Price every resource before you apply.
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: View on GitHub
      link: https://github.com/c3xdev/c3x

features:
  - icon: 🚫
    title: No API key, no account
    details: Default pricing endpoint is the public c3x-pricing-api. Self-host it with a single docker-compose if you prefer.
  - icon: 🔍
    title: Free CloudFormation + recommendations
    details: Both are paid features in other tools. c3x ships them in the same binary as the estimate engine.
  - icon: 📦
    title: 325+ resources across AWS / Azure / GCP
    details: Declarative TOML catalog. Adding a new resource is one file, not a Go rewrite.
  - icon: ⚡
    title: Fast
    details: 1.3 ms to load the full catalog, ~5 µs per resource estimated. SQLite price cache survives offline.
---

## At a glance

```bash
# Estimate the monthly cost of a Terraform project
c3x estimate --path .

# Compare against a saved baseline (CI gate friendly)
c3x diff --baseline base.json --path .

# Post the breakdown as a PR comment
c3x comment github --path .

# Surface cheaper alternatives
c3x recommend --path .

# Run a Rego policy
c3x policy eval --policy ./policies --path .
```

No SaaS account, no API key, no telemetry. Works against the public
`pricing.c3x.dev` endpoint by default, or your own self-hosted
`c3x-pricing-api` for fully air-gapped use.

## What you get

- **No API key**, no SaaS account, no telemetry.
- **CloudFormation** support alongside Terraform and Terragrunt.
- **Free recommendations** including cross-resource (tree) rules
  like NAT-consolidation with net-savings math.
- **Self-host without credentials** — the c3x scraper pulls directly
  from AWS Bulk Pricing / Azure Retail Prices / GCP Cloud Billing.
- Output formats: text, markdown, JSON, JUnit, HTML, CSV, SARIF.
- 5 forge integrations (GitHub, GitLab, Bitbucket, Azure DevOps,
  Atlantis), all free.

## Project status

This is **early software**. The catalog has 81 LIVE-priced resources
verified daily against the upstream backend, 61 STATIC entries with
hand-curated rates, and 183 free / structural resources. See
[the catalog reference](/reference/catalog) for the current matrix.
