package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureNonInteractiveWritesDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"configure", "--non-interactive"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("configure: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Wrote") {
		t.Errorf("expected confirmation, got: %s", out.String())
	}
	cfgPath := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "c3x", "config.toml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	for _, must := range []string{`currency = "USD"`, `endpoint = "https://pricing.c3x.dev/graphql"`} {
		if !strings.Contains(string(data), must) {
			t.Errorf("config missing %q\n--- file ---\n%s", must, string(data))
		}
	}
}

func TestConfigureInteractivePreservesExisting(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfgPath := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "c3x", "config.toml")
	_ = os.MkdirAll(filepath.Dir(cfgPath), 0o755)
	_ = os.WriteFile(cfgPath, []byte(`currency = "EUR"
region = "eu-west-1"
format = "json"

[pricing]
endpoint = "https://pricing.internal/graphql"
`), 0o644)

	cmd := newRootCmd()
	out := &bytes.Buffer{}
	// All blank lines → keep the loaded defaults.
	cmd.SetIn(strings.NewReader("\n\n\n\n"))
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"configure"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("configure: %v\n%s", err, out.String())
	}
	data, _ := os.ReadFile(cfgPath)
	for _, must := range []string{`currency = "EUR"`, `region = "eu-west-1"`, `format = "json"`, `endpoint = "https://pricing.internal/graphql"`} {
		if !strings.Contains(string(data), must) {
			t.Errorf("config lost %q\n--- file ---\n%s", must, string(data))
		}
	}
}

func TestConfigureRejectsInvalidCurrency(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newRootCmd()
	out := &bytes.Buffer{}
	// Endpoint blank, region blank, currency=NOPE → expect error.
	cmd.SetIn(strings.NewReader("\n\nNOPE\n\n"))
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"configure"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error on invalid currency")
	}
}
