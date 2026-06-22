package pricing_test

// FX converter tests. The HTTP layer is stubbed via httptest so the
// suite never touches the real Frankfurter API. We cover:
//
//   - USD target returns the inner Source unchanged (no FX layer wrapped)
//   - non-USD target multiplies the inner USD value by the cached rate
//   - the rate is cached for the configured TTL and refreshed after
//   - HTTP failure falls back to last-known rate (logged, not errored)

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/pricing"
	"github.com/shopspring/decimal"
)

// fxServer builds an httptest server that responds to `?from=USD&to=EUR`
// with the given rate. Returns the server URL and a pointer to a
// counter so tests can assert call frequency.
func fxServer(t *testing.T, rate float64) (string, *int) {
	t.Helper()
	calls := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		target := r.URL.Query().Get("to")
		if target == "" {
			http.Error(w, "missing to=", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rates": map[string]float64{target: rate},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL, calls
}

func TestFXConverterPassthroughForUSD(t *testing.T) {
	t.Parallel()
	stub := pricing.NewStub()
	q := pricing.Query{
		Provider: "aws", Service: "AmazonEC2", ProductFamily: "Compute Instance",
		Region: "us-east-1", PurchaseOption: "on_demand",
	}
	stub.Set(q, decimal.RequireFromString("0.192"))

	wrapped := pricing.NewFXConverter(stub, domain.CurrencyUSD)
	// CurrencyUSD short-circuits to passthrough.
	if wrapped != stub {
		t.Error("FX converter should pass through stub when target is USD")
	}
}

func TestFXConverterMultipliesByFetchedRate(t *testing.T) {
	t.Parallel()
	stub := pricing.NewStub()
	q := pricing.Query{
		Provider: "aws", Service: "AmazonEC2", ProductFamily: "Compute Instance",
		Region: "us-east-1", PurchaseOption: "on_demand",
		AttributeFilters: []pricing.KV{{Key: "instanceType", Value: "m5.xlarge"}},
	}
	stub.Set(q, decimal.RequireFromString("100"))

	url, calls := fxServer(t, 0.92) // 1 USD = 0.92 EUR
	wrapped := pricing.NewFXConverter(stub, domain.CurrencyEUR,
		pricing.WithFXEndpoint(url),
	)
	got, _, err := wrapped.Lookup(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	want := decimal.RequireFromString("92")
	if !got.Equal(want) {
		t.Errorf("USD 100 × 0.92 = %s, want %s", got, want)
	}
	if *calls != 1 {
		t.Errorf("expected 1 FX fetch, got %d", *calls)
	}
}

func TestFXConverterCachesRateWithinTTL(t *testing.T) {
	t.Parallel()
	stub := pricing.NewStub()
	q := pricing.Query{
		Provider: "aws", Service: "S", Region: "us-east-1", PurchaseOption: "on_demand",
	}
	stub.Set(q, decimal.RequireFromString("1"))

	url, calls := fxServer(t, 1.5)
	wrapped := pricing.NewFXConverter(stub, domain.CurrencyGBP,
		pricing.WithFXEndpoint(url),
		pricing.WithFXTTL(time.Hour),
	)
	for i := 0; i < 5; i++ {
		if _, _, err := wrapped.Lookup(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	if *calls != 1 {
		t.Errorf("expected 1 FX fetch across 5 lookups, got %d", *calls)
	}
}

func TestFXConverterRefetchesAfterTTL(t *testing.T) {
	t.Parallel()
	stub := pricing.NewStub()
	q := pricing.Query{
		Provider: "aws", Service: "S", Region: "us-east-1", PurchaseOption: "on_demand",
	}
	stub.Set(q, decimal.RequireFromString("1"))

	url, calls := fxServer(t, 1.5)
	now := time.Now()
	clock := func() time.Time { return now }
	wrapped := pricing.NewFXConverter(stub, domain.CurrencyGBP,
		pricing.WithFXEndpoint(url),
		pricing.WithFXTTL(time.Hour),
		pricing.WithFXClock(clock),
	)
	if _, _, err := wrapped.Lookup(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	// Advance the clock past the TTL and look up again — should refetch.
	now = now.Add(2 * time.Hour)
	if _, _, err := wrapped.Lookup(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if *calls != 2 {
		t.Errorf("expected 2 FX fetches after TTL expiry, got %d", *calls)
	}
}

func TestFXConverterFallsBackToUSDOnFetchError(t *testing.T) {
	t.Parallel()
	stub := pricing.NewStub()
	q := pricing.Query{
		Provider: "aws", Service: "S", Region: "us-east-1", PurchaseOption: "on_demand",
	}
	stub.Set(q, decimal.RequireFromString("100"))

	// Server that always 500s.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "broken", http.StatusInternalServerError)
	}))
	defer srv.Close()

	wrapped := pricing.NewFXConverter(stub, domain.CurrencyEUR,
		pricing.WithFXEndpoint(srv.URL),
	)
	got, _, err := wrapped.Lookup(context.Background(), q)
	if err != nil {
		t.Errorf("expected fallback to USD without error, got %v", err)
	}
	if !got.Equal(decimal.RequireFromString("100")) {
		t.Errorf("expected USD passthrough on FX failure, got %s", got)
	}
}

func TestParseCurrencyAcceptsNewSet(t *testing.T) {
	t.Parallel()
	cases := []string{"USD", "eur", "GBP", "jpy", "cad", "AUD", "CHF", "BRL", "INR"}
	for _, code := range cases {
		c, err := domain.ParseCurrency(code)
		if err != nil {
			t.Errorf("ParseCurrency(%q): %v", code, err)
			continue
		}
		if strings.ToUpper(code) != c.String() {
			t.Errorf("ParseCurrency(%q).String() = %q", code, c.String())
		}
	}
}

// closableProbe is a Source that records whether Close was called.
// Stands in for DiskCache in the unwrap test below without coupling
// to SQLite close semantics.
type closableProbe struct {
	pricing.Source
	closed bool
}

func (p *closableProbe) Close() error {
	p.closed = true
	return nil
}

// TestTryCloseReachesInnerSourceThroughFXWrapper is the regression
// pin for the wrapper-hides-Close bug: with --currency wrapping the
// chain in an FXConverter, TryClose must still unwrap down to the
// innermost closable Source (the DiskCache in production) so the
// SQLite handle flushes on shutdown.
func TestTryCloseReachesInnerSourceThroughFXWrapper(t *testing.T) {
	t.Parallel()
	probe := &closableProbe{Source: pricing.NewStub()}
	chain := pricing.NewFXConverter(pricing.NewMemoCache(probe), domain.CurrencyEUR)
	if err := pricing.TryClose(chain); err != nil {
		t.Fatalf("TryClose through FX wrapper: %v", err)
	}
	if !probe.closed {
		t.Error("inner Source's Close was never called through FXConverter(MemoCache(...))")
	}
}
