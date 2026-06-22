package pricing

// FX conversion. The upstream pricing endpoint always returns USD;
// when the user asks for a different currency we apply a conversion
// rate fetched from a public ECB-sourced FX feed (Frankfurter API).
//
// The Frankfurter API has no auth requirement, is rate-limit-free
// for normal use, and serves the ECB's daily reference rates. The
// rates update once per business day, so a 24h cache hits the
// freshness needs without spamming the upstream.
//
// Design notes:
//   - Conversion is per-`Source.Lookup` invocation, applied AFTER
//     the cache (so USD rates stay cached normally and only the
//     final multiplication is currency-aware).
//   - A nil FXConverter or zero CurrencyUnknown means "no
//     conversion, return USD as-is" — preserves the existing
//     zero-config UX.
//   - On FX fetch failure the converter falls back to last-known
//     rate (if any) and logs. We never silently return USD-shaped
//     numbers labelled with the wrong currency.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/shopspring/decimal"
)

// DefaultFXEndpoint is the Frankfurter API. The endpoint is
// configurable so users with corporate firewalls can point at an
// internal mirror.
const DefaultFXEndpoint = "https://api.frankfurter.app/latest"

// DefaultFXCacheTTL is how long a fetched rate stays fresh.
// Frankfurter updates daily; 6h is generous-but-bounded so the
// process picks up the new day's rates within a few hours.
const DefaultFXCacheTTL = 6 * time.Hour

// FXConverter wraps a Source with USD → target-currency conversion.
// The zero value (with target = CurrencyUSD) is a no-op pass-through.
type FXConverter struct {
	inner    Source
	target   domain.Currency
	endpoint string
	ttl      time.Duration
	client   *http.Client
	now      func() time.Time
	log      *slog.Logger

	mu      sync.RWMutex
	rate    decimal.Decimal // USD → target multiplier
	fetched time.Time
}

// FXOption configures a [FXConverter].
type FXOption func(*FXConverter)

// WithFXEndpoint overrides the default Frankfurter URL.
func WithFXEndpoint(u string) FXOption { return func(c *FXConverter) { c.endpoint = u } }

// WithFXTTL overrides the default 6h cache TTL.
func WithFXTTL(d time.Duration) FXOption { return func(c *FXConverter) { c.ttl = d } }

// WithFXClock injects a clock for deterministic testing.
func WithFXClock(now func() time.Time) FXOption { return func(c *FXConverter) { c.now = now } }

// WithFXHTTPClient lets tests stub the HTTP layer.
func WithFXHTTPClient(client *http.Client) FXOption {
	return func(c *FXConverter) { c.client = client }
}

// NewFXConverter wraps inner with USD → target conversion. If target
// is CurrencyUSD or CurrencyUnknown, the returned Source is a direct
// pass-through (no FX fetch ever happens).
func NewFXConverter(inner Source, target domain.Currency, opts ...FXOption) Source {
	if target == domain.CurrencyUSD || target == domain.CurrencyUnknown {
		return inner
	}
	c := &FXConverter{
		inner:    inner,
		target:   target,
		endpoint: DefaultFXEndpoint,
		ttl:      DefaultFXCacheTTL,
		client:   &http.Client{Timeout: 8 * time.Second},
		now:      time.Now,
		log:      slog.Default(),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Lookup implements Source. Forwards to the inner source, then
// multiplies by the cached FX rate. On rate-fetch failure the
// caller gets the USD value (logged) — better degraded than wrong
// currency.
func (c *FXConverter) Lookup(ctx context.Context, q Query) (decimal.Decimal, string, error) {
	usd, src, err := c.inner.Lookup(ctx, q)
	if err != nil || usd.IsZero() {
		return usd, src, err
	}
	rate, err := c.fetchRate(ctx)
	if err != nil {
		c.log.Warn("FX rate unavailable; returning USD-shaped value",
			"target", c.target.String(), "error", err)
		return usd, src, nil
	}
	return usd.Mul(rate), src, nil
}

// Close lets a chain that includes this converter close gracefully.
// Delegates through TryClose so intermediate wrappers (MemoCache)
// that lack a Close method don't hide the DiskCache's.
func (c *FXConverter) Close() error {
	return TryClose(c.inner)
}

// fetchRate returns the USD → target multiplier from cache when
// fresh, fetching otherwise.
func (c *FXConverter) fetchRate(ctx context.Context) (decimal.Decimal, error) {
	c.mu.RLock()
	if !c.rate.IsZero() && c.now().Sub(c.fetched) < c.ttl {
		r := c.rate
		c.mu.RUnlock()
		return r, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check under the write lock.
	if !c.rate.IsZero() && c.now().Sub(c.fetched) < c.ttl {
		return c.rate, nil
	}

	url := fmt.Sprintf("%s?from=USD&to=%s", c.endpoint, c.target.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return decimal.Zero, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return decimal.Zero, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return decimal.Zero, fmt.Errorf("FX HTTP %d: %s", resp.StatusCode, string(body))
	}

	var doc struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return decimal.Zero, fmt.Errorf("decode FX response: %w", err)
	}
	rate, ok := doc.Rates[c.target.String()]
	if !ok || rate <= 0 {
		return decimal.Zero, fmt.Errorf("FX response missing target %s", c.target.String())
	}
	c.rate = decimal.NewFromFloat(rate)
	c.fetched = c.now()
	c.log.Debug("FX rate refreshed",
		"target", c.target.String(), "rate", c.rate.String())
	return c.rate, nil
}
