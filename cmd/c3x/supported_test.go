package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSupportedResourcesTableDefault(t *testing.T) {
	cmd := newRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"supported-resources"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("supported-resources: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "KIND") || !strings.Contains(got, "PROVIDER") {
		t.Errorf("expected table header, got:\n%s", got)
	}
	if !strings.Contains(got, "aws_instance") {
		t.Errorf("expected aws_instance in default output (it's a core LIVE entry)")
	}
}

func TestSupportedResourcesFilterByProviderAndStatus(t *testing.T) {
	cmd := newRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"supported-resources", "--provider", "azure", "--status", "free"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("supported-resources: %v", err)
	}
	got := out.String()
	// Negative assertion: no AWS kind should leak through the
	// provider filter.
	if strings.Contains(got, "aws_instance") {
		t.Errorf("provider=azure filter leaked AWS kind:\n%s", got)
	}
	// Positive assertion: at least one Azure FREE kind shows up.
	if !strings.Contains(got, "FREE") {
		t.Errorf("expected at least one FREE row:\n%s", got)
	}
}

func TestSupportedResourcesJSONFormat(t *testing.T) {
	cmd := newRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"supported-resources", "--format", "json", "--provider", "gcp"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("supported-resources: %v", err)
	}
	var doc struct {
		Resources []struct {
			Kind     string `json:"kind"`
			Provider string `json:"provider"`
			Status   string `json:"status"`
		} `json:"resources"`
		Count int `json:"count"`
	}
	if err := json.NewDecoder(out).Decode(&doc); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if doc.Count == 0 {
		t.Errorf("expected GCP rows in JSON output, got 0")
	}
	for _, r := range doc.Resources {
		if r.Provider != "gcp" {
			t.Errorf("non-gcp row leaked through filter: %+v", r)
		}
	}
}

func TestSupportedResourcesRejectsUnknownFormat(t *testing.T) {
	cmd := newRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"supported-resources", "--format", "yaml"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error for unknown format")
	}
}
