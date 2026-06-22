package pricing_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/pricing"
	"github.com/shopspring/decimal"
)

// fakeServer returns an httptest.Server that responds to every POST
// with `body` and tracks every received request in `received`.
func fakeServer(t *testing.T, status int, body string, received *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*received = append(*received, string(raw))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestHTTPSourceReturnsFirstNonZeroPrice(t *testing.T) {
	t.Parallel()

	// Mirrors the SNS shape: free-tier $0 first, paid rate second.
	var seen []string
	srv := fakeServer(t, 200, `{
		"data": {"products": [{"prices": [
			{"USD": "0.0000000000", "unit": "Notifications"},
			{"USD": "0.0000006000", "unit": "Notifications"}
		]}]}
	}`, &seen)
	t.Cleanup(srv.Close)

	src := pricing.NewHTTPSource(
		pricing.WithEndpoint(srv.URL),
		pricing.WithHTTPClient(srv.Client()),
	)
	got, label, err := src.Lookup(context.Background(), pricing.Query{
		Provider: "aws", Service: "AmazonSNS", Region: "us-east-1",
		AttributeFilters: []pricing.KV{{Key: "endpointType", Value: "HTTP"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := decimal.RequireFromString("0.0000006")
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
	if label != domain.PriceSourceLive {
		t.Errorf("label = %q, want %q", label, domain.PriceSourceLive)
	}
	if len(seen) != 1 {
		t.Fatalf("expected 1 POST, got %d", len(seen))
	}
}

// TestHTTPSourcePicksMaxFromTieredPrices pins the conservative-tier
// behaviour for products like CloudFront that return a mix of
// committed-tier and on-demand-tier rates in a single response.
func TestHTTPSourcePicksMaxFromTieredPrices(t *testing.T) {
	t.Parallel()

	// Real CloudFront US-DataTransfer-Out shape: first two are
	// committed tiers (cheaper), then on-demand tiers.
	var receivedTiered []string
	srvTiered := fakeServer(t, 200, `{
		"data": {"products": [{"prices": [
			{"USD": "0.0250000000"},
			{"USD": "0.0200000000"},
			{"USD": "0.0850000000"},
			{"USD": "0.0800000000"},
			{"USD": "0.0600000000"}
		]}]}
	}`, &receivedTiered)
	t.Cleanup(srvTiered.Close)

	src := pricing.NewHTTPSource(
		pricing.WithEndpoint(srvTiered.URL),
		pricing.WithHTTPClient(srvTiered.Client()),
	)
	got, _, err := src.Lookup(context.Background(), pricing.Query{Service: "AmazonCloudFront"})
	if err != nil {
		t.Fatal(err)
	}
	want := decimal.RequireFromString("0.085")
	if !got.Equal(want) {
		t.Errorf("got %s, want %s (max non-zero from tiered array)", got, want)
	}
}

func TestHTTPSourceOmitsRegionWhenGlobal(t *testing.T) {
	t.Parallel()

	var seen []string
	srv := fakeServer(t, 200, `{"data":{"products":[]}}`, &seen)
	t.Cleanup(srv.Close)

	src := pricing.NewHTTPSource(
		pricing.WithEndpoint(srv.URL),
		pricing.WithHTTPClient(srv.Client()),
	)
	_, _, err := src.Lookup(context.Background(), pricing.Query{
		Provider: "aws", Service: "AmazonCloudFront", Region: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 {
		t.Fatalf("expected 1 POST, got %d", len(seen))
	}
	// Decode the GraphQL envelope; the body field must not contain a
	// region: clause when the caller asked for "global".
	var env struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(seen[0]), &env); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(env.Query, "region:") {
		t.Errorf("region filter not omitted for global query:\n%s", env.Query)
	}
}

func TestHTTPSourceReturnsZeroWhenNoMatch(t *testing.T) {
	t.Parallel()

	var seen []string
	srv := fakeServer(t, 200, `{"data":{"products":[]}}`, &seen)
	t.Cleanup(srv.Close)

	src := pricing.NewHTTPSource(
		pricing.WithEndpoint(srv.URL),
		pricing.WithHTTPClient(srv.Client()),
	)
	got, _, err := src.Lookup(context.Background(), pricing.Query{
		Provider: "aws", Service: "AmazonEC2", Region: "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero, got %s", got)
	}
}

func TestHTTPSourceSurfacesUpstreamErrors(t *testing.T) {
	t.Parallel()

	var seen []string
	srv := fakeServer(t, 200, `{"errors":[{"message":"context deadline exceeded"}]}`, &seen)
	t.Cleanup(srv.Close)

	src := pricing.NewHTTPSource(
		pricing.WithEndpoint(srv.URL),
		pricing.WithHTTPClient(srv.Client()),
	)
	_, _, err := src.Lookup(context.Background(), pricing.Query{
		Provider: "aws", Service: "AmazonEC2",
	})
	if err == nil {
		t.Fatal("expected error from GraphQL errors block")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("error doesn't mention upstream message: %v", err)
	}
}

func TestHTTPSourceSurfacesHTTPNon200(t *testing.T) {
	t.Parallel()

	var seen []string
	srv := fakeServer(t, 503, `{"error":"down"}`, &seen)
	t.Cleanup(srv.Close)

	src := pricing.NewHTTPSource(
		pricing.WithEndpoint(srv.URL),
		pricing.WithHTTPClient(srv.Client()),
	)
	_, _, err := src.Lookup(context.Background(), pricing.Query{Provider: "aws"})
	if err == nil {
		t.Fatal("expected non-200 to surface an error")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error doesn't mention status: %v", err)
	}
}

// TestLookupFallsBackToReferenceRegion pins the regional-miss
// behaviour: a query for a region the upstream has no rows for must
// retry against the provider's reference region instead of silently
// returning $0 — the worst failure mode a cost tool can have.
func TestLookupFallsBackToReferenceRegion(t *testing.T) {
	t.Parallel()
	var regions []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		q := string(body)
		switch {
		case strings.Contains(q, `eu-west-3`):
			regions = append(regions, "eu-west-3")
			_, _ = io.WriteString(w, `{"data":{"products":[]}}`)
		case strings.Contains(q, `us-east-1`):
			regions = append(regions, "us-east-1")
			_, _ = io.WriteString(w, `{"data":{"products":[{"prices":[{"USD":"0.0464"}]}]}}`)
		default:
			t.Errorf("unexpected query: %s", q)
		}
	}))
	defer srv.Close()

	src := pricing.NewHTTPSource(pricing.WithEndpoint(srv.URL))
	got, _, err := src.Lookup(context.Background(), pricing.Query{
		Provider: "aws", Service: "AmazonEC2", ProductFamily: "Compute Instance",
		Region: "eu-west-3", PurchaseOption: "on_demand",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "0.0464" {
		t.Errorf("rate = %s, want fallback us-east-1 rate 0.0464", got)
	}
	if len(regions) != 2 || regions[0] != "eu-west-3" || regions[1] != "us-east-1" {
		t.Errorf("query order = %v, want [eu-west-3 us-east-1]", regions)
	}
}

// TestLookupNoFallbackWhenRegionalHit: a successful regional lookup
// must NOT trigger a second query.
func TestLookupNoFallbackWhenRegionalHit(t *testing.T) {
	t.Parallel()
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = io.WriteString(w, `{"data":{"products":[{"prices":[{"USD":"0.052"}]}]}}`)
	}))
	defer srv.Close()

	src := pricing.NewHTTPSource(pricing.WithEndpoint(srv.URL))
	got, _, err := src.Lookup(context.Background(), pricing.Query{
		Provider: "aws", Service: "AmazonEC2", Region: "eu-west-3", PurchaseOption: "on_demand",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "0.052" {
		t.Errorf("rate = %s, want regional 0.052", got)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 upstream call, got %d", calls)
	}
}
