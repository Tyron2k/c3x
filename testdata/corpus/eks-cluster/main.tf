// EKS-style configuration: control plane + node group + an external
// load balancer + S3 buckets for state/artifacts. Mirrors the shape
// of terraform-aws-modules/eks examples.

variable "region"        { default = "us-east-1" }
variable "cluster_name"  { default = "platform" }
variable "node_count"    { default = 4 }
variable "node_type"     { default = "m5.xlarge" }

provider "aws" { region = var.region }

resource "aws_eks_cluster" "main" {}

resource "aws_instance" "worker" {
  count         = var.node_count
  instance_type = var.node_type
  ami           = "ami-worker-${count.index}"
  root_block_device {
    volume_size = 100
    volume_type = "gp3"
  }
}

resource "aws_lb" "ingress" {
  name               = format("%s-ingress", var.cluster_name)
  load_balancer_type = "application"
  monthly_lcu_hours  = 500
}

resource "aws_s3_bucket" "artifacts" {
  bucket                  = format("%s-artifacts", var.cluster_name)
  standard_storage_gb     = 200
  monthly_tier_1_requests = 50000
  monthly_tier_2_requests = 500000
}

resource "aws_s3_bucket" "state" {
  bucket                  = format("%s-state", var.cluster_name)
  standard_storage_gb     = 10
  monthly_tier_1_requests = 1000
  monthly_tier_2_requests = 10000
}

resource "aws_cloudwatch_log_group" "cluster" {
  name                     = format("/aws/eks/%s/cluster", var.cluster_name)
  monthly_data_ingested_gb = 30
  monthly_data_stored_gb   = 60
  monthly_data_scanned_gb  = 5
}
