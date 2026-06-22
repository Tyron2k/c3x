package catalog_test

// Benchmarks for catalog load + lookup. Run with:
//
//	go test ./internal/catalog/... -bench=. -benchmem -run='^$'
//
// These exist so future TOML count growth doesn't silently regress
// startup time. The catalog is loaded once per CLI invocation;
// anything over ~100 ms starts being user-noticeable.

import (
	"testing"

	"github.com/c3xdev/c3x/internal/catalog"
)

func BenchmarkLoad(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := catalog.Load()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRegistryGet(b *testing.B) {
	reg, err := catalog.Load()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reg.Get("aws_instance")
	}
}

func BenchmarkRegistryGetMiss(b *testing.B) {
	reg, err := catalog.Load()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reg.Get("aws_does_not_exist")
	}
}
