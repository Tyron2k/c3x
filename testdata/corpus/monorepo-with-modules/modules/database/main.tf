variable "env"            {}
variable "instance_class" {}
variable "storage_gb"     {}

resource "aws_db_instance" "primary" {
  identifier        = format("%s-db", var.env)
  instance_class    = var.instance_class
  engine            = "postgres"
  allocated_storage = var.storage_gb
  storage_type      = "gp3"
  multi_az          = false
}
