package main

// `c3x configure` is the first-run wizard. It prompts for the
// handful of settings most users want to pin (pricing endpoint,
// default region, default currency, default format) and writes them
// to the user-level config file at the XDG-standard location.
//
// Design choices:
//   - prompts only ask for things that have non-obvious defaults;
//     for cache path, format, etc. we use the platform defaults
//     and surface them in the confirmation summary.
//   - non-interactive mode (--non-interactive) writes defaults
//     directly; useful for Dockerfile / Ansible workflows.
//   - existing config file: read it as the prompt's default so
//     `c3x configure` doubles as "edit my config".

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/c3xdev/c3x/internal/config"
	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/pricing"
	"github.com/c3xdev/c3x/internal/render"
	"github.com/spf13/cobra"
)

func newConfigureCmd() *cobra.Command {
	var nonInteractive bool
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Run the first-run setup wizard or edit existing config.",
		Long: `Walks through the handful of settings most users pin once and
writes the result to the user-level config file at
~/.config/c3x/config.toml (XDG-aware). Re-running pre-fills the
prompts from the current values.

The wizard owns this file: it rewrites it with the wizard-managed
keys only (currency, format, region, no_cache, pricing.endpoint).
Keep hand-added settings in a project-level .c3x.toml, which the
wizard never touches.

In non-interactive environments (CI containers, Dockerfile
RUN steps) pass --non-interactive to skip the prompts and write
the platform defaults.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.UserConfigPath()
			if err != nil {
				return fmt.Errorf("locating config path: %w", err)
			}
			existing := loadExistingConfig(path)
			values := existing
			if !nonInteractive {
				values, err = promptValues(cmd.InOrStdin(), cmd.OutOrStdout(), existing)
				if err != nil {
					return err
				}
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("creating config dir: %w", err)
			}
			if err := writeConfig(path, values); err != nil {
				return fmt.Errorf("writing config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nWrote %s\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false,
		"skip prompts; write current values (or platform defaults if empty)")
	return cmd
}

// configValues is the small subset of `Resolved` the wizard
// persists. Keeping it intentionally narrow — power-users edit the
// TOML by hand for the long-tail options.
type configValues struct {
	Endpoint string
	Region   string
	Currency string
	Format   string
	NoCache  bool
}

func loadExistingConfig(path string) configValues {
	v := configValues{
		Endpoint: pricing.DefaultEndpoint,
		Region:   "us-east-1",
		Currency: domain.CurrencyUSD.String(),
		Format:   render.FormatText.String(),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return v
	}
	// Lightweight parser — we only read 4-5 keys, full TOML decoding
	// would pull a parser into this tiny path.
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"`)
		switch key {
		case "pricing.endpoint", "endpoint":
			v.Endpoint = val
		case "region":
			v.Region = val
		case "currency":
			v.Currency = val
		case "format":
			v.Format = val
		case "no_cache":
			v.NoCache = val == "true"
		}
	}
	return v
}

func promptValues(in io.Reader, out io.Writer, current configValues) (configValues, error) {
	r := bufio.NewReader(in)
	fmt.Fprintln(out, "c3x configure")
	fmt.Fprintln(out, "Press <enter> to keep the shown default.")
	fmt.Fprintln(out)

	ask := func(label, defaultVal string) (string, error) {
		fmt.Fprintf(out, "  %s [%s]: ", label, defaultVal)
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return defaultVal, nil
		}
		return line, nil
	}

	out2 := configValues{}
	var err error
	if out2.Endpoint, err = ask("Pricing endpoint", current.Endpoint); err != nil {
		return current, err
	}
	if out2.Region, err = ask("Default region (us-east-1, eu-west-1, …)", current.Region); err != nil {
		return current, err
	}
	if out2.Currency, err = ask("Default currency (USD, EUR, GBP, JPY, …)", current.Currency); err != nil {
		return current, err
	}
	if out2.Format, err = ask("Default output format (text, markdown, json, junit, html, csv, sarif)", current.Format); err != nil {
		return current, err
	}
	// Validate currency choice now (cheap), so the user sees the
	// error before we write a broken config.
	if _, err := domain.ParseCurrency(out2.Currency); err != nil {
		return current, fmt.Errorf("currency: %w", err)
	}
	if _, err := render.ParseFormat(out2.Format); err != nil {
		return current, fmt.Errorf("format: %w", err)
	}
	return out2, nil
}

func writeConfig(path string, v configValues) error {
	var b strings.Builder
	b.WriteString("# c3x user configuration\n")
	b.WriteString("# Generated by `c3x configure`. NOTE: re-running configure rewrites\n")
	b.WriteString("# this file with only the wizard-managed keys (currency, format,\n")
	b.WriteString("# region, no_cache, pricing.endpoint); keep hand-added settings in\n")
	b.WriteString("# a project-level .c3x.toml instead.\n\n")
	fmt.Fprintf(&b, "currency = %q\n", v.Currency)
	fmt.Fprintf(&b, "format = %q\n", v.Format)
	fmt.Fprintf(&b, "region = %q\n", v.Region)
	if v.NoCache {
		b.WriteString("no_cache = true\n")
	}
	b.WriteString("\n[pricing]\n")
	fmt.Fprintf(&b, "endpoint = %q\n", v.Endpoint)
	return os.WriteFile(path, []byte(b.String()), 0o600)
}
