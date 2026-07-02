# Language Support

Helix RPC is designed around a multi-language philosophy, recognising that different languages excel at different parts of the AI stack. All three runtimes now share **100% feature parity** across the core production feature set.

---

## Feature Parity Matrix

| Feature | Go | Rust | Python |
|---|:---:|:---:|:---:|
| Dynamic Batching | ✅ | ✅ | ✅ |
| Graceful Shutdown | ✅ | ✅ | ✅ |
| gRPC / HTTP/2 | ✅ | ✅ | ✅ |
| REST / JSON Transcoding | ✅ | ✅ | ✅ |
| Server-Sent Events (SSE) | ✅ | ✅ | ✅ |
| Deadline Propagation (`grpc-timeout`) | ✅ | ✅ | ✅ (all 6 units) |
| Per-Message Compression (`gzip`) | ✅ | ✅ | ✅ |
| Health Checking (`grpc.health.v1`) | ✅ | ✅ | — |
| mTLS Transport Security | ✅ | ✅ | — |
| OpenTelemetry Tracing | ✅ | ✅ | ✅ |
| Probabilistic Trace Sampling | ✅ | ✅ | ✅ |
| Circuit Breaker | ✅ | ✅ | ✅ |
| Exponential Backoff Retry | ✅ | ✅ | ✅ |
| P99 Hedging (with Token Bucket) | ✅ | ✅ | ✅ |
| Least-Connections Load Balancing | ✅ | ✅ | — |
| Round-Robin Load Balancing | ✅ | ✅ | — |
| Structured Errors (`HelixError`) | ✅ | ✅ | ✅ |
| Shared Memory Transport (SHM) | ✅ | ✅ | — |
| PyO3 Zero-Serialization Embedding | — | ✅ | — |
| Thrift Protocol Support | ✅ | — | — |
| Code Generation | ✅ | — | ✅ |

> **Note:** `—` means the feature is not applicable to that runtime's role (e.g. SHM is not relevant to Python as a standalone server; PyO3 is a Rust-only capability).

---

## Go (`runtime-go`)

Go is the king of highly concurrent, networked routing. We use Go for the **Gateway layer** where horizontal scaling and request fan-in/fan-out are paramount.

**Key Strengths:**
- **Lock-Free `LeastConnBalancer`** with cache-line padding eliminates mutex contention at 10k+ RPS.
- **Multi-Protocol Multiplexing** on a single port: gRPC, REST, Apache Thrift, SSE.
- **Full Interceptor Chain** with `UnaryServerInterceptor` hooks for logging, auth, and tracing.
- **Circuit Breaker + Hedging** to prevent cascading failures under tail latency.

*Use Go when building high-throughput API Gateways or multi-protocol edge proxies.*

---

## Rust (`runtime-rust`)

Rust is the king of memory safety and zero-cost abstractions. We use Rust where deep system integration and absolute raw compute speed are paramount.

**Key Strengths:**
- **PyO3 Zero-Serialization Python Embedding** — the Rust gateway calls Python model functions directly in-process with zero network hops and zero JSON overhead.
- **`tokio`-native async I/O** with HTTP/2 multiplexing via `hyper`.
- **Atomic `CircuitBreaker` + `TokenBucket`** for production-hardened retry/hedging.
- **`BatchScheduler`** that aggregates concurrent requests into a single GPU dispatch window.

*Use Rust when you need absolute minimum inference latency by co-locating the gateway with the AI model in the same memory space.*

---

## Python (`runtime-python`)

Python is the king of AI modeling (PyTorch, Transformers, vLLM). While Helix RPC natively embeds Python into the Rust runtime via PyO3, Python is also fully supported as a **first-class standalone language**.

**Key Strengths:**
- **`asyncio` Dynamic BatchScheduler** — transparently aggregates concurrent HTTP requests into vectorised GPU batches without any extra infrastructure.
- **Graceful Shutdown** — SIGTERM/SIGINT handling with configurable drain period, matching the Go and Rust gateway behaviour.
- **`HelixError` + `ErrorCode`** — structured gRPC status codes automatically converted to HTTP status in the handler.
- **`CircuitBreaker`, `RetryPolicy`, `TokenBucket`** — full Python port of the Go/Rust resilience primitives via `helix_rt.retry`.
- **Probabilistic OpenTelemetry Sampling** — 1% default sampling rate prevents collector overload in production.

*Use Python when writing a pure-Python model server that still needs enterprise-grade batching, retries, and observability.*
