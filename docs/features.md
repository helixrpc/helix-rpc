# Core Features

## Dynamic Batching
AI models process large batches of data far more efficiently than individual requests. HTTP clients, however, fire one request at a time.

Helix RPC's `BatchScheduler` intercepts concurrent individual REST/gRPC requests, holds them open for a configurable window (e.g. 50 ms), collapses them into a single array, and dispatches the array to the AI model as one GPU call. When the model finishes, the scheduler fans results back out to the correct waiting HTTP connections.

This single feature can increase GPU throughput by up to **10,000%** under high load.

**Available in:** Go (`batching.go`), Rust (`batching.rs`), Python (`batching.py`)

---

## Zero-Serialization Engine (Rust)
JSON unmarshaling is incredibly CPU-intensive. Under heavy load, an API gateway can spend 60% of its CPU cycles just formatting JSON and Protobufs to talk to a Python backend.

Helix RPC completely bypasses the network stack between the Gateway and the Model. It uses Rust's `PyO3` to invoke Python C-bindings natively in-memory — zero network hops, zero serialization, zero copies.

**Available in:** Rust only (by design)

---

## Native SSE Streaming
Chat UIs require real-time token streaming. Helix RPC implements native Server-Sent Events (SSE) across all three runtimes. When a client sends `Accept: text/event-stream`, the gateway automatically detects the stream, launches the async generator, and proxies yields into standard `data: {...}\n\n` SSE frames.

**Available in:** Go, Rust, Python, Node.js, C++, Java

---

## Graceful Shutdown
In Kubernetes, pods are terminated during rolling updates. Without graceful shutdown, in-flight AI inference requests are silently dropped.

Helix RPC implements graceful shutdown across all three runtimes:
- **Go**: `broadcast.Sender` signals all connection handlers to call `conn.GracefulShutdown()` and drain.
- **Rust**: `tokio::sync::broadcast` with `graceful_shutdown()` on each hyper connection.
- **Python**: SIGTERM/SIGINT handlers set a stop event; `stop_async(drain_seconds=5.0)` waits for in-flight requests before tearing down the `AppRunner`.

**Available in:** Go, Rust, Python, Node.js

---

## Structured Errors (`HelixError`)
All three runtimes define a common `HelixError` type carrying a gRPC-compatible `ErrorCode` (0–16) alongside a human-readable message. The runtime automatically maps the code to the correct HTTP status (e.g. `NOT_FOUND → 404`, `UNAVAILABLE → 503`) so handlers simply raise/return the error and the framework takes care of the wire format.

**Available in:** Go (`errors.go`), Rust (`errors.rs`), Python (`errors.py`), Node.js (`errors.ts`)

---

## Production Middlewares
- **mTLS**: Mutually authenticated TLS (`tokio-rustls` in Rust; `crypto/tls` in Go).
- **Health Checking**: Standard `grpc.health.v1.Health/Check` auto-mounted so Kubernetes liveness/readiness probes work out of the box.
- **Interceptors / Middleware**: Full unary interceptor chains in Go; `aiohttp` middleware stack in Python; `Service` wrappers in Rust.
- **Deadline Propagation (`grpc-timeout`)**: All six units (`n/u/m/S/M/H`) parsed and converted into native `context.Context` deadlines (Go), `tokio::time::timeout` (Rust), or `asyncio.wait_for` (Python).
- **Per-Message Compression**: Automatic `gzip` compress/decompress when the client sends `grpc-encoding: gzip`, across all runtimes.

**Available in:** Go, Rust, Python, Node.js, C++, Java

---

## Distributed Tracing (OpenTelemetry)
W3C `traceparent` / `tracestate` headers are extracted from every inbound request and used to start a child span, allowing full distributed tracing from the edge gateway down to the GPU model — across language boundaries.

A **configurable probabilistic sampler** (default 1%) prevents the OpenTelemetry collector from being overwhelmed in production, while ensuring sufficient coverage for latency debugging.

**Available in:** Go (`telemetry.go`), Rust (`telemetry.rs`), Python (`telemetry.py`), Node.js (`telemetry.ts`)

---

## Circuit Breaker, Retry & P99 Hedging
Three resilience primitives work together to guarantee extreme availability:

