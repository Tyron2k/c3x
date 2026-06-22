# STATIC vs LIVE vs FREE

Every resource c3x supports falls into one of three buckets. The
verifier (`go run ./cmd/verify_catalog`) reports each kind's
status; the `c3x supported-resources` subcommand exposes it to
users at runtime.

## LIVE

The resource's per-unit rate comes from the upstream pricing API
(`pricing.c3x.dev` by default). Rates track vendor changes
automatically — when AWS lowers the EC2 m5.xlarge price, c3x
picks it up at the next cache refresh (TTL is 7 days locally; the
backend re-scrapes daily).

LIVE is the preferred state. The verifier reports `[OK]` and
applies a ±5% snapshot tolerance.

## STATIC

The resource's rate is a numeric literal hard-coded in the TOML.
This happens when the upstream pricing catalog doesn't expose the
SKU c3x needs. Each STATIC entry is documented in
[`docs/upstream-gaps.md`](https://github.com/c3xdev/c3x/blob/main/docs/upstream-gaps.md)
with the exact backend probe that returned empty.

STATIC rates have a mandatory `last_verified` date. The verifier
reports `[STALE]` when the date is older than 6 months — a forcing
function for periodic re-validation against the vendor's published
pricing page.

The fix path for any STATIC entry is to enrich `c3x-pricing-api`
(the backend), not the c3x-go TOML. Once the backend exposes the
meter, the TOML flips from `rate = "0.05"` to
`rate = 'price("<mapping>")'`.

## FREE

The Terraform / CloudFormation kind has no per-resource AWS / Azure /
GCP charge. Three sub-cases:

1. **Truly free** — IAM roles, security groups, VPC plumbing, ECS
   task definitions. The provider doesn't bill at all.
2. **Parent-billed** — `aws_lb_target_group` (parent
   `aws_lb` carries the LCU fee), `azurerm_eventhub` (namespace is
   the priced unit). Cost shows on the parent.
3. **Data-plane** — `aws_internet_gateway` doesn't charge; the
   data transfer it enables bills via the originating resource.

The verifier reports `[FREE]` when an explicitly-marked free kind
estimates to $0. Bulk-added "shell" TOMLs (single `rate = "0"`
dimension) auto-classify as FREE via the
`Registry.IsFreeShell()` heuristic.

## Why three buckets, not two

The distinction between STATIC and FREE matters in one specific
case: a STATIC resource with `quantity = 0` could look like FREE
to a verifier that only checks "subtotal == 0". The verifier
specifically distinguishes "configured to bill but currently quiet"
from "structurally free" so a fixture change to a STATIC entry
doesn't silently degrade it to FREE.
