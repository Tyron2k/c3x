package pricing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// fakeFlaky returns N transient errors then a success. Used to
// exercise the retry loop without a real network.
type fakeFlaky struct {
	failTimes int32
	calls     int64
}

func (f *fakeFlaky) Lookup(_ context.Context, _ Query) (decimal.Decimal, string, error) {
	n := atomic.AddInt64(&f.calls, 1)
	if int32(n) <= f.failTimes {
		// Mimic an HTTPSource error so isTransient recognises it.
		return decimal.Zero, "live", errors.New("upstream returned 503")
	}
	return decimal.RequireFromString("0.192"), "live", nil
}

func instantSleep(_ context.Context, _ time.Duration) error { return nil }

func TestRetrySucceedsAfterTransientFailures(t *testing.T) {
	t.Parallel()
	inner := &fakeFlaky{failTimes: 2}
	r := withRetry(inner, RetryPolicy{
		MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, Multiplier: 2,
	}).(*retrying)
	r.sleep = instantSleep

	got, _, err := r.Lookup(context.Background(), Query{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(decimal.RequireFromString("0.192")) {
		t.Errorf("got %s", got)
	}
	if c := atomic.LoadInt64(&inner.calls); c != 3 {
		t.Errorf("inner called %d times, want 3", c)
	}
}

func TestRetryGivesUpAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	inner := &fakeFlaky{failTimes: 100}
	r := withRetry(inner, RetryPolicy{
		MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, Multiplier: 2,
	}).(*retrying)
	r.sleep = instantSleep

	_, _, err := r.Lookup(context.Background(), Query{})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if c := atomic.LoadInt64(&inner.calls); c != 3 {
		t.Errorf("inner called %d times, want 3", c)
	}
}

type permanentErr struct{ calls int64 }

func (p *permanentErr) Lookup(_ context.Context, _ Query) (decimal.Decimal, string, error) {
	atomic.AddInt64(&p.calls, 1)
	return decimal.Zero, "live", errors.New("upstream returned 400")
}

func TestRetryDoesNotRetryPermanentFailures(t *testing.T) {
	t.Parallel()
	inner := &permanentErr{}
	r := withRetry(inner, RetryPolicy{
		MaxAttempts: 5, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, Multiplier: 2,
	}).(*retrying)
	r.sleep = instantSleep

	_, _, err := r.Lookup(context.Background(), Query{})
	if err == nil {
		t.Fatal("expected error to surface")
	}
	if c := atomic.LoadInt64(&inner.calls); c != 1 {
		t.Errorf("inner called %d times, want 1 (permanent failure)", c)
	}
}

func TestRetryHonorsContextCancellation(t *testing.T) {
	t.Parallel()
	inner := &fakeFlaky{failTimes: 100}
	r := withRetry(inner, RetryPolicy{
		MaxAttempts: 5, InitialDelay: time.Minute, MaxDelay: time.Minute, Multiplier: 2,
	}).(*retrying)
	// Real sleep so context cancellation actually matters.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := r.Lookup(ctx, Query{})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// TestRetryAgainstFlakyServer is a defense-in-depth integration test:
// a real http.Server returns 503 twice then 200, and the chained
// retrying source must produce the expected price exactly once.
func TestRetryAgainstFlakyServer(t *testing.T) {
	t.Parallel()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&hits, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"products":[{"prices":[{"USD":"0.192"}]}]}}`))
	}))
	t.Cleanup(srv.Close)

	src := withRetry(
		NewHTTPSource(WithEndpoint(srv.URL), WithHTTPClient(srv.Client())),
		RetryPolicy{MaxAttempts: 4, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, Multiplier: 2},
	)
	got, _, err := src.Lookup(context.Background(), Query{Service: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(decimal.RequireFromString("0.192")) {
		t.Errorf("got %s", got)
	}
}
