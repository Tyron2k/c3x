package pricing

import (
	"context"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/shopspring/decimal"
)

// Stub is a deterministic, in-memory Source used by tests and the
// `--offline` runtime mode. Construct via [NewStub] and load fixtures
// with [Stub.Set].
type Stub struct {
	prices map[string]decimal.Decimal
}

// NewStub returns an empty Stub.
func NewStub() *Stub { return &Stub{prices: map[string]decimal.Decimal{}} }

// Set seeds the stub with a price for the given query. The key derived
// here is the same one [DiskCache] and [HTTPSource] use, so a stub
// populated from test fixtures will hit on the same queries the live
// engine produces.
func (s *Stub) Set(q Query, rate decimal.Decimal) {
	s.prices[queryKey(q)] = rate
}

// Lookup implements [Source]. Returns the rate stored by [Stub.Set],
// or zero+nil if no fixture matches (mirroring the live behaviour for
// "no priced products").
func (s *Stub) Lookup(_ context.Context, q Query) (decimal.Decimal, string, error) {
	if r, ok := s.prices[queryKey(q)]; ok {
		return r, domain.PriceSourceStub, nil
	}
	return decimal.Zero, domain.PriceSourceStub, nil
}
