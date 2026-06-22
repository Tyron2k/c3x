// Package domain defines the core value types used across c3x.
//
// Modules in `internal/` may depend on `domain`, but `domain` depends on
// nothing else in the codebase. This is the contract every layer agrees on
// so that the engine, renderers, recommenders, and CLI all speak the same
// vocabulary without coupling to each other.
package domain

import (
	"fmt"
	"strings"
)

// Currency is the unit a Cost or LineItem is denominated in.
//
// Currencies are intentionally constants rather than a `string`-backed
// enum so callers can't construct an invalid one. New currencies are
// added by extending the constants and the Symbol/String maps.
type Currency int

const (
	// CurrencyUnknown is the zero value; callers must replace it with a
	// real currency before producing output. Renderers treat it as a bug.
	CurrencyUnknown Currency = iota
	CurrencyUSD
	CurrencyEUR
	CurrencyGBP
	CurrencyJPY
	CurrencyCAD
	CurrencyAUD
	CurrencyCHF
	CurrencyCNY
	CurrencyINR
	CurrencyBRL
	CurrencyMXN
	CurrencySGD
	CurrencyHKD
	CurrencyNZD
	CurrencyKRW
	CurrencySEK
	CurrencyNOK
	CurrencyDKK
	CurrencyZAR
)

// String returns the ISO 4217 code.
func (c Currency) String() string {
	if name, ok := currencyByCode[c]; ok {
		return name.code
	}
	return "UNKNOWN"
}

// Symbol returns the typographic symbol used by text renderers.
func (c Currency) Symbol() string {
	if name, ok := currencyByCode[c]; ok {
		return name.symbol
	}
	return "?"
}

// ParseCurrency converts an ISO 4217 code (case-insensitive) into the
// matching Currency. Returned errors are wrapped at the call site so
// the user sees `config: invalid currency "FOO"`.
func ParseCurrency(code string) (Currency, error) {
	upper := strings.ToUpper(strings.TrimSpace(code))
	if c, ok := currencyByName[upper]; ok {
		return c, nil
	}
	return CurrencyUnknown, fmt.Errorf("unknown currency %q", code)
}

// SupportedCurrencies returns every Currency we recognise except the
// zero value. Useful for `--help` rendering and validation in
// `c3x doctor`.
func SupportedCurrencies() []Currency {
	out := make([]Currency, 0, len(currencyByCode)-1)
	for c := range currencyByCode {
		if c == CurrencyUnknown {
			continue
		}
		out = append(out, c)
	}
	return out
}

type currencyInfo struct {
	code   string
	symbol string
}

// currencyByCode maps the typed constant to display strings. New
// currencies extend both this map and currencyByName below.
var currencyByCode = map[Currency]currencyInfo{
	CurrencyUnknown: {"UNKNOWN", "?"},
	CurrencyUSD:     {"USD", "$"},
	CurrencyEUR:     {"EUR", "€"},
	CurrencyGBP:     {"GBP", "£"},
	CurrencyJPY:     {"JPY", "¥"},
	CurrencyCAD:     {"CAD", "CA$"},
	CurrencyAUD:     {"AUD", "A$"},
	CurrencyCHF:     {"CHF", "CHF"},
	CurrencyCNY:     {"CNY", "¥"},
	CurrencyINR:     {"INR", "₹"},
	CurrencyBRL:     {"BRL", "R$"},
	CurrencyMXN:     {"MXN", "MX$"},
	CurrencySGD:     {"SGD", "S$"},
	CurrencyHKD:     {"HKD", "HK$"},
	CurrencyNZD:     {"NZD", "NZ$"},
	CurrencyKRW:     {"KRW", "₩"},
	CurrencySEK:     {"SEK", "kr"},
	CurrencyNOK:     {"NOK", "kr"},
	CurrencyDKK:     {"DKK", "kr"},
	CurrencyZAR:     {"ZAR", "R"},
}

// currencyByName is the inverse lookup. Initialised in init() so
// the two maps stay in sync.
var currencyByName = func() map[string]Currency {
	out := make(map[string]Currency, len(currencyByCode))
	for c, info := range currencyByCode {
		out[info.code] = c
	}
	return out
}()
