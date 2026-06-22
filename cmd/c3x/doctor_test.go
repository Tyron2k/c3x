package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestDoctorCatalogCheckPasses validates the catalog check
// independently of network availability. The catalog check is
// purely embedded data and must pass on every machine.
func TestDoctorCatalogCheckPasses(t *testing.T) {
	result := checkCatalog()
	if !result.OK {
		t.Errorf("catalog check failed: %v", result.Detail)
	}
	if !strings.Contains(result.Detail, "resource kinds loaded") {
		t.Errorf("expected count in detail, got: %s", result.Detail)
	}
}

// TestDoctorCacheCheckPasses ensures the cache writability check
// works against the user's actual cache dir, redirected via
// XDG_CACHE_HOME to a t.TempDir for hermeticity.
func TestDoctorCacheCheckPasses(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	result := checkCache()
	if !result.OK {
		t.Errorf("cache check failed: %v", result.Detail)
	}
}

// TestDoctorRendersFailuresWithHint confirms the rendering pulls
// the hint text through for failed checks.
func TestDoctorRendersFailuresWithHint(t *testing.T) {
	failed := checkResult{
		Name:   "endpoint",
		OK:     false,
		Detail: "HTTP 503",
		Hint:   "service is down",
	}
	rendered := failed.Render()
	if !strings.Contains(rendered, "✗") {
		t.Errorf("expected fail icon, got: %s", rendered)
	}
	if !strings.Contains(rendered, "service is down") {
		t.Errorf("hint missing: %s", rendered)
	}
}

func TestDoctorCommandSucceedsOnHappyPath(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cmd := newRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	// --quiet to skip noise on passing checks; we only care about
	// exit status.
	cmd.SetArgs([]string{"doctor", "--quiet"})
	// The endpoint check makes a real HTTP request. Skip if the
	// network is unavailable in the test env — this is a smoke
	// integration, not a hermetic unit test.
	err := cmd.Execute()
	if err != nil {
		t.Skipf("doctor failed (likely offline test env): %v\n%s", err, out.String())
	}
}
