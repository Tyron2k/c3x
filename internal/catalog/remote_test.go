package catalog_test

// Chain-semantics tests for the remote catalog client. The contract
// under test: the CLI must never end up WORSE than its embedded
// snapshot, whatever the server or network does.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c3xdev/c3x/internal/catalog"
)

const validTOML = `
kind         = "aws_remote_test_only"
display_name = "Remote Test"
provider     = "aws"

[[dimensions]]
id       = "x"
label    = "X"
unit     = "n/a"
quantity = "0"
rate     = "0"
`

func bundleJSON(t *testing.T, schema string, entries ...map[string]string) []byte {
	t.Helper()
	b := map[string]any{"schema_version": schema, "hash": "testhash", "count": len(entries), "entries": entries}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestLoadAutoUsesRemoteBundle(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bundleJSON(t, "1",
			map[string]string{"provider": "aws", "kind": "aws_remote_test_only", "toml": validTOML}))
	}))
	defer srv.Close()

	reg, err := catalog.LoadAuto(context.Background(), catalog.LoadOptions{
		Endpoint: srv.URL, CacheDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Get("aws_remote_test_only") == nil {
		t.Error("remote-only kind missing: remote bundle was not used")
	}
	if reg.Len() != 1 {
		t.Errorf("registry len = %d, want 1 (remote bundle replaces embedded)", reg.Len())
	}
}

func TestLoadAutoFallsBackToEmbeddedOnInvalidBundle(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":"1","entries":[{"provider":"aws","kind":"x","toml":"not toml at all ["}]}`))
	}))
	defer srv.Close()

	reg, err := catalog.LoadAuto(context.Background(), catalog.LoadOptions{
		Endpoint: srv.URL, CacheDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Get("aws_instance") == nil {
		t.Error("expected embedded snapshot fallback (aws_instance present)")
	}
}

func TestLoadAutoFallsBackToEmbeddedOnSchemaMismatch(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bundleJSON(t, "999",
			map[string]string{"provider": "aws", "kind": "aws_remote_test_only", "toml": validTOML}))
	}))
	defer srv.Close()

	reg, err := catalog.LoadAuto(context.Background(), catalog.LoadOptions{
		Endpoint: srv.URL, CacheDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Get("aws_remote_test_only") != nil {
		t.Error("engine accepted a bundle from an incompatible schema generation")
	}
	if reg.Get("aws_instance") == nil {
		t.Error("expected embedded snapshot fallback")
	}
}

func TestLoadAutoUsesStaleDiskCacheWhenNetworkFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	raw := bundleJSON(t, "1",
		map[string]string{"provider": "aws", "kind": "aws_remote_test_only", "toml": validTOML})
	if err := os.WriteFile(filepath.Join(dir, "catalog-bundle.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	// Age the cache beyond any MaxAge so only the stale path can use it.
	old := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(filepath.Join(dir, "catalog-bundle.json"), old, old)

	reg, err := catalog.LoadAuto(context.Background(), catalog.LoadOptions{
		Endpoint: "http://127.0.0.1:1", // nothing listens here
		CacheDir: dir,
		MaxAge:   time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Get("aws_remote_test_only") == nil {
		t.Error("stale disk cache should win over embedded when the network is down")
	}
}

func TestLoadAutoOfflineNeverTouchesNetwork(t *testing.T) {
	t.Parallel()
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
	}))
	defer srv.Close()

	reg, err := catalog.LoadAuto(context.Background(), catalog.LoadOptions{
		Endpoint: srv.URL, Offline: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Errorf("offline mode made %d network calls", hits)
	}
	if reg.Get("aws_instance") == nil {
		t.Error("offline must serve the embedded snapshot")
	}
}

func TestLoadAuto304UsesRevalidatedCache(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	raw := bundleJSON(t, "1",
		map[string]string{"provider": "aws", "kind": "aws_remote_test_only", "toml": validTOML})
	_ = os.WriteFile(filepath.Join(dir, "catalog-bundle.json"), raw, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "catalog-bundle.etag"), []byte(`"testhash"`), 0o644)
	old := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(filepath.Join(dir, "catalog-bundle.json"), old, old)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"testhash"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		t.Error("expected conditional request with cached ETag")
	}))
	defer srv.Close()

	reg, err := catalog.LoadAuto(context.Background(), catalog.LoadOptions{
		Endpoint: srv.URL, CacheDir: dir, MaxAge: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Get("aws_remote_test_only") == nil {
		t.Error("304 should revalidate and use the cached bundle")
	}
}
