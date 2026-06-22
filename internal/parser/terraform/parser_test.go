package terraform_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/c3xdev/c3x/internal/parser/terraform"
)

// write is a tiny helper that creates a file under `dir`. Saves a
// dozen lines of os.WriteFile boilerplate across the test file.
func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestParsesSimpleAwsInstance is the canonical smoke test: one
// resource block with literal attributes, no vars, no modules.
func TestParsesSimpleAwsInstance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "web" {
		  instance_type = "m5.xlarge"
		  ami           = "ami-123"
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(got))
	}
	if got[0].Ref.Kind != "aws_instance" || got[0].Ref.Name != "web" {
		t.Errorf("ref = %v, want aws_instance.web", got[0].Ref)
	}
	if got[0].Attributes["instance_type"] != "m5.xlarge" {
		t.Errorf("instance_type = %v, want m5.xlarge", got[0].Attributes["instance_type"])
	}
	if got[0].Region == nil || *got[0].Region != "us-east-1" {
		t.Errorf("region = %v, want us-east-1", got[0].Region)
	}
}

// TestResolvesVariableReferencesInAttributes proves the var.x scope
// flows through to attribute evaluation.
func TestResolvesVariableReferencesInAttributes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		variable "instance_type" { default = "m5.large" }
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "web" {
		  instance_type = var.instance_type
		  ami           = "ami-123"
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Attributes["instance_type"] != "m5.large" {
		t.Errorf("instance_type = %v, want m5.large", got[0].Attributes["instance_type"])
	}
}

// TestResolvesLocalsWithChainedReferences exercises the fixed-point
// loop: local.tier depends on local.prefix which depends on var.env.
func TestResolvesLocalsWithChainedReferences(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		variable "env" { default = "prod" }
		locals {
		  prefix = "${var.env}-app"
		  tier   = "${local.prefix}-db"
		}
		provider "aws" { region = "us-east-1" }
		resource "aws_db_instance" "main" {
		  identifier     = local.tier
		  instance_class = "db.t3.medium"
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Attributes["identifier"] != "prod-app-db" {
		t.Errorf("identifier = %v, want prod-app-db", got[0].Attributes["identifier"])
	}
}

func TestCountExpandsToNResources(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		variable "fleet" { default = 4 }
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "node" {
		  count         = var.fleet
		  instance_type = "t3.small"
		  ami           = "ami-x"
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 resources, got %d", len(got))
	}
	for i, r := range got {
		want := []string{"node[0]", "node[1]", "node[2]", "node[3]"}[i]
		if r.Ref.Name != want {
			t.Errorf("ref[%d].name = %q, want %q", i, r.Ref.Name, want)
		}
	}
}

func TestCountUsingLengthOfList(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		variable "subnets" { default = ["a", "b", "c", "d"] }
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "per_subnet" {
		  count         = length(var.subnets)
		  instance_type = "t3.small"
		  ami           = "ami-x"
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Errorf("expected 4 resources, got %d", len(got))
	}
}

func TestForEachOverMap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "by_role" {
		  for_each      = { web = "t3.small", api = "t3.medium" }
		  instance_type = each.value
		  ami           = "ami-x"
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Ref.Name < got[j].Ref.Name })
	if len(got) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(got))
	}
	if got[0].Ref.Name != `by_role["api"]` {
		t.Errorf("name[0] = %q", got[0].Ref.Name)
	}
	if got[0].Attributes["instance_type"] != "t3.medium" {
		t.Errorf("each.value did not resolve correctly: %v", got[0].Attributes["instance_type"])
	}
}

func TestForEachOverToset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "by_role" {
		  for_each      = toset(["web", "api", "worker"])
		  instance_type = "t3.small"
		  ami           = "ami-${each.key}"
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 resources, got %d", len(got))
	}
}

func TestFormatAndLookupResolveInAttributes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		variable "tiers" { default = { prod = "m5.xlarge", dev = "t3.small" } }
		variable "env"   { default = "prod" }
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "node" {
		  instance_type = lookup(var.tiers, var.env, "t3.micro")
		  ami           = format("ami-%s-%04d", var.env, 7)
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Attributes["instance_type"] != "m5.xlarge" {
		t.Errorf("lookup mis-resolved: %v", got[0].Attributes["instance_type"])
	}
	if got[0].Attributes["ami"] != "ami-prod-0007" {
		t.Errorf("format mis-resolved: %v", got[0].Attributes["ami"])
	}
}

func TestAutoTfvarsOverridesDefault(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		variable "instance_type" { default = "t3.micro" }
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "web" {
		  instance_type = var.instance_type
		  ami           = "ami-x"
		}
	`)
	write(t, dir, "terraform.tfvars", `instance_type = "m5.xlarge"`)

	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Attributes["instance_type"] != "m5.xlarge" {
		t.Errorf("expected tfvars override, got %v", got[0].Attributes["instance_type"])
	}
}

func TestAutoTfvarsLexicalOrder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		variable "size" { default = "s" }
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "x" {
		  instance_type = var.size
		  ami           = "ami-x"
		}
	`)
	write(t, dir, "a.auto.tfvars", `size = "first"`)
	write(t, dir, "z.auto.tfvars", `size = "last"`)

	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Attributes["instance_type"] != "last" {
		t.Errorf("expected lexically-last auto.tfvars to win, got %v",
			got[0].Attributes["instance_type"])
	}
}

