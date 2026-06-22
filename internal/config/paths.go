// Package config owns the 5-layer configuration system:
//
//	defaults  <  ~/.config/c3x/config.toml  <  ./.c3x.toml
//	          <  C3X_* env vars             <  CLI flags
//
// Resolved exposes the merged result as plain Go fields so downstream
// modules don't carry a viper.Viper dependency.
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// UserConfigPath returns the user-level config path following XDG when
// available. On macOS we still honour XDG_CONFIG_HOME because that's
// what every cross-platform CLI does in practice.
func UserConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "c3x", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "c3x", "config.toml"), nil
		}
	}
	return filepath.Join(home, ".config", "c3x", "config.toml"), nil
}

// UserCachePath returns the user-level cache path (for the on-disk
// pricing cache). Follows XDG_CACHE_HOME conventions.
func UserCachePath() (string, error) {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "c3x", "cache.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Caches", "c3x", "cache.db"), nil
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "c3x", "cache.db"), nil
		}
	}
	return filepath.Join(home, ".cache", "c3x", "cache.db"), nil
}

// ProjectConfigPath returns the project-level config path: `.c3x.toml`
// in the same directory as the IaC input. It's a function so callers
// don't have to remember the filename.
func ProjectConfigPath(projectDir string) string {
	return filepath.Join(projectDir, ".c3x.toml")
}
