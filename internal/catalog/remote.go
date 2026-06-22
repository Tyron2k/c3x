package catalog

// Remote catalog loading. The pricing API (c3x-pricing-api) is the
// knowledge base; this CLI is a client. Definitions are fetched from
// the /catalog endpoint as a JSON bundle of raw TOML entries, cached
// on disk against the bundle's content hash (ETag), and parsed by
// the same loader/validator the embedded snapshot uses.
//
// Failure philosophy — the CLI must never get WORSE because the
// network or a server deploy is bad:
//
//	fetch OK + bundle valid      → use remote (freshest knowledge)
//	fetch OK + bundle INVALID    → warn loudly, use embedded snapshot
//	fetch fails + disk cache     → use cached bundle (stale is fine)
//	fetch fails + no cache       → use embedded snapshot
//	--offline                    → embedded snapshot, no network
//
// A schema_version major the engine doesn't understand counts as
// "bundle invalid": refusing to evaluate unknown semantics beats
// silently mis-pricing.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing/fstest"
	"time"
)

// EngineSchemaVersion is the catalog TOML schema generation this
// engine understands. Must match the bundle's schema_version major.
const EngineSchemaVersion = "1"

// DefaultCatalogEndpoint serves the knowledge base.
const DefaultCatalogEndpoint = "https://pricing.c3x.dev/catalog"

// remoteBundle mirrors the server's transport shape.
type remoteBundle struct {
	SchemaVersion string `json:"schema_version"`
	Hash          string `json:"hash"`
	Count         int    `json:"count"`
	Entries       []struct {
		Provider string `json:"provider"`
		Kind     string `json:"kind"`
		TOML     string `json:"toml"`
	} `json:"entries"`
}

// LoadOptions configure the loader chain.
type LoadOptions struct {
	// Endpoint of the catalog service. Empty → DefaultCatalogEndpoint.
	Endpoint string
	// CacheDir for the bundle disk cache. Empty → no disk caching.
	CacheDir string
	// Offline skips the network entirely (embedded snapshot only).
	Offline bool
	// MaxAge before a cached bundle is revalidated. Zero → 1h.
	MaxAge time.Duration

	HTTPClient *http.Client
	Logger     *slog.Logger
}

// LoadAuto resolves the catalog through the remote → disk-cache →
// embedded chain described in the package comment.
func LoadAuto(ctx context.Context, opts LoadOptions) (*Registry, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Offline {
		return Load()
	}
	if opts.Endpoint == "" {
		opts.Endpoint = DefaultCatalogEndpoint
	}
	if opts.MaxAge == 0 {
		opts.MaxAge = time.Hour
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}

	// Fresh disk cache → no network at all.
	if raw, ok := readCachedBundle(opts.CacheDir, opts.MaxAge); ok {
		if reg, err := registryFromBundle(raw); err == nil {
			return reg, nil
		}
		// Corrupt cache: fall through to refetch.
	}

	raw, notModified, err := fetchBundle(ctx, opts)
	switch {
	case err == nil && notModified:
		// Revalidated: any cached copy (even past MaxAge) is current.
		if cached, ok := readCachedBundle(opts.CacheDir, 0); ok {
			if reg, perr := registryFromBundle(cached); perr == nil {
				touchCachedBundle(opts.CacheDir)
				return reg, nil
			}
		}
		// 304 but no usable cache — refetch without the ETag.
		raw, _, err = fetchBundle(ctx, LoadOptions{
			Endpoint: opts.Endpoint, HTTPClient: opts.HTTPClient, Logger: opts.Logger,
		})
		if err == nil {
			if reg, perr := registryFromBundle(raw); perr == nil {
				writeCachedBundle(opts.CacheDir, raw, opts.Logger)
				return reg, nil
			}
		}
	case err == nil:
		reg, perr := registryFromBundle(raw)
		if perr == nil {
			writeCachedBundle(opts.CacheDir, raw, opts.Logger)
			return reg, nil
		}
		opts.Logger.Warn("remote catalog bundle is invalid; using embedded snapshot",
			"endpoint", opts.Endpoint, "error", perr)
		return Load()
	}

	// Network failed: stale cache beats the embedded snapshot
	// (it is at most one deploy behind; the snapshot is one release
	// behind).
	if cached, ok := readCachedBundle(opts.CacheDir, 0); ok {
		if reg, perr := registryFromBundle(cached); perr == nil {
			opts.Logger.Debug("catalog fetch failed; using stale disk cache", "error", err)
			return reg, nil
		}
	}
	opts.Logger.Debug("catalog fetch failed; using embedded snapshot", "error", err)
	return Load()
}

