package runtime

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMultiTenantRateLimiter(t *testing.T) {
	// 1. Define resolver mapping headers to tiers
	resolver := func(r *http.Request) (string, TenantConfig, error) {
		tenantID := r.Header.Get("x-tenant-id")
		if tenantID == "" {
			return "", TenantConfig{}, fmt.Errorf("missing tenant header")
		}

		if tenantID == "tenant-free" {
			return tenantID, TenantConfig{RequestsPerSecond: 1, BurstSize: 1}, nil
		}
		if tenantID == "tenant-premium" {
			return tenantID, TenantConfig{RequestsPerSecond: 100, BurstSize: 100}, nil
		}
		return tenantID, TenantConfig{}, nil // Use defaults
	}

	defaultLimit := TenantConfig{RequestsPerSecond: 5, BurstSize: 5}
	limiter := NewMultiTenantRateLimiter(defaultLimit, resolver)

	// Mock handler that returns 200 OK
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Context().Value("tenant-id").(string)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("OK: %s", tenantID)))
	})

	middleware := limiter.HTTPMiddleware(dummyHandler)

	// 2. Test Tenant Free: Burst = 1
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.Header.Set("x-tenant-id", "tenant-free")
	w1 := httptest.NewRecorder()
	middleware.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w1.Code)
	}

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("x-tenant-id", "tenant-free")
	w2 := httptest.NewRecorder()
	middleware.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for free tenant on second request, got %d", w2.Code)
	}

	// 3. Test Tenant Premium: Burst = 100 (should not throttle)
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("x-tenant-id", "tenant-premium")
		w := httptest.NewRecorder()
		middleware.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for premium tenant on req %d, got %d", i, w.Code)
		}
	}
}
