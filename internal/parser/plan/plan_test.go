package plan_test

import (
	"strings"
	"testing"

	"github.com/c3xdev/c3x/internal/parser/plan"
)

func TestParsesPlanWithModuleAddresses(t *testing.T) {
	t.Parallel()

	raw := `{
		"resource_changes": [
			{
				"address": "module.frontend.aws_instance.web",
				"type": "aws_instance",
				"name": "web",
				"change": {
					"actions": ["create"],
					"after": { "instance_type": "m5.large", "ami": "ami-123" }
				}
			},
			{
				"address": "aws_db_instance.main",
				"type": "aws_db_instance",
				"name": "main",
				"change": {
					"actions": ["create"],
					"after": { "instance_class": "db.t3.medium" }
				}
			},
			{
				"address": "aws_instance.gone",
				"type": "aws_instance",
				"name": "gone",
				"change": {
					"actions": ["delete"],
					"after": {}
				}
			}
		],
		"configuration": {
			"provider_config": {
				"aws": { "expressions": { "region": { "constant_value": "us-east-2" } } }
			}
		}
	}`

	got, err := plan.ParseBytes([]byte(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 resources (delete filtered), got %d", len(got))
	}
	if got[0].Ref.Name != "module.frontend.web" {
		t.Errorf("module-prefixed name lost: %q", got[0].Ref.Name)
	}
	if got[1].Ref.Name != "main" {
		t.Errorf("top-level name = %q", got[1].Ref.Name)
	}
	if got[0].Region == nil || *got[0].Region != "us-east-2" {
		t.Errorf("region not picked up from provider_config: %v", got[0].Region)
	}
}

func TestPicksUpAzureLocation(t *testing.T) {
	t.Parallel()

	raw := `{
		"resource_changes": [
			{
				"address": "azurerm_storage_account.main",
				"type": "azurerm_storage_account",
				"name": "main",
				"change": {"actions": ["create"], "after": {}}
			}
		],
		"configuration": {
			"provider_config": {
				"azurerm": { "expressions": { "location": { "constant_value": "westeurope" } } }
			}
		}
	}`
	got, err := plan.ParseBytes([]byte(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Region == nil || *got[0].Region != "westeurope" {
		t.Errorf("expected westeurope, got %v", got[0].Region)
	}
}

func TestPicksUpGcpRegion(t *testing.T) {
	t.Parallel()

	raw := `{
		"resource_changes": [{
			"address": "google_compute_instance.x",
			"type": "google_compute_instance",
			"name": "x",
			"change": {"actions": ["create"], "after": {}}
		}],
		"configuration": {
			"provider_config": {
				"google": { "expressions": { "region": { "constant_value": "europe-west1" } } }
			}
		}
	}`
	got, err := plan.ParseBytes([]byte(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Region == nil || *got[0].Region != "europe-west1" {
		t.Errorf("expected europe-west1, got %v", got[0].Region)
	}
}

func TestRejectsInvalidJson(t *testing.T) {
	t.Parallel()
	_, err := plan.ParseBytes([]byte("not json"), nil)
	if err == nil {
		t.Fatalf("expected error on invalid JSON")
	}
	if !strings.Contains(err.Error(), "decode plan") {
		t.Errorf("error message lacks context: %v", err)
	}
}
