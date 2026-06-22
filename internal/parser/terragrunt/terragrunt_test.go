package terragrunt_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/c3xdev/c3x/internal/parser/terragrunt"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParsesSimpleTerragruntConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	moduleDir := filepath.Join(dir, "module")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, moduleDir, "main.tf", `
		variable "instance_type" {}
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "web" {
		  instance_type = var.instance_type
		  ami           = "ami-x"
		}
	`)
	write(t, dir, "terragrunt.hcl", `
		terraform {
		  source = "./module"
		}
		inputs = {
		  instance_type = "m5.xlarge"
		}
	`)

	got, err := terragrunt.ParseDirectory(dir, terragrunt.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(got))
	}
	if got[0].Attributes["instance_type"] != "m5.xlarge" {
		t.Errorf("input not threaded: %v", got[0].Attributes["instance_type"])
	}
}

func TestLocalsInTerragruntScope(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	moduleDir := filepath.Join(dir, "module")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, moduleDir, "main.tf", `
		variable "name" {}
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "x" {
		  instance_type = "t3.small"
		  ami           = "ami-${var.name}"
		}
	`)
	write(t, dir, "terragrunt.hcl", `
		locals {
		  env    = "prod"
		  prefix = "${local.env}-app"
		}
		terraform { source = "./module" }
		inputs = { name = local.prefix }
	`)

	got, err := terragrunt.ParseDirectory(dir, terragrunt.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Attributes["ami"] != "ami-prod-app" {
		t.Errorf("locals didn't flow through: %v", got[0].Attributes["ami"])
	}
}

func TestIncludeFindsParent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	childDir := filepath.Join(dir, "envs", "prod")
	moduleDir := filepath.Join(dir, "modules", "web")
	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, moduleDir, "main.tf", `
		variable "name" {}
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "x" {
		  instance_type = "t3.small"
		  ami           = "ami-${var.name}"
		}
	`)
	// Parent terragrunt.hcl at repo root supplies a baseline input.
	write(t, dir, "terragrunt.hcl", `
		inputs = {
		  name = "from-parent"
		}
	`)
	write(t, childDir, "terragrunt.hcl", `
		include "root" {
		  path = find_in_parent_folders()
		}
		terraform {
		  source = "../../modules/web"
		}
	`)

	got, err := terragrunt.ParseDirectory(childDir, terragrunt.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Attributes["ami"] != "ami-from-parent" {
		t.Errorf("parent include didn't merge: %v", got[0].Attributes["ami"])
	}
}

func TestCliVarOverridesTerragruntInput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	moduleDir := filepath.Join(dir, "module")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, moduleDir, "main.tf", `
		variable "instance_type" {}
		provider "aws" { region = "us-east-1" }
		resource "aws_instance" "x" {
		  instance_type = var.instance_type
		  ami           = "ami-x"
		}
	`)
	write(t, dir, "terragrunt.hcl", `
		terraform { source = "./module" }
		inputs    = { instance_type = "m5.xlarge" }
	`)

	got, err := terragrunt.ParseDirectory(dir, terragrunt.Options{
		Vars: map[string]string{"instance_type": `"t3.micro"`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Attributes["instance_type"] != "t3.micro" {
		t.Errorf("CLI override should beat terragrunt input: %v",
			got[0].Attributes["instance_type"])
	}
}

func TestRejectsRemoteSource(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "terragrunt.hcl", `
		terraform {
		  source = "git::https://github.com/example/modules//web"
		}
	`)
	_, err := terragrunt.ParseDirectory(dir, terragrunt.Options{})
	if err == nil {
		t.Fatal("expected error on remote source")
	}
}

func TestIsTerragruntDirectory(t *testing.T) {
	t.Parallel()
	withCfg := t.TempDir()
	write(t, withCfg, "terragrunt.hcl", `inputs = {}`)
	if !terragrunt.IsTerragruntDirectory(withCfg) {
		t.Errorf("expected true for directory with terragrunt.hcl")
	}
	if terragrunt.IsTerragruntDirectory(t.TempDir()) {
		t.Errorf("expected false for empty directory")
	}
}
