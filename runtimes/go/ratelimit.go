package runtime

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// RateLimiter — per-client token bucket backed by sync.Map
// ---------------------------------------------------------------------------

// RateLimitConfig configures the rate limiter.
type RateLimitConfig struct {
	// RequestsPerSecond is the sustained request rate per client.
	RequestsPerSecond float64

	// BurstSize is the maximum number of tokens that can accumulate.
	// Defaults to RequestsPerSecond if zero.
	BurstSize int

	// KeyFunc extracts the rate-limit key from the request.
	// Defaults to remote IP address.
	KeyFunc func(r *http.Request) string

	// ErrorMessage is the JSON body returned on 429.
	// Defaults to a standard message.
	ErrorMessage string
}

// RateLimiter is a concurrent-safe per-client token bucket rate limiter.
type RateLimiter struct {
	cfg     RateLimitConfig
	buckets sync.Map // key → *clientBucket
}

type clientBucket struct {
	tokens   atomic.Int64 // stored as nano-tokens (1 token = 1e9 nano-tokens)
	lastSeen atomic.Int64 // Unix nanoseconds
	capacity int64        // nano-tokens
	refillNs int64        // nano-tokens per nanosecond (= tokens/s)
}

const nanoToken = int64(1e9)

func newClientBucket(rps float64, burst int) *clientBucket {
	cap := int64(float64(burst) * float64(nanoToken))
	b := &clientBucket{
		capacity: cap,
		refillNs: int64(rps), // nano-tokens/ns ≈ tokens/s at nano scale
	}
	b.tokens.Store(cap)
	b.lastSeen.Store(time.Now().UnixNano())
	return b
}

func (b *clientBucket) allow() (remaining int64, ok bool) {
	now := time.Now().UnixNano()
	last := b.lastSeen.Swap(now)
	elapsed := now - last
	if elapsed > 0 {
		refill := elapsed * b.refillNs
		for {
			cur := b.tokens.Load()
			next := cur + refill
			if next > b.capacity {
				next = b.capacity
			}
			if b.tokens.CompareAndSwap(cur, next) {
				break
			}
		}
	}
	for {
		cur := b.tokens.Load()
		if cur < nanoToken {
			return cur / nanoToken, false
		}
		if b.tokens.CompareAndSwap(cur, cur-nanoToken) {
			return cur/nanoToken - 1, true
		}
	}
}

// NewRateLimiter creates a new RateLimiter with the given config.
//
// Example:
//
//	limiter := runtime.NewRateLimiter(runtime.RateLimitConfig{
//	    RequestsPerSecond: 100,
//	    BurstSize:         20,
//	})
//	server.AddInterceptor(limiter.Interceptor())
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	if cfg.BurstSize == 0 {
		cfg.BurstSize = max(1, int(cfg.RequestsPerSecond))
	}
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = ipKeyFunc
	}
	if cfg.ErrorMessage == "" {
		cfg.ErrorMessage = `{"error":"rate limit exceeded","code":14}`
	}
	return &RateLimiter{cfg: cfg}
}

func ipKeyFunc(r *http.Request) string {
	// Strip port suffix
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}

// bucket returns (or creates) the per-client token bucket.
func (rl *RateLimiter) bucket(key string) *clientBucket {
	if v, ok := rl.buckets.Load(key); ok {
		return v.(*clientBucket)
	}
	b := newClientBucket(rl.cfg.RequestsPerSecond, rl.cfg.BurstSize)
	actual, _ := rl.buckets.LoadOrStore(key, b)
	return actual.(*clientBucket)
}

// Allow returns true if the request for the given key is within the rate limit.
func (rl *RateLimiter) Allow(key string) (remaining int64, ok bool) {
	return rl.bucket(key).allow()
}

