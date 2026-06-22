// Realistic small VPC stack: NAT gateways behind an ALB, a couple of
// services, a managed database. Patterns drawn from common
// terraform-aws-modules examples. Used by the integration test as a
// proxy for "real Terraform users would write code like this."

variable "region"     { default = "us-east-1" }
variable "env"        { default = "prod" }
variable "azs"        { default = ["a", "b", "c"] }
variable "instance_t" { default = "m5.large" }

locals {
  prefix = format("%s-vpc", var.env)
  tags   = { Environment = var.env, ManagedBy = "terraform" }
}

provider "aws" { region = var.region }

resource "aws_eip" "nat" {
  count = length(var.azs)
  tags  = merge(local.tags, { Name = format("%s-nat-%s", local.prefix, var.azs[count.index]) })
}

resource "aws_nat_gateway" "main" {
  count                       = length(var.azs)
  allocation_id               = "eipalloc-${count.index}"
  monthly_data_processed_gb   = 200
  tags                        = local.tags
}

resource "aws_lb" "edge" {
  name               = format("%s-edge", local.prefix)
  load_balancer_type = "application"
  monthly_lcu_hours  = 1000
  tags               = local.tags
}

resource "aws_db_instance" "primary" {
  identifier        = format("%s-db", local.prefix)
  instance_class    = "db.t3.medium"
  engine            = "postgres"
  allocated_storage = 100
  storage_type      = "gp3"
  multi_az          = true
  tags              = local.tags
}

resource "aws_instance" "api" {
  for_each      = toset(["api", "worker", "scheduler"])
  instance_type = var.instance_t
  ami           = "ami-${each.key}"
  tags          = merge(local.tags, { Role = each.key })
}
