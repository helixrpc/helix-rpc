package runtime

import (
	"context"
	"net/http"
	"sync"
)

// TenantConfig defines the rate limiting properties for a specific tenant.
type TenantConfig struct {
	RequestsPerSecond float64
	BurstSize         int
}

// MultiTenantRateLimiter manages dynamic rate limits per tenant.
type MultiTenantRateLimiter struct {
	mu           sync.RWMutex
	limiters     map[string]*RateLimiter
	defaultLimit TenantConfig
	tenantFn     func(r *http.Request) (string, TenantConfig, error)
}

// NewMultiTenantRateLimiter creates a new MultiTenantRateLimiter.
func NewMultiTenantRateLimiter(
	defaultLimit TenantConfig,
	tenantFn func(r *http.Request) (string, TenantConfig, error),
) *MultiTenantRateLimiter {
	return &MultiTenantRateLimiter{
		limiters:     make(map[string]*RateLimiter),
		defaultLimit: defaultLimit,
		tenantFn:     tenantFn,
	}
}

// Interceptor returns a gRPC server interceptor (or HTTP middleware) that applies multi-tenant rate limiting.
func (m *MultiTenantRateLimiter) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID, cfg, err := m.tenantFn(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized tenant","code":16}`, http.StatusUnauthorized)
			return
		}

		if tenantID == "" {
			next.ServeHTTP(w, r)
			return
		}

		m.mu.RLock()
		limiter, exists := m.limiters[tenantID]
		m.mu.RUnlock()

		if !exists {
			m.mu.Lock()
			// Double check
			limiter, exists = m.limiters[tenantID]
			if !exists {
				if cfg.RequestsPerSecond <= 0 {
					cfg = m.defaultLimit
				}
				limiter = NewRateLimiter(RateLimitConfig{
					RequestsPerSecond: cfg.RequestsPerSecond,
					BurstSize:         cfg.BurstSize,
					KeyFunc: func(req *http.Request) string {
						return tenantID
					},
				})
				m.limiters[tenantID] = limiter
			}
			m.mu.Unlock()
		}

		// Check rate limit
		bucketKey := tenantID
		val, _ := limiter.buckets.LoadOrStore(bucketKey, newClientBucket(limiter.cfg.RequestsPerSecond, limiter.cfg.BurstSize))
		bucket := val.(*clientBucket)

		if _, allowed := bucket.allow(); !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded for tenant","code":14}`))
			return
		}

		// Inject tenant ID into context
		ctx := context.WithValue(r.Context(), "tenant-id", tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
