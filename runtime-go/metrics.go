package runtime

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HelixRequestsTotal tracks the total number of RPC requests.
	HelixRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "helix_requests_total",
			Help: "Total number of RPC requests served by Helix.",
		},
		[]string{"method", "path", "status"},
	)

	// HelixRequestDurationSeconds tracks the request latency in seconds.
	HelixRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "helix_request_duration_seconds",
			Help:    "RPC execution latency histogram in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"method", "path"},
	)

	// HelixErrorsTotal tracks the total number of RPC errors.
	HelixErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "helix_errors_total",
			Help: "Total number of RPC errors.",
		},
		[]string{"method", "path"},
	)

	// HelixActiveConnections tracks active connections per backend.
	HelixActiveConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "helix_backend_active_connections",
			Help: "Number of active connections to each backend.",
		},
		[]string{"backend"},
	)
)

func init() {
	prometheus.MustRegister(HelixRequestsTotal)
	prometheus.MustRegister(HelixRequestDurationSeconds)
	prometheus.MustRegister(HelixErrorsTotal)
	prometheus.MustRegister(HelixActiveConnections)
}

// RecordRequestMetrics updates request, error, and latency metrics.
func RecordRequestMetrics(method, path string, status int, duration time.Duration) {
	statusStr := strconv.Itoa(status)
	HelixRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	HelixRequestDurationSeconds.WithLabelValues(method, path).Observe(duration.Seconds())
	if status >= 400 {
		HelixErrorsTotal.WithLabelValues(method, path).Inc()
	}
}

// RecordActiveConnections sets the active connections count for a backend.
func RecordActiveConnections(backend string, count int64) {
	HelixActiveConnections.WithLabelValues(backend).Set(float64(count))
}

// MetricsInterceptor returns a UnaryServerInterceptor that captures Prometheus metrics.
func MetricsInterceptor() UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		status := http.StatusOK
		if err != nil {
			status = http.StatusInternalServerError
			var he *HelixError
			if errors.As(err, &he) {
				status = MapToHTTPStatus(he.Code)
			}
		}

		RecordRequestMetrics("POST", info.FullMethod, status, duration)
		return resp, err
	}
}

// HTTPMetricsMiddleware wraps an http.Handler to capture Prometheus metrics.
func HTTPMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Wrap ResponseWriter to capture status code
		rw := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
		
		defer func() {
			duration := time.Since(start)
			RecordRequestMetrics(r.Method, r.URL.Path, rw.status, duration)
		}()

		next.ServeHTTP(rw, r)
	})
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// MountMetricsHandler mounts /metrics and /__helix/metrics onto the mux.
func MountMetricsHandler(mux *http.ServeMux) {
	handler := promhttp.Handler()
	mux.Handle("/metrics", handler)
	mux.Handle("/metrics/", handler)
	mux.Handle("/__helix/metrics", handler)
	mux.Handle("/__helix/metrics/", handler)
}
