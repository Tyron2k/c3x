package terraform_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/c3xdev/c3x/internal/parser/terraform"
)

// TestRegistryModuleResolvesViaInitManifest exercises the
// `.terraform/modules/modules.json` integration: with the manifest in
// place, a registry-style source string is mapped to a local directory
// the parser can descend into.
func TestRegistryModuleResolvesViaInitManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registryDir := filepath.Join(dir, ".terraform", "modules", "vpc")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, registryDir, "main.tf", `
		variable "name" {}
		resource "aws_instance" "edge" {
		  instance_type = "t3.micro"
		  ami           = "ami-${var.name}"
		}
	`)
	manifest := map[string]any{
		"Modules": []map[string]any{
			{"Key": "", "Source": "", "Dir": "."},
			{"Key": "vpc", "Source": "terraform-aws-modules/vpc/aws", "Version": "5.0.0", "Dir": ".terraform/modules/vpc"},
		},
	}
	raw, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, ".terraform", "modules", "modules.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	write(t, dir, "main.tf", `
		provider "aws" { region = "us-east-1" }
		module "vpc" {
		  source  = "terraform-aws-modules/vpc/aws"
		  version = "5.0.0"
		  name    = "main"
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(got))
	}
	if got[0].Ref.Name != "module.vpc.edge" {
		t.Errorf("name = %q, want module.vpc.edge", got[0].Ref.Name)
	}
	if got[0].Attributes["ami"] != "ami-main" {
		t.Errorf("ami = %v, want ami-main", got[0].Attributes["ami"])
	}
}
