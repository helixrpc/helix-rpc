package runtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Debug registry — shared singletons updated by other modules
// ---------------------------------------------------------------------------

var globalDebug = &debugRegistry{
	startTime: time.Now(),
	counters:  make(map[string]*methodCounter),
}

type methodCounter struct {
	requests  atomic.Int64
	errors    atomic.Int64
	latencyNs atomic.Int64 // cumulative ns for avg
}

type debugRegistry struct {
	startTime time.Time
	mu        sync.RWMutex
	counters  map[string]*methodCounter
}

func (r *debugRegistry) counter(method string) *methodCounter {
	r.mu.RLock()
	c, ok := r.counters[method]
	r.mu.RUnlock()
	if ok {
		return c
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok = r.counters[method]; ok {
		return c
	}
	c = &methodCounter{}
	r.counters[method] = c
	return c
}

// RecordRequest is called by the telemetry interceptor for every RPC.
func RecordRequest(method string, latency time.Duration, err error) {
	c := globalDebug.counter(method)
	c.requests.Add(1)
	c.latencyNs.Add(latency.Nanoseconds())
	if err != nil {
		c.errors.Add(1)
	}
}

// ---------------------------------------------------------------------------
// Debug snapshot types (JSON-serialisable)
// ---------------------------------------------------------------------------

type debugSnapshot struct {
	Version    string                    `json:"helix_version"`
	Uptime     string                    `json:"uptime"`
	GoVersion  string                    `json:"go_version"`
	Methods    []methodSnapshot          `json:"methods"`
	Backends   []backendSnapshot         `json:"backends,omitempty"`
	Circuit    *circuitSnapshot          `json:"circuit_breaker,omitempty"`
}

type methodSnapshot struct {
	Method     string  `json:"method"`
	Requests   int64   `json:"requests_total"`
	Errors     int64   `json:"errors_total"`
	AvgLatency string  `json:"avg_latency_ms"`
}

type backendSnapshot struct {
	Addr        string `json:"addr"`
	ActiveConns int64  `json:"active_conns"`
}

type circuitSnapshot struct {
	State    string `json:"state"`
	Failures int64  `json:"consecutive_failures"`
}

// ---------------------------------------------------------------------------
// MountDebugHandler wires /__helix/debug onto an existing ServeMux.
// Call this from NewServer once.
// ---------------------------------------------------------------------------

func MountDebugHandler(mux *http.ServeMux, server *Server) {
	mux.HandleFunc("/__helix/debug", debugHandler(server))
	mux.HandleFunc("/__helix/debug/", debugHandler(server))
}

func debugHandler(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		snap := buildSnapshot(server)
		json.NewEncoder(w).Encode(snap) //nolint:errcheck
	}
}

func buildSnapshot(server *Server) debugSnapshot {
	bi, ok := debug.ReadBuildInfo()
	goVer := "unknown"
	if ok {
		goVer = bi.GoVersion
	}

	snap := debugSnapshot{
		Version:   "0.2.0",
		Uptime:    fmt.Sprintf("%.0fs", time.Since(globalDebug.startTime).Seconds()),
		GoVersion: goVer,
	}

	// Collect method stats
	globalDebug.mu.RLock()
	methods := make([]string, 0, len(globalDebug.counters))
	for m := range globalDebug.counters {
		methods = append(methods, m)
	}
	sort.Strings(methods)
	for _, m := range methods {
		c := globalDebug.counters[m]
		reqs := c.requests.Load()
		avgMs := 0.0
		if reqs > 0 {
			avgMs = float64(c.latencyNs.Load()) / float64(reqs) / 1e6
		}
		snap.Methods = append(snap.Methods, methodSnapshot{
			Method:     m,
			Requests:   reqs,
			Errors:     c.errors.Load(),
			AvgLatency: fmt.Sprintf("%.2f", avgMs),
		})
	}
	globalDebug.mu.RUnlock()

	// Backend active-conn stats (from the balancer if registered)
	if server != nil && server.balancer != nil {
		for _, addr := range server.balancerTargets {
			snap.Backends = append(snap.Backends, backendSnapshot{
				Addr:        addr,
				ActiveConns: server.balancer.ActiveConns(addr),
			})
		}
	}

	// Circuit breaker state
	if server != nil && server.debugBreaker != nil {
		snap.Circuit = &circuitSnapshot{
			State:    server.debugBreaker.State().String(),
			Failures: atomic.LoadInt64(&server.debugBreaker.failures),
		}
	}

	return snap
}
