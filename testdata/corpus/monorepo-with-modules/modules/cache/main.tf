variable "env"       {}
variable "node_type" {}
variable "nodes"     {}

resource "aws_elasticache_cluster" "main" {
  cluster_id      = format("%s-cache", var.env)
  node_type       = var.node_type
  engine          = "redis"
  num_cache_nodes = var.nodes
}