1. **`CircuitBreaker`** — atomic FSM (Closed → Open → HalfOpen) that fast-fails requests when error rates exceed a threshold, preventing cascading failures across the cluster.
2. **Exponential Backoff with Full Jitter** — avoids thundering-herd retries. Sleep duration is `rand[0, backoff)` per the AWS recommendation.
3. **P99 Hedging with `TokenBucket`** — if a request doesn't complete within the P99 latency threshold, a duplicate is fired to a different backend. The token bucket limits hedging to e.g. 10 duplicates/second, preventing hedge-induced amplification during cluster-wide slowdowns.

**Available in:** Go, Rust, Python, Node.js

---

## Advanced Load Balancing
- **Round-Robin**: Thread-safe atomic counter; available in Go, Rust, Python, and Node.js.
- **Least-Connections (`LeastConnBalancer`)**: Lock-free in Go (atomic pointer swaps + cache-line padding); `RwLock`-based in Rust. Dynamically routes new requests to the backend with the fewest in-flight connections — ideal for AI inference where request latency is highly variable.

**Available in:** Go, Rust, Python, Node.js, C++, Java

---

## Multi-Protocol Multiplexing (Go)
Unlike frameworks that force you to choose a protocol, Helix RPC multiplexes **gRPC, REST, Apache Thrift, and SSE** on the exact same port using `h2c` (HTTP/2 cleartext):

- **gRPC**: First-class Protobuf framing.
- **Thrift**: Deep native support for Apache Thrift binary protocol.
- **REST/JSON**: Standard HTTP/1.1 with automatic JSON unmarshaling and path parameter extraction.
- **Server-Sent Events (SSE)**: Built-in `text/event-stream` streaming.

**Available in:** Go, C++, Java (Sniffing & Multiplexing core)

---

## Prometheus Metrics
Helix RPC runtimes gather and expose performance metrics at `/metrics` and `/__helix/metrics` in the standard Prometheus text exposition format:
- `helix_requests_total`: Total number of RPC requests labeled by method, path, and HTTP status.
- `helix_errors_total`: Total number of RPC errors labeled by method and path.
- `helix_request_duration_seconds`: Histogram of request latencies.
- `helix_backend_active_connections`: Current active connections to each backend target.

**Available in:** Go, Rust, Python, Node.js

---

## Developer Dashboard
Go services embed a single-page reactive dashboard at `/__helix/ui`. It offers:
- Real-time visualization of circuit breaker states (CLOSED, OPEN, HALF-OPEN).
- Average latency, error rate, and request counters per RPC method.
- Real-time active connection tracking for the backend server pool.
- **Chaos Testing Override Controls**: Live buttons to manually **Trip** (Force Open) or **Reset** (Force Close) the circuit breaker for verifying client-side failover logic.

**Available in:** Go (Dev UI)

---

## Schema Compatibility Engine
The `helix-gen diff <old.proto> <new.proto>` command uses a semantic compatibility engine to verify that schema updates don't break downstream clients:
- **`BACKWARD_COMPATIBLE` (Safe)**: Adding optional fields, new RPC services, new methods, or enum values.
- **`BREAKING` (Fatal)**: Deleting fields, changing field types, altering field tag IDs, renaming methods, changing streaming configurations, or making optional fields required.

**Available in:** Compiler CLI

---

## DNS Service Discovery
Helix supports pluggable service discovery interfaces. Out of the box, all runtimes provide:
- **`StaticResolver`**: A local registry mapping service names directly to static IP addresses.
- **`DNSResolver`**: Resolves target endpoints dynamically using standard A/AAAA record queries or port-aware SRV records—making it ready for Kubernetes cluster-internal service discovery.

**Available in:** Go, Rust, Python, Node.js

---

## High-Performance Engines & Allocators
To support extreme throughput requirements, Helix embeds customized high-performance parsers and memory systems:
- **JIT JSON Engine (Go)**: Uses JIT-compiled **ByteDance Sonic** with AVX/AVX2 instruction support for zero-reflection, hyper-fast JSON REST transcoding.
- **SIMD JSON Engine (Rust)**: Employs SIMD-vectorized **simd-json** to accelerate JSON deserialization/serialization on hot API paths.
- **Global Memory Allocator (Rust)**: Binds Microsoft's **mimalloc** thread-caching memory allocator to eliminate allocator lock contention in multi-threaded network tasks.

**Available in:** Go, Rust

