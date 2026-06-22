// Monorepo pattern: top-level orchestration, two local modules each
// declaring its own variables and resources. Tests that local module
// expansion threads parent inputs and prefixes resource names.

variable "env" { default = "prod" }

provider "aws" { region = "us-east-1" }

module "database" {
  source         = "./modules/database"
  env            = var.env
  instance_class = "db.r6g.large"
  storage_gb     = 200
}

module "cache" {
  source    = "./modules/cache"
  env       = var.env
  node_type = "cache.r6g.large"
  nodes     = 3
}
