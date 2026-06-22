package pricing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/shopspring/decimal"

	// Pure-Go SQLite driver (modernc.org/sqlite). No CGO so the c3x
	// binary stays single-static-file.
	_ "modernc.org/sqlite"
)

// DefaultCacheTTL is how long a disk-cached price is considered fresh.
// Seven days matches v2 Rust's value and is roughly the cadence at
// which AWS/Azure/GCP move their public catalogs.
const DefaultCacheTTL = 7 * 24 * time.Hour

// DiskCache wraps a Source with a SQLite-backed cache. Hits within TTL
// return without touching the inner Source; misses (or stale entries)
// fall through, then write back so subsequent runs hit.
//
// The database is opened on construction and held open for the
// process's lifetime; concurrent goroutines share it via SQLite's
// built-in locking.
type DiskCache struct {
	inner Source
	db    *sql.DB
	ttl   time.Duration
	now   func() time.Time
}

// DiskCacheOption configures a [DiskCache].
type DiskCacheOption func(*DiskCache)

// WithTTL overrides the freshness window. A zero or negative value
// disables expiry entirely.
func WithTTL(d time.Duration) DiskCacheOption {
	return func(c *DiskCache) { c.ttl = d }
}

// WithClock injects a clock — tests stamp deterministic times so
// "stale" rows are reproducible.
func WithClock(now func() time.Time) DiskCacheOption {
	return func(c *DiskCache) { c.now = now }
}

// OpenDiskCache opens (or creates) the SQLite database at `path` and
// wraps `inner` with cache reads. The schema is migrated on first
// open; subsequent opens are no-ops.
func OpenDiskCache(path string, inner Source, opts ...DiskCacheOption) (*DiskCache, error) {
	if inner == nil {
		return nil, errors.New("pricing.OpenDiskCache: inner Source is nil")
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		// Best-effort: callers usually pre-create the dir, but ensuring
		// it here means `c3x estimate` on a fresh user works.
		_ = ensureDir(dir)
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite at %s: %w", path, err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	c := &DiskCache{
		inner: inner,
		db:    db,
		ttl:   DefaultCacheTTL,
		now:   time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Close releases the SQLite handle. Idempotent.
func (c *DiskCache) Close() error { return c.db.Close() }

// Lookup implements [Source]. On a fresh hit it returns the cached
// value directly. On miss or stale, it delegates to the inner Source,
// writes the result back, then returns it.
//
// We intentionally cache zero-priced results too: ACM public certs,
// "no matching products" responses, and similar zero rows are the
// correct steady-state answer for those queries.
func (c *DiskCache) Lookup(ctx context.Context, q Query) (decimal.Decimal, string, error) {
	key := queryKey(q)
	if v, src, ok := c.read(key); ok {
		return v, src, nil
	}
	v, src, err := c.inner.Lookup(ctx, q)
	if err != nil {
		return v, src, err
	}
	c.write(key, v, src, summary(q))
	return v, src, nil
}

// read returns the cached entry if it's present and fresh. Stale rows
// are reported as misses (the caller will refresh and overwrite).
func (c *DiskCache) read(key string) (decimal.Decimal, string, bool) {
	row := c.db.QueryRow(
		`SELECT price, source, fetched_at FROM prices WHERE cache_key = ?`,
		key,
	)
	var priceStr, source string
	var fetchedAt int64
	if err := row.Scan(&priceStr, &source, &fetchedAt); err != nil {
		return decimal.Zero, "", false
	}
	if c.ttl > 0 {
		age := c.now().Unix() - fetchedAt
		if age > int64(c.ttl.Seconds()) {
			return decimal.Zero, "", false
		}
	}
	d, err := decimal.NewFromString(priceStr)
	if err != nil {
		return decimal.Zero, "", false
	}
	return d, source, true
}

func (c *DiskCache) write(key string, price decimal.Decimal, source, summary string) {
	_, _ = c.db.Exec(
		`INSERT INTO prices (cache_key, price, source, fetched_at, query_summary)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(cache_key) DO UPDATE SET
		   price = excluded.price,
		   source = excluded.source,
		   fetched_at = excluded.fetched_at,
		   query_summary = excluded.query_summary`,
		key, price.String(), source, c.now().Unix(), summary,
	)
}

// Stats describes the disk cache state. `Live` and `Stale` partition
// `Total` by the configured TTL at the moment of inspection.
type Stats struct {
	Total int64
	Live  int64
	Stale int64
}

// Stats returns the current row counts. Used by `c3x pricing stats`.
func (c *DiskCache) Stats() (Stats, error) {
	var s Stats
	if err := c.db.QueryRow(`SELECT count(*) FROM prices`).Scan(&s.Total); err != nil {
		return s, err
	}
	if c.ttl <= 0 {
		s.Live = s.Total
		return s, nil
	}
	cutoff := c.now().Add(-c.ttl).Unix()
	if err := c.db.QueryRow(
		`SELECT count(*) FROM prices WHERE fetched_at >= ?`, cutoff,
	).Scan(&s.Live); err != nil {
		return s, err
	}
	s.Stale = s.Total - s.Live
	return s, nil
}

// Clear deletes every row and returns how many were removed. Used by
// `c3x pricing clear`.
func (c *DiskCache) Clear() (int64, error) {
	res, err := c.db.Exec(`DELETE FROM prices`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// summary builds a short human-readable string stored alongside the
// price for `c3x pricing inspect`.
func summary(q Query) string {
	return fmt.Sprintf("%s/%s/%s@%s", q.Provider, q.Service, q.ProductFamily, q.Region)
}

// migrate creates the schema if absent. Called on every open; it's
// cheap and ensures users with old DB files don't need a manual step.
func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS prices (
			cache_key     TEXT PRIMARY KEY,
			price         TEXT NOT NULL,
			source        TEXT NOT NULL,
			fetched_at    INTEGER NOT NULL,
			query_summary TEXT
		);
		CREATE INDEX IF NOT EXISTS prices_fetched_at ON prices(fetched_at);
	`)
	if err != nil {
		return fmt.Errorf("migrate cache schema: %w", err)
	}
	return nil
}

// ensureDir creates a directory with sensible perms; MkdirAll is
// idempotent so concurrent calls don't race.
func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// PriceSourceLabel returns the domain-level price source label for a
// disk-cache hit. The DiskCache itself never returns "static"; that's
// the calculator's call.
func PriceSourceLabel(src string) string {
	switch src {
	case domain.PriceSourceLive, domain.PriceSourceStub, domain.PriceSourceStatic:
		return src
	default:
		return domain.PriceSourceLive
	}
}
