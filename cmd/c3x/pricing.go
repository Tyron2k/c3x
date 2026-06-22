package main

import (
	"fmt"
	"os"

	"github.com/c3xdev/c3x/internal/config"
	"github.com/c3xdev/c3x/internal/pricing"
	"github.com/spf13/cobra"
)

// newPricingCmd assembles the `c3x pricing` family. The three actions
// give users direct control over the on-disk cache. Without this, the
// only way to recover from a stuck $0 row was `rm ~/.cache/c3x/cache.db`
// — fine for power users, not OK as a primary UX.
func newPricingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pricing",
		Short: "Manage the on-disk price cache.",
	}
	cmd.AddCommand(
		newPricingWhereCmd(),
		newPricingStatsCmd(),
		newPricingClearCmd(),
	)
	return cmd
}

// newPricingWhereCmd prints the resolved cache path. Useful when a user
// has set XDG_CACHE_HOME or --cache-path and wants to confirm where
// c3x actually writes.
func newPricingWhereCmd() *cobra.Command {
	var cachePath string
	cmd := &cobra.Command{
		Use:   "where",
		Short: "Print the on-disk cache path.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := resolveCachePath(cachePath)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	cmd.Flags().StringVar(&cachePath, "cache-path", "", "override path (else platform default)")
	return cmd
}

// newPricingStatsCmd prints the row counts split by freshness so a
// user can see at a glance whether their cache is healthy.
func newPricingStatsCmd() *cobra.Command {
	var cachePath string
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show on-disk cache row counts (total, live, stale).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := resolveCachePath(cachePath)
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(),
					"no cache file at %s yet — run `c3x estimate` to populate it\n", path)
				return nil
			}
			// Open the cache wrapping a no-op inner source. Stats only
			// reads from SQLite so the inner is irrelevant.
			cache, err := pricing.OpenDiskCache(path, pricing.NewStub())
			if err != nil {
				return fmt.Errorf("opening cache: %w", err)
			}
			defer func() { _ = cache.Close() }()

			stats, err := cache.Stats()
			if err != nil {
				return fmt.Errorf("reading stats: %w", err)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "path:  %s\n", path)
			fmt.Fprintf(out, "total: %d\n", stats.Total)
			fmt.Fprintf(out, "live:  %d\n", stats.Live)
			fmt.Fprintf(out, "stale: %d\n", stats.Stale)
			return nil
		},
	}
	cmd.Flags().StringVar(&cachePath, "cache-path", "", "override path (else platform default)")
	return cmd
}

// newPricingClearCmd empties the cache. The print-then-act ordering
// lets users CTRL-C if they ran it accidentally; the actual deletion
// happens in one atomic SQL statement.
func newPricingClearCmd() *cobra.Command {
	var cachePath string
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Delete every row from the on-disk cache.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := resolveCachePath(cachePath)
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(),
					"no cache file at %s; nothing to clear\n", path)
				return nil
			}
			cache, err := pricing.OpenDiskCache(path, pricing.NewStub())
			if err != nil {
				return fmt.Errorf("opening cache: %w", err)
			}
			defer func() { _ = cache.Close() }()

			n, err := cache.Clear()
			if err != nil {
				return fmt.Errorf("clearing cache: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %d entries from %s\n", n, path)
			return nil
		},
	}
	cmd.Flags().StringVar(&cachePath, "cache-path", "", "override path (else platform default)")
	return cmd
}

// resolveCachePath returns the configured cache path, defaulting to
// the XDG-aware platform location when the user hasn't overridden.
func resolveCachePath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return config.UserCachePath()
}
