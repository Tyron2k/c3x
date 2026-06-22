provider "aws" {
  region = "us-east-1"
}

resource "aws_instance" "web" {
  ami           = "ami-0c55b159cbfafe1f0"
  instance_type = "t3.medium"

  root_block_device {
    volume_type = "gp3"
    volume_size = 50
  }
}

resource "aws_ebs_volume" "data" {
  availability_zone = "us-east-1a"
  type              = "gp3"
  size              = 200
}

resource "aws_db_instance" "postgres" {
  engine            = "postgres"
  instance_class    = "db.t3.medium"
  allocated_storage = 100
}

resource "aws_nat_gateway" "nat" {
  subnet_id = "subnet-12345"
}

resource "aws_eks_cluster" "k8s" {
  name     = "main"
  role_arn = "arn:aws:iam::123456789012:role/eks"
  vpc_config {
    subnet_ids = ["subnet-12345"]
  }
}

resource "aws_lb" "alb" {
  load_balancer_type = "application"
}

resource "aws_elasticache_cluster" "redis" {
  cluster_id      = "redis"
  engine          = "redis"
  node_type       = "cache.t3.medium"
  num_cache_nodes = 1
}

resource "aws_dynamodb_table" "tbl" {
  name           = "events"
  billing_mode   = "PROVISIONED"
  read_capacity  = 20
  write_capacity = 10
  hash_key       = "id"
  attribute {
    name = "id"
    type = "S"
  }
}
