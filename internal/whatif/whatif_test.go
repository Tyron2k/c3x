package whatif_test

import (
	"testing"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/whatif"
)

func TestParseCoercesTypes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want any
	}{
		{`aws_instance.web.instance_type=m6i.xlarge`, "m6i.xlarge"},
		{`aws_instance.web.monitored=true`, true},
		{`aws_instance.web.monitored=false`, false},
		{`aws_ebs_volume.x.size=200`, int64(200)},
		{`aws_ebs_volume.x.iops=3.5`, 3.5},
		{`aws_s3_bucket.x.bucket="my-bucket"`, "my-bucket"},
	}
	for _, tc := range cases {
		got, err := whatif.Parse([]string{tc.in})
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.in, err)
			continue
		}
		if got[0].Value != tc.want {
			t.Errorf("Parse(%q).Value = %v (%T), want %v (%T)",
				tc.in, got[0].Value, got[0].Value, tc.want, tc.want)
		}
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	t.Parallel()
	cases := []string{
		"missing-equals",
		"only.two=x",
		"=val",
	}
	for _, tc := range cases {
		if _, err := whatif.Parse([]string{tc}); err == nil {
			t.Errorf("expected error for %q", tc)
		}
	}
}

func TestApplyOverridesMatchingResource(t *testing.T) {
	t.Parallel()
	resources := []domain.Resource{
		{
			Ref:        domain.Reference{Kind: "aws_instance", Name: "web"},
			Attributes: map[string]any{"instance_type": "m5.xlarge"},
		},
		{Ref: domain.Reference{Kind: "aws_instance", Name: "api"}},
	}
	overrides, _ := whatif.Parse([]string{`aws_instance.web.instance_type=m6i.xlarge`})
	unmatched := whatif.Apply(resources, overrides)
	if len(unmatched) != 0 {
		t.Errorf("expected zero unmatched, got %d", len(unmatched))
	}
	if resources[0].Attributes["instance_type"] != "m6i.xlarge" {
		t.Errorf("override didn't apply: %v", resources[0].Attributes)
	}
	// Untargeted resource is untouched.
	if _, ok := resources[1].Attributes["instance_type"]; ok {
		t.Errorf("api resource was unexpectedly modified: %v", resources[1].Attributes)
	}
}

func TestApplyReportsUnmatched(t *testing.T) {
	t.Parallel()
	overrides, _ := whatif.Parse([]string{`aws_typo.x.attr=value`})
	unmatched := whatif.Apply(nil, overrides)
	if len(unmatched) != 1 {
		t.Errorf("expected 1 unmatched, got %d", len(unmatched))
	}
}

func TestParseHandlesAttrWithDots(t *testing.T) {
	t.Parallel()
	// Resource names can contain dots when module prefixes are added.
	// LHS `aws_instance.module.frontend.web.instance_type` should
	// parse as kind=aws_instance, name=module.frontend.web,
	// attr=instance_type.
	overrides, err := whatif.Parse([]string{
		`aws_instance.module.frontend.web.instance_type=m6i.xlarge`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if overrides[0].Name != "module.frontend.web" {
		t.Errorf("Name = %q, want module.frontend.web", overrides[0].Name)
	}
	if overrides[0].Attr != "instance_type" {
		t.Errorf("Attr = %q", overrides[0].Attr)
	}
}
