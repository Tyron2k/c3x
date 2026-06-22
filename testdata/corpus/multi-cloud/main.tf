// Cross-cloud config: a CDN/origin pattern with AWS at the edge, an
// Azure Postgres backing service, and a GCP object store. Exercises
// the multi-cloud region detection and the per-provider default
// purchaseOption logic in the calculator.

variable "primary_region" { default = "us-east-1" }

provider "aws"     { region   = var.primary_region }
provider "azurerm" { location = "eastus" }
provider "google"  { region   = "us-central1" }

# CDN + origin at the edge.
resource "aws_cloudfront_distribution" "edge" {
  monthly_data_transfer_us_gb = 5000
  monthly_https_requests      = 10000000
}

resource "aws_s3_bucket" "origin" {
  bucket                  = "global-origin"
  standard_storage_gb     = 500
  monthly_tier_1_requests = 100000
  monthly_tier_2_requests = 1000000
}

# Azure backing database.
resource "azurerm_postgresql_flexible_server" "main" {
  name        = "main-db"
  sku_name    = "B_Standard_B1ms"
  storage_mb  = 65536
}

# GCP object store for cold backups.
resource "google_storage_bucket" "backups" {
  name               = "global-backups"
  storage_class      = "STANDARD"
  monthly_storage_gb = 2000
}