// fetchBundle GETs the catalog, sending If-None-Match when a cached
// hash exists. Returns notModified=true on 304.
func fetchBundle(ctx context.Context, opts LoadOptions) (raw []byte, notModified bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.Endpoint, http.NoBody)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "c3x (+https://c3x.dev)")
	if etag := readCachedETag(opts.CacheDir); etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusNotModified:
		return nil, true, nil
	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		return body, false, err
	default:
		return nil, false, fmt.Errorf("catalog endpoint HTTP %d", resp.StatusCode)
	}
}

// registryFromBundle parses + validates a bundle through the same
// path as the embedded catalog (an in-memory FS reuses LoadFromFS
// and every validation rule it enforces).
func registryFromBundle(raw []byte) (*Registry, error) {
	var b remoteBundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("decode bundle: %w", err)
	}
	if major, _, _ := strings.Cut(b.SchemaVersion, "."); major != EngineSchemaVersion {
		return nil, fmt.Errorf("bundle schema_version %q not supported by this engine (want %s.x)",
			b.SchemaVersion, EngineSchemaVersion)
	}
	if len(b.Entries) == 0 {
		return nil, fmt.Errorf("bundle has no entries")
	}
	mem := fstest.MapFS{}
	for _, e := range b.Entries {
		if e.Provider == "" || e.Kind == "" {
			return nil, fmt.Errorf("bundle entry missing provider/kind")
		}
		mem[e.Provider+"/"+e.Kind+".toml"] = &fstest.MapFile{Data: []byte(e.TOML)}
	}
	return LoadFromFS(mem, ".")
}

// --- disk cache ------------------------------------------------------

func bundlePath(dir string) string { return filepath.Join(dir, "catalog-bundle.json") }
func etagPath(dir string) string   { return filepath.Join(dir, "catalog-bundle.etag") }

func readCachedBundle(dir string, maxAge time.Duration) ([]byte, bool) {
	if dir == "" {
		return nil, false
	}
	info, err := os.Stat(bundlePath(dir))
	if err != nil {
		return nil, false
	}
	if maxAge > 0 && time.Since(info.ModTime()) > maxAge {
		return nil, false
	}
	raw, err := os.ReadFile(bundlePath(dir))
	if err != nil {
		return nil, false
	}
	return raw, true
}

func readCachedETag(dir string) string {
	if dir == "" {
		return ""
	}
	raw, err := os.ReadFile(etagPath(dir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func writeCachedBundle(dir string, raw []byte, logger *slog.Logger) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Debug("catalog cache dir", "error", err)
		return
	}
	if err := os.WriteFile(bundlePath(dir), raw, 0o600); err != nil {
		logger.Debug("catalog cache write", "error", err)
		return
	}
	var b remoteBundle
	if json.Unmarshal(raw, &b) == nil && b.Hash != "" {
		_ = os.WriteFile(etagPath(dir), []byte(`"`+b.Hash+`"`), 0o600)
	}
}

// touchCachedBundle resets the cache freshness window after a
// successful 304 revalidation.
func touchCachedBundle(dir string) {
	if dir == "" {
		return
	}
	now := time.Now()
	_ = os.Chtimes(bundlePath(dir), now, now)
}
