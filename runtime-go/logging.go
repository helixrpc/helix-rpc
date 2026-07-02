package runtime

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// LoggingInterceptor returns a UnaryServerInterceptor that records request execution times,
// method details, status, and OpenTelemetry trace IDs using structured JSON logging.
func LoggingInterceptor() UnaryServerInterceptor {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		md, _ := FromContext(ctx)
		traceID := ""
		if md != nil {
			traceVals := md.Get("traceparent")
			if len(traceVals) > 0 {
				traceID = traceVals[0]
			}
		}

		status := "success"
		if err != nil {
			status = "error"
		}

		logger.Info("RPC Request",
			"method", info.FullMethod,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"trace_id", traceID,
		)
		return resp, err
	}
}