func TestCliVarBeatsTfvars(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		variable "region" { default = "fallback" }
		provider "aws" { region = var.region }
		resource "aws_instance" "x" {
		  instance_type = "t3.micro"
		  ami           = "ami-x"
		}
	`)
	write(t, dir, "terraform.tfvars", `region = "from-file"`)

	got, err := terraform.ParseDirectory(dir, terraform.Options{
		Vars: map[string]string{"region": `"from-cli"`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Region == nil || *got[0].Region != "from-cli" {
		t.Errorf("CLI var didn't win; region = %v", got[0].Region)
	}
}

func TestJsonTfvarsLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		variable "size" { default = "s" }
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "x" {
		  instance_type = var.size
		  ami           = "ami-x"
		}
	`)
	write(t, dir, "terraform.tfvars.json", `{"size": "xl"}`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Attributes["instance_type"] != "xl" {
		t.Errorf("got %v", got[0].Attributes["instance_type"])
	}
}

func TestDataBlockIdResolvesToPlaceholder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		provider "aws" { region = "us-east-1" }
		data "aws_ami" "ubuntu" {
		  most_recent = true
		  owners      = ["099720109477"]
		}
		resource "aws_instance" "web" {
		  instance_type = "t3.small"
		  ami           = data.aws_ami.ubuntu.id
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(got))
	}
	if got[0].Attributes["ami"] != "data.aws_ami.ubuntu.id" {
		t.Errorf("ami = %v, want data placeholder", got[0].Attributes["ami"])
	}
}

func TestNestedBlockBecomesNestedMap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "web" {
		  instance_type = "t3.small"
		  ami           = "ami-x"
		  root_block_device {
		    volume_size = 50
		    volume_type = "gp3"
		  }
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rbd, ok := got[0].Attributes["root_block_device"].(map[string]any)
	if !ok {
		t.Fatalf("expected root_block_device map, got %T", got[0].Attributes["root_block_device"])
	}
	if rbd["volume_size"] != float64(50) {
		t.Errorf("volume_size = %v", rbd["volume_size"])
	}
	if rbd["volume_type"] != "gp3" {
		t.Errorf("volume_type = %v", rbd["volume_type"])
	}
}

func TestMultiCloudRegionDetection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		provider string
		attr     string
		want     string
	}{
		{"aws", "region", "us-east-1"},
		{"azurerm", "location", "westus"},
		{"google", "region", "us-central1"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.provider, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			write(t, dir, "main.tf", `
				provider "`+tc.provider+`" { `+tc.attr+` = "`+tc.want+`" }
				resource "aws_instance" "x" {
				  instance_type = "t3.micro"
				  ami           = "ami-x"
				}
			`)
			got, err := terraform.ParseDirectory(dir, terraform.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got[0].Region == nil || *got[0].Region != tc.want {
				t.Errorf("region = %v, want %s", got[0].Region, tc.want)
			}
		})
	}
}

func TestLocalModuleIsExpandedAndResourcesPrefixed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	moduleDir := filepath.Join(dir, "modules", "web")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, moduleDir, "main.tf", `
		variable "size" {}
		variable "name" {}
		resource "aws_instance" "node" {
		  instance_type = var.size
		  ami           = "ami-${var.name}"
		}
	`)
	write(t, dir, "main.tf", `
		provider "aws" { region = "us-east-1" }
		module "frontend" {
		  source = "./modules/web"
		  size   = "t3.medium"
		  name   = "fe"
		}
	`)

	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(got))
	}
	if got[0].Ref.Name != "module.frontend.node" {
		t.Errorf("name = %q, want module.frontend.node", got[0].Ref.Name)
	}
	if got[0].Attributes["instance_type"] != "t3.medium" {
		t.Errorf("instance_type = %v", got[0].Attributes["instance_type"])
	}
	if got[0].Attributes["ami"] != "ami-fe" {
		t.Errorf("ami = %v", got[0].Attributes["ami"])
	}
	if got[0].Region == nil || *got[0].Region != "us-east-1" {
		t.Errorf("region not inherited from parent provider")
	}
}

func TestNestedModulesRecurse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	inner := filepath.Join(dir, "modules", "inner")
	outer := filepath.Join(dir, "modules", "outer")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outer, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, inner, "main.tf", `
		variable "size" {}
		resource "aws_instance" "leaf" {
		  instance_type = var.size
		  ami           = "ami-x"
		}
	`)
	write(t, outer, "main.tf", `
		variable "passthrough" {}
		module "inner" {
		  source = "../inner"
		  size   = var.passthrough
		}
	`)
	write(t, dir, "main.tf", `
		provider "aws" { region = "us-east-1" }
		module "outer" {
		  source      = "./modules/outer"
		  passthrough = "m5.large"
		}
	`)
	got, err := terraform.ParseDirectory(dir, terraform.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(got))
	}
	if got[0].Ref.Name != "module.outer.module.inner.leaf" {
		t.Errorf("nested module addressing wrong: %q", got[0].Ref.Name)
	}
	if got[0].Attributes["instance_type"] != "m5.large" {
		t.Errorf("instance_type didn't propagate: %v", got[0].Attributes["instance_type"])
	}
}

func TestNonLocalModuleSkippedWithoutManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "main.tf", `
		provider "aws" { region = "us-east-1" }
		module "remote" {
		  source = "terraform-aws-modules/vpc/aws"
		  version = "5.0.0"
		}
		resource "aws_instance" "x" {
		  instance_type = "t3.micro"
		  ami           = "ami-x"
		}
	`)
	// Offline: without it the native module fetcher would download
	// the real registry module mid-test. With it, no manifest entry
	// and no network → the module is skipped with a warning.
	got, err := terraform.ParseDirectory(dir, terraform.Options{Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	// The registry module is ignored (offline, no manifest); the
	// local resource still costs out.
	if len(got) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(got))
	}
	if got[0].Ref.Name != "x" {
		t.Errorf("name = %q, want x", got[0].Ref.Name)
	}
}
