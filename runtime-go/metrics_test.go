package runtime

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetricsCollection(t *testing.T) {
	// Record some sample metrics
	RecordRequestMetrics("POST", "/test-method", http.StatusOK, 50*time.Millisecond)
	RecordRequestMetrics("POST", "/test-method", http.StatusInternalServerError, 150*time.Millisecond)
	RecordActiveConnections("localhost:9090", 5)

	// Create request to metrics endpoint
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	handler := promhttp.Handler()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status OK, got %v", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `helix_requests_total{method="POST",path="/test-method",status="200"} 1`) {
		t.Error("expected helix_requests_total for 200 status to be recorded")
	}
	if !strings.Contains(body, `helix_requests_total{method="POST",path="/test-method",status="500"} 1`) {
		t.Error("expected helix_requests_total for 500 status to be recorded")
	}
	if !strings.Contains(body, `helix_errors_total{method="POST",path="/test-method"} 1`) {
		t.Error("expected helix_errors_total to be recorded")
	}
	if !strings.Contains(body, `helix_backend_active_connections{backend="localhost:9090"} 5`) {
		t.Error("expected active connections metric to be recorded")
	}
}
