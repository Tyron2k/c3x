package pricing

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestSelectRate(t *testing.T) {
	t.Parallel()
	prices := []enumPrice{
		{USD: "0", Unit: "Hrs", PurchaseOption: "on_demand"},      // skipped: zero
		{USD: "0.0104", Unit: "Hrs", PurchaseOption: "on_demand"}, // candidate
		{USD: "0.0050", Unit: "Hrs", PurchaseOption: "reserved"},  // wrong PO
		{USD: "0.0200", Unit: "GB", PurchaseOption: "on_demand"},  // wrong unit
		{USD: "0.0090", Unit: "Hrs", PurchaseOption: "on_demand"}, // candidate, lower
	}
	cases := []struct {
		name     string
		po, unit string
		want     string
	}{
		{"po+unit filter, max non-zero", "on_demand", "Hrs", "0.0104"},
		{"no unit filter takes max across units", "on_demand", "", "0.02"},
		{"any sentinel skips po filter", "any", "Hrs", "0.0104"},
		{"empty po skips po filter", "", "Hrs", "0.0104"},
		{"no match -> zero", "spot", "Hrs", "0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := selectRate(prices, c.po, c.unit)
			if !got.Equal(decimal.RequireFromString(c.want)) {
				t.Fatalf("selectRate(%q,%q) = %s, want %s", c.po, c.unit, got, c.want)
			}
		})
	}
}

func TestMatchesFixedAndProject(t *testing.T) {
	t.Parallel()
	attrs := map[string]string{"operatingSystem": "Linux", "instanceType": "t3.micro"}
	if !matchesFixed(attrs, []KV{{Key: "operatingSystem", Value: "Linux"}}) {
		t.Fatal("expected Linux to match fixed filter")
	}
	if matchesFixed(attrs, []KV{{Key: "operatingSystem", Value: "Windows"}}) {
		t.Fatal("expected Windows to NOT match")
	}
	if _, ok := projectFilters(attrs, []string{"instanceType", "tenancy"}); ok {
		t.Fatal("expected miss when a key is absent")
	}
	kvs, ok := projectFilters(attrs, []string{"instanceType"})
	if !ok || len(kvs) != 1 || kvs[0] != (KV{Key: "instanceType", Value: "t3.micro"}) {
		t.Fatalf("unexpected projection: %+v ok=%v", kvs, ok)
	}
}

// enumStub serves the enumeration GraphQL query. It returns the EC2
// product set only for us-east-1 so the test can exercise the
// reference-region fallback for other regions.
func enumStub(t *testing.T) *httptest.Server {
	t.Helper()
	const usEast1 = `{"data":{"products":[
		{"productFamily":"Compute Instance","attributes":[{"key":"instanceType","value":"t3.micro"},{"key":"operatingSystem","value":"Linux"}],"prices":[{"USD":"0.0104","unit":"Hrs","purchaseOption":"on_demand"}]},
		{"productFamily":"Compute Instance","attributes":[{"key":"instanceType","value":"t3.micro"},{"key":"operatingSystem","value":"Windows"}],"prices":[{"USD":"0.0204","unit":"Hrs","purchaseOption":"on_demand"}]},
		{"productFamily":"Storage","attributes":[{"key":"volumeType","value":"gp3"}],"prices":[{"USD":"0.08","unit":"GB-Mo","purchaseOption":"on_demand"}]}
	]}}`
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		w.Header().Set("Content-Type", "application/json")
		// Only the reference region carries products; offset>0 is empty.
		if strings.Contains(body, `region:\"us-east-1\"`) && strings.Contains(body, `offset:0`) {
			_, _ = w.Write([]byte(usEast1))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"products":[]}}`))
	}))
}

func TestSyncThenOfflineLookup(t *testing.T) {
	t.Parallel()
	srv := enumStub(t)
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "cache.db")
	shapes := []MappingShape{{
		Provider:       "aws",
		Service:        "AmazonEC2",
		ProductFamily:  "Compute Instance",
		FixedFilters:   []KV{{Key: "operatingSystem", Value: "Linux"}},
		FilterKeys:     []string{"instanceType"},
		PurchaseOption: "on_demand",
		Unit:           "Hrs",
	}}

	res, err := Sync(context.Background(), SyncOptions{
		Endpoint:    srv.URL,
		CachePath:   cachePath,
		Regions:     []string{"us-east-1"},
		Shapes:      shapes,
		Concurrency: 2,
		Client:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// One key: the Linux t3.micro. Windows is excluded by the fixed
	// filter; Storage by product family.
	if res.Entries != 1 {
		t.Fatalf("entries = %d, want 1", res.Entries)
	}

	disk, err := OpenDiskCache(cachePath, NewStub(), WithTTL(-1))
	if err != nil {
		t.Fatalf("open cache: %v", err)
	}
	off := NewOfflineSource(disk)
	defer func() { _ = off.Close() }()

	// The query the engine would build for a Linux t3.micro in us-east-1.
	q := Query{
		Provider:         "aws",
		Service:          "AmazonEC2",
		ProductFamily:    "Compute Instance",
		Region:           "us-east-1",
		AttributeFilters: []KV{{Key: "instanceType", Value: "t3.micro"}, {Key: "operatingSystem", Value: "Linux"}},
		PurchaseOption:   "on_demand",
		Unit:             "Hrs",
	}
	rate, _, err := off.Lookup(context.Background(), q)
	if err != nil {
		t.Fatalf("offline lookup: %v", err)
	}
	if !rate.Equal(decimal.RequireFromString("0.0104")) {
		t.Fatalf("us-east-1 rate = %s, want 0.0104", rate)
	}

	// A region we never synced resolves via the reference-region fallback.
	qOther := q
	qOther.Region = "eu-west-1"
	rate2, _, err := off.Lookup(context.Background(), qOther)
	if err != nil {
		t.Fatalf("offline fallback lookup: %v", err)
	}
	if !rate2.Equal(decimal.RequireFromString("0.0104")) {
		t.Fatalf("fallback rate = %s, want 0.0104 (reference region)", rate2)
	}

	// A Windows query was never written (fixed filter excluded it) → $0,
	// matching live "no priced products".
	qWin := q
	qWin.AttributeFilters = []KV{{Key: "instanceType", Value: "t3.micro"}, {Key: "operatingSystem", Value: "Windows"}}
	rWin, _, err := off.Lookup(context.Background(), qWin)
	if err != nil {
		t.Fatalf("windows lookup: %v", err)
	}
	if !rWin.IsZero() {
		t.Fatalf("windows rate = %s, want 0 (not synced)", rWin)
	}
}

func TestPlanUnitsRegionOverride(t *testing.T) {
	t.Parallel()
	shapes := []MappingShape{
		{Provider: "aws", Service: "AmazonEC2"},                                  // ref region
		{Provider: "aws", Service: "AmazonCloudFront", RegionOverride: "global"}, // pinned
	}
	units := planUnits(shapes, nil) // default regions => reference region
	regionsByService := map[string]string{}
	for _, u := range units {
		regionsByService[u.service] = u.region
	}
	if regionsByService["AmazonEC2"] != "us-east-1" {
		t.Fatalf("EC2 region = %q, want us-east-1", regionsByService["AmazonEC2"])
	}
	if regionsByService["AmazonCloudFront"] != "global" {
		t.Fatalf("CloudFront region = %q, want global (override)", regionsByService["AmazonCloudFront"])
	}
}