// Interceptor returns a UnaryServerInterceptor that enforces the rate limit.
// The key is derived from the gRPC metadata (falls back to "unknown").
func (rl *RateLimiter) Interceptor() UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (interface{}, error) {
		md, _ := FromContext(ctx)
		key := "unknown"
		if ip, ok := md[":remote-addr"]; ok && len(ip) > 0 {
			key = ip[0]
		} else if fwd, ok := md["x-forwarded-for"]; ok && len(fwd) > 0 {
			key = fwd[0]
		}

		remaining, ok := rl.Allow(key)
		if !ok {
			retryAfter := fmt.Sprintf("%.0f", 1.0/rl.cfg.RequestsPerSecond)
			return nil, &HelixError{
				Code:    CodeUnavailable,
				Message: fmt.Sprintf("rate limit exceeded; retry after %ss", retryAfter),
			}
		}
		_ = remaining
		return handler(ctx, req)
	}
}

// HTTPMiddleware returns an http.Handler middleware for REST servers.
// It injects standard X-RateLimit-* response headers.
func (rl *RateLimiter) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := rl.cfg.KeyFunc(r)
		remaining, ok := rl.Allow(key)
		limit := int64(rl.cfg.BurstSize)
		retryAfter := fmt.Sprintf("%.3f", 1.0/rl.cfg.RequestsPerSecond)

		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", max64(0, remaining)))

		if !ok {
			w.Header().Set("Retry-After", retryAfter)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, rl.cfg.ErrorMessage)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// AdaptiveConcurrencyLimiter implements a concurrency limiter that dynamically
// adjusts the concurrency limit based on measured latency durations (Vegas algorithm).
type AdaptiveConcurrencyLimiter struct {
	mu             sync.Mutex
	limit          int32
	minLimit       int32
	maxLimit       int32
	rttNoLoad      time.Duration
	rttActual      time.Duration
	activeRequests int32
	alpha          float64
	beta           float64
}

// NewAdaptiveConcurrencyLimiter creates a new AdaptiveConcurrencyLimiter.
func NewAdaptiveConcurrencyLimiter(initialLimit, minLimit, maxLimit int32) *AdaptiveConcurrencyLimiter {
	return &AdaptiveConcurrencyLimiter{
		limit:    initialLimit,
		minLimit: minLimit,
		maxLimit: maxLimit,
		alpha:    3.0,
		beta:     6.0,
	}
}

func (acl *AdaptiveConcurrencyLimiter) Acquire() bool {
	for {
		active := atomic.LoadInt32(&acl.activeRequests)
		limit := atomic.LoadInt32(&acl.limit)
		if active >= limit {
			return false
		}
		if atomic.CompareAndSwapInt32(&acl.activeRequests, active, active+1) {
			return true
		}
	}
}

func (acl *AdaptiveConcurrencyLimiter) Release(rtt time.Duration) {
	atomic.AddInt32(&acl.activeRequests, -1)
	acl.updateLimit(rtt)
}

func (acl *AdaptiveConcurrencyLimiter) updateLimit(rtt time.Duration) {
	acl.mu.Lock()
	defer acl.mu.Unlock()

	if acl.rttNoLoad == 0 || rtt < acl.rttNoLoad {
		acl.rttNoLoad = rtt
	}

	if acl.rttActual == 0 {
		acl.rttActual = rtt
	} else {
		acl.rttActual = time.Duration(0.9*float64(acl.rttActual) + 0.1*float64(rtt))
	}

	limitFloat := float64(acl.limit)
	expected := limitFloat * (float64(acl.rttNoLoad) / float64(acl.rttActual))
	queue := limitFloat - expected

	if queue < acl.alpha {
		if acl.limit < acl.maxLimit {
			acl.limit++
		}
	} else if queue > acl.beta {
		if acl.limit > acl.minLimit {
			acl.limit--
		}
	}
}

func (acl *AdaptiveConcurrencyLimiter) Limit() int32 {
	return atomic.LoadInt32(&acl.limit)
}

// Interceptor returns a UnaryServerInterceptor that enforces the adaptive concurrency limit.
func (acl *AdaptiveConcurrencyLimiter) Interceptor() UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (interface{}, error) {
		if !acl.Acquire() {
			return nil, &HelixError{
				Code:    CodeResourceExhausted,
				Message: "concurrency limit exceeded (adaptive)",
			}
		}
		start := time.Now()
		resp, err := handler(ctx, req)
		acl.Release(time.Since(start))
		return resp, err
	}
}
