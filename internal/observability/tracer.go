package observability

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// tracerName is the instrumentation name embedded in every span c3x
// emits. By convention this is the Go module path so backends can
// route or filter by it.
const tracerName = "github.com/c3xdev/c3x"

// Tracer returns the OpenTelemetry tracer c3x instruments against.
//
// By default the global tracer provider is a no-op, so calls to
// `span.End()` are zero-cost and no network IO happens. Users who
// want real traces install their own TracerProvider in main()
// (via go.opentelemetry.io/otel/sdk/trace), and the spans we already
// emit start flowing — no c3x code changes required.
//
// We intentionally don't depend on the otel SDK in this package; the
// SDK pulls in OTLP exporters and gRPC, which would bloat the binary
// for the 99% of users who don't need them. SDK choice belongs in
// the caller's program, not in the library.
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}
