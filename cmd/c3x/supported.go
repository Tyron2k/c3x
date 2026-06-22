package main

// `c3x supported-resources` lists every kind c3x understands, with
// per-kind status (LIVE / STATIC / FREE) derived from the catalog.
// Useful for users discovering what's supported without running an
// estimate or reading source.
//
// Output formats mirror the rest of the CLI: a human table by
// default, JSON for piping into jq, CSV for spreadsheet workflows.
// Filtering by provider keeps the output skimmable when a user only
// cares about one cloud.

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/c3xdev/c3x/internal/catalog"
	"github.com/spf13/cobra"
)

func newSupportedResourcesCmd() *cobra.Command {
	var (
		provider string
		format   string
		status   string
	)
	cmd := &cobra.Command{
		Use:   "supported-resources",
		Short: "List the resource kinds c3x can estimate, with their pricing status.",
		Long: `Prints every Terraform / CloudFormation resource kind in the
catalog plus its pricing status:

  LIVE    queried against pricing.c3x.dev
  STATIC  inline rate (upstream catalog doesn't expose the meter)
  FREE    no per-resource charge (parent-billed, IAM, VPC plumbing, …)

Filter with --provider aws|azure|gcp or --status live|static|free.
Output --format table (default), json, or csv.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, err := catalog.Load()
			if err != nil {
				return fmt.Errorf("loading catalog: %w", err)
			}
			rows := buildSupportedRows(reg, provider, status)
			switch strings.ToLower(format) {
			case "", "table":
				return writeSupportedTable(cmd, rows)
			case "json":
				return writeSupportedJSON(cmd, rows)
			case "csv":
				return writeSupportedCSV(cmd, rows)
			default:
				return fmt.Errorf("unsupported format %q (want table|json|csv)", format)
			}
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "filter by provider (aws|azure|gcp)")
	cmd.Flags().StringVar(&format, "format", "table", "output format (table|json|csv)")
	cmd.Flags().StringVar(&status, "status", "", "filter by status (live|static|free)")
	return cmd
}

// supportedRow is the public-API shape of one catalog entry. We
// avoid leaking internal catalog types so consumers of the JSON
// output don't pin to fields we may reshape later.
type supportedRow struct {
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
	Status      string `json:"status"`
}

func buildSupportedRows(reg *catalog.Registry, provider, statusFilter string) []supportedRow {
	provider = strings.ToLower(provider)
	statusFilter = strings.ToLower(statusFilter)

	kinds := reg.Kinds()
	sort.Strings(kinds)

	rows := make([]supportedRow, 0, len(kinds))
	for _, k := range kinds {
		def := reg.Get(k)
		if def == nil {
			continue
		}
		if provider != "" && !strings.EqualFold(def.Provider, provider) {
			continue
		}
		status := statusForKind(reg, def)
		if statusFilter != "" && !strings.EqualFold(status, statusFilter) {
			continue
		}
		rows = append(rows, supportedRow{
			Kind:        def.Kind,
			DisplayName: def.DisplayName,
			Provider:    def.Provider,
			Status:      status,
		})
	}
	return rows
}

// statusForKind mirrors the verifier's bucketing: FREE > STATIC >
// LIVE in resolution order. Free-shell TOMLs (every dimension
// quantifies to 0) take precedence so e.g. `aws_iam_role` reads as
// FREE rather than STATIC just because the rate is the literal "0".
func statusForKind(reg *catalog.Registry, def *catalog.Definition) string {
	if catalog.IsLegitimatelyFree(def.Kind) || reg.IsFreeShell(def.Kind) {
		return "FREE"
	}
	if reg.HasStaticRate(def.Kind) {
		return "STATIC"
	}
	return "LIVE"
}

func writeSupportedTable(cmd *cobra.Command, rows []supportedRow) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KIND\tPROVIDER\tSTATUS\tDISPLAY NAME")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Kind, r.Provider, r.Status, r.DisplayName)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n%d resources (filter applied where given)\n", len(rows))
	return nil
}

func writeSupportedJSON(cmd *cobra.Command, rows []supportedRow) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		Resources []supportedRow `json:"resources"`
		Count     int            `json:"count"`
	}{rows, len(rows)})
}

func writeSupportedCSV(cmd *cobra.Command, rows []supportedRow) error {
	w := csv.NewWriter(cmd.OutOrStdout())
	defer w.Flush()
	if err := w.Write([]string{"kind", "provider", "status", "display_name"}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{r.Kind, r.Provider, r.Status, r.DisplayName}); err != nil {
			return err
		}
	}
	return nil
}
