package pricing

import (
	"context"
	"errors"
	"math/rand/v2"
	"net"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// RetryPolicy controls how transient HTTP failures are retried.
//
// Defaults (see [DefaultRetryPolicy]) follow the well-tempered
// exponential-backoff-with-jitter convention from AWS's own client
// library guidelines: short initial delay so fast-recovery upstreams
// barely notice the retry, jitter to avoid synchronised herds.
type RetryPolicy struct {
	// MaxAttempts is the total number of tries including the first.
	// 1 means "no retries", which is also what setting Policy to a
	// zero value yields — failing closed rather than open.
	MaxAttempts int
	// InitialDelay is the wait before the first retry.
	InitialDelay time.Duration
	// MaxDelay caps the per-iteration wait so a long chain doesn't
	// blow up under the exponential schedule.
	MaxDelay time.Duration
	// Multiplier is the per-attempt growth factor.
	Multiplier float64
}

// DefaultRetryPolicy is the production setting: 3 attempts, 1s/2s/4s
// nominal delays plus jitter of up to 50%, capped at 5s. Total
// worst-case wall-clock added: ~8s on a fully-retried failure.
var DefaultRetryPolicy = RetryPolicy{
	MaxAttempts:  3,
	InitialDelay: time.Second,
	MaxDelay:     5 * time.Second,
	Multiplier:   2.0,
}

// retrying wraps a Source so transient failures are retried per
// [RetryPolicy]. Permanent errors (4xx, schema mismatches, context
// cancellation) propagate immediately.
//
// The retrying source is unexported because the constructor is
// [BuildChain]; callers wanting a tailored chain compose [HTTPSource]
// and [withRetry] in their tests.
type retrying struct {
	inner  Source
	policy RetryPolicy
	sleep  func(ctx context.Context, d time.Duration) error
}

func withRetry(inner Source, p RetryPolicy) Source {
	if p.MaxAttempts <= 1 {
		return inner
	}
	return &retrying{
		inner:  inner,
		policy: p,
		sleep: func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
}

// Lookup implements [Source]. Retries happen behind one attempt-loop
// so logs and metrics see a single logical lookup, not N separate
// ones.
func (r *retrying) Lookup(ctx context.Context, q Query) (decimal.Decimal, string, error) {
	var lastErr error
	delay := r.policy.InitialDelay
	for attempt := 1; attempt <= r.policy.MaxAttempts; attempt++ {
		v, src, err := r.inner.Lookup(ctx, q)
		if err == nil {
			return v, src, nil
		}
		if !isTransient(err) {
			return v, src, err
		}
		lastErr = err
		if attempt == r.policy.MaxAttempts {
			break
		}
		jittered := jitter(delay)
		if err := r.sleep(ctx, jittered); err != nil {
			return decimal.Zero, "", err
		}
		delay = nextDelay(delay, r.policy.Multiplier, r.policy.MaxDelay)
	}
	return decimal.Zero, "", lastErr
}

func nextDelay(current time.Duration, factor float64, ceiling time.Duration) time.Duration {
	d := time.Duration(float64(current) * factor)
	if d > ceiling {
		return ceiling
	}
	return d
}

// jitter adds ±25% randomness to a delay so a fleet of c3x runs doesn't
// stampede on the same upstream window.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	delta := float64(d) * (rand.Float64()*0.5 - 0.25) //nolint:gosec // jitter only
	return d + time.Duration(delta)
}

// isTransient classifies whether an error from the inner Source is
// worth retrying. We retry on:
//   - network errors (DNS, connection refused, timeout)
//   - upstream 5xx
//   - upstream 429 (rate-limit)
//
// We do NOT retry on:
//   - context.Canceled / context.DeadlineExceeded (caller intent)
//   - 4xx (the request is malformed — retries won't fix it)
//   - decode errors (the server returned something we can't parse)
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		// Timeouts are transient; non-timeout net.Errors are usually
		// connection-level (refused, reset, EOF mid-stream) which also
		// retry well. We classify both as transient.
		_ = netErr.Timeout()
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "upstream") {
		// HTTPSource includes the status code in the error string.
		for _, code := range []string{"500", "502", "503", "504", "429"} {
			if strings.Contains(msg, code) {
				return true
			}
		}
	}
	return false
}
