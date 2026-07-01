package runtime

import (
	"context"
	"math/rand"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// SamplingStrategy controls how many spans are recorded.
type SamplingStrategy int

const (
	// SampleAll records every request (good for dev/debug, expensive in prod).
	SampleAll SamplingStrategy = iota
	// SampleNone records no spans (disables telemetry entirely).
	SampleNone
	// SampleProbabilistic records a random fraction of requests.
	SampleProbabilistic
)

// TelemetryConfig controls tracing behaviour for a Helix gateway.
type TelemetryConfig struct {
	// TracerName is the OpenTelemetry instrumentation library name.
	TracerName string
	// Strategy selects the sampling approach.
	Strategy SamplingStrategy
	// SampleRate is the fraction [0.0, 1.0] of requests to trace when
	// Strategy == SampleProbabilistic. 0.01 = 1%.
	SampleRate float64
}

// DefaultTelemetryConfig returns a 1% probabilistic sampling configuration.
func DefaultTelemetryConfig(tracerName string) TelemetryConfig {
	return TelemetryConfig{
		TracerName: tracerName,
		Strategy:   SampleProbabilistic,
		SampleRate: 0.01,
	}
}

// TelemetryInterceptor automatically extracts W3C trace context from incoming
// requests and starts an OpenTelemetry span for the RPC execution.
//
// It honours SamplingStrategy so that high-frequency production traffic is not
// uniformly instrumented (which would overwhelm the collector and waste CPU).
func TelemetryInterceptor(cfg TelemetryConfig) UnaryServerInterceptor {
	tracer := otel.Tracer(cfg.TracerName)
	propagator := otel.GetTextMapPropagator()
	if propagator == nil {
		propagator = propagation.TraceContext{}
	}

	shouldSample := buildSampler(cfg)

	return func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (interface{}, error) {
		if !shouldSample() {
			// Skip tracing for this request; still invoke the handler.
			return handler(ctx, req)
		}

		md, ok := FromContext(ctx)
		var span trace.Span

		if ok {
			headerMap := make(http.Header)
			for k, values := range md {
				for _, v := range values {
					headerMap.Add(k, v)
				}
			}
			ctx = propagator.Extract(ctx, propagation.HeaderCarrier(headerMap))
		}

		ctx, span = tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		span.SetAttributes(attribute.String("rpc.method", info.FullMethod))

		resp, err := handler(ctx, req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		return resp, err
	}
}

// buildSampler returns a zero-allocation closure for the configured strategy.
func buildSampler(cfg TelemetryConfig) func() bool {
	switch cfg.Strategy {
	case SampleAll:
		return func() bool { return true }
	case SampleNone:
		return func() bool { return false }
	default: // SampleProbabilistic
		rate := cfg.SampleRate
		if rate <= 0 {
			return func() bool { return false }
		}
		if rate >= 1 {
			return func() bool { return true }
		}
		return func() bool {
			return rand.Float64() < rate //nolint:gosec // sampling doesn't require crypto-random
		}
	}
}
