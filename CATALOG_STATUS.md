# Catalog verification status

Run `go run ./cmd/verify_catalog` against the live API to regenerate.
The harness builds a representative resource per kind, runs the
engine, and reports per-resource pricing health.

**Current state on `pricing.c3x.dev`:**
**122 live / 39 static / 181 free / 0 zero / 0 errored — 342 total.**

Catalog coverage spans 342 kinds across AWS, Azure, and GCP.
Breakdown:
- **122 LIVE**: priced against the upstream API; tracks vendor price changes.
- **39 STATIC**: priced inline because the upstream catalog doesn't
  expose the meter. Each documented in `docs/upstream-gaps.md` and
  carrying a `last_verified` date the verifier enforces.
- **182 FREE**: structural / parent-billed / metadata Terraform kinds
  that AWS/Azure/GCP don't charge for at the resource level (IAM,
  VPC plumbing, security groups, ECS task definitions, etc.).

The per-kind matrix is auto-generated at `docs/catalog.md`
(`go run ./cmd/gen_catalog_doc`). The hand-written sections below
cover the engine features and conversion workflow; per-kind status
lives in the generated page, not here.

## Output legend

- `[OK]` — produced a non-zero estimate against the live API
- `[STATIC]` — produced a non-zero estimate via an inline TOML literal
  (the catalog won't track upstream price changes here)
- `[FREE]` — known-free resource (parent-rolled or always free)
- `[ZERO]` — no priced line items (regression sentinel; fails CI)
- `[ERR]` — engine returned an error

## Static-rate resources

45 kinds carry inline rates because the upstream catalogue doesn't
expose their meter. The authoritative per-kind list with the exact
GraphQL probes that returned empty lives in `docs/upstream-gaps.md`;
the generated `docs/catalog.md` shows current status per kind. Every
STATIC fixture carries a `last_verified` date the verifier enforces
([STALE] after 6 months).

## Live-priced resources (122)

See `docs/catalog.md` for the full per-kind matrix. Notable shapes
covered:

- AWS: EC2 / RDS + Aurora / EBS + snapshots / S3 / Lambda /
  DynamoDB / Fargate (ECS service) / CloudFront / Route53 / API
  Gateway / Kinesis (+Firehose, +Managed Flink) / KMS / CloudHSM /
  MSK (+Serverless) / Glue (+crawler) / CodeBuild / ECR / Transit
  Gateway / VPC Endpoint + VPN + Direct Connect / NAT Gateway /
  Network Firewall / Transfer Family / Directory Service / Managed
  Grafana / Config / Glacier / OpenSearch / ElastiCache
  (+Serverless) / MemoryDB-adjacent / Redshift / Neptune /
  DocumentDB / Step Functions / SNS / SQS / MQ / Secrets Manager /
  CloudWatch (logs, alarms) / WAFv2 / ELB / Lightsail / FSx (all
  four flavors) / MWAA / DMS / Backup / CloudTrail / EKS / Dedicated
  Hosts
- Azure: VM (Linux + Windows) / Managed Disk / Storage Account /
  SQL Database + MySQL Flexible / Application Gateway / Container
  Instances / Function App / AKS / Log Analytics / PostgreSQL
  Flexible / Redis Cache / Service Plan / VPN Gateway / API
  Management / Synapse Serverless / AI Search / Databricks / NAT
  Gateway / Bastion / Traffic Manager / Event Grid / SignalR
- GCP: Compute / Cloud SQL / GKE / Cloud Run / Cloud Functions /
  Pub/Sub / Cloud Storage / Compute disks + addresses / Cloud DNS

## Legitimately free

182 kinds — IAM, VPC plumbing, security groups, listener/wiring
resources, parent-billed children. The full list is in
`docs/catalog.md`; the curated explanations live in
`internal/catalog/registry.go` (`LegitimatelyFreeKinds`).

## How to convert a STATIC entry to live

```bash
# 1. Probe what attributes the upstream exposes for the service:
curl -sS -X POST https://pricing.c3x.dev/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"{products(filter:{vendorName:\"<aws|azure|gcp>\",service:\"<service>\"},limit:5){productFamily attributes{key value} prices{USD purchaseOption unit}}}"}' | jq

# 2. Find a unique (service, productFamily, description/meterName/skuName) tuple.

# 3. Update the resource TOML to use a [mappings.X] block + price("X"):
#    service        = "..."
#    product_family = "..."
#    region         = "global"  (if the upstream entry has no regionCode)
#    purchase_option = "any"     (if the entries don't carry a purchaseOption label)
#    attribute_filters = [{ key = "description", const = "..." }]

# 4. Re-run the verifier:
go run ./cmd/verify_catalog | grep <kind>
```

## Engine-level features supporting the catalog

- **Max non-zero pricing.** Tiered services (CloudFront, S3 requests)
  return multiple rates per product; the HTTP source picks the
  largest non-zero so c3x quotes the conservative first-tier
  on-demand rate, not the cheapest committed tier.
- **`purchase_option = "any"` sentinel.** When the upstream's price
  entries don't carry a purchaseOption label, mappings can opt out
  of the filter so the query still returns rows.
- **Global-region override.** When a product is billed without a
  regionCode (CloudFront, Pub/Sub Message Delivery, GKE control
  plane), mappings set `region = "global"` to omit the region clause.
- **Three-layer cache.** Memo (in-process) → SQLite (7-day TTL) →
  HTTP. Most runs against a populated cache complete in <50 ms.
