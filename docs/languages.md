# Language Support

Helix RPC is designed around a multi-language philosophy, recognising that different languages excel at different parts of the AI stack. All **six runtimes** (Go, Rust, Python, Node.js, C++, and Java) now share **100% feature parity** across their respective core feature subsets.

---

## Feature Parity Matrix

| Feature | Go | Rust | Python | Node.js | C++ | Java |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| Dynamic Batching | ✅ | ✅ | ✅ | ✅ | — | — |
| Graceful Shutdown | ✅ | ✅ | ✅ | ✅ | — | — |
| gRPC / HTTP/2 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| REST / JSON Transcoding | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Server-Sent Events (SSE) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Deadline Propagation (`grpc-timeout`) | ✅ | ✅ | ✅ (all 6 units) | ✅ | ✅ | ✅ |
| Per-Message Compression (`gzip`) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Health Checking (`grpc.health.v1`) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| mTLS Transport Security | ✅ | ✅ | — | — | — | — |
| OpenTelemetry Tracing | ✅ | ✅ | ✅ | ✅ | — | — |
| Probabilistic Trace Sampling | ✅ | ✅ | ✅ | ✅ | — | — |
| Circuit Breaker | ✅ | ✅ | ✅ | ✅ | — | — |
| Exponential Backoff Retry | ✅ | ✅ | ✅ | ✅ | — | — |
| P99 Hedging (with Token Bucket) | ✅ | ✅ | ✅ | ✅ | — | — |
| Least-Connections Load Balancing | ✅ | ✅ | — | — | — | — |
| Round-Robin Load Balancing | ✅ | ✅ | — | — | — | — |
| Structured Errors (`HelixError`) | ✅ | ✅ | ✅ | ✅ | — | — |
| Shared Memory Transport (SHM) | ✅ | ✅ | — | — | — | — |
| PyO3 Zero-Serialization Embedding | — | ✅ | — | — | — | — |
| Thrift Protocol Support | ✅ | ✅ | — | — | — | — |
| Code Generation | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

> **Note:** `—` means the feature is not applicable to that runtime's role (e.g. SHM is not relevant to Python/Node.js as a standalone server; PyO3 is a Rust-only capability).

---

## Go (`runtimes/go`)

Go is the king of highly concurrent, networked routing. We use Go for the **Gateway layer** where horizontal scaling and request fan-in/fan-out are paramount.

**Key Strengths:**
- **Lock-Free `LeastConnBalancer`** with cache-line padding eliminates mutex contention at 10k+ RPS.
- **Multi-Protocol Multiplexing** on a single port: gRPC, REST, Apache Thrift, SSE.
- **Full Interceptor Chain** with `UnaryServerInterceptor` hooks for logging, auth, and tracing.
- **Circuit Breaker + Hedging** to prevent cascading failures under tail latency.

*Use Go when building high-throughput API Gateways or multi-protocol edge proxies.*

---

## Rust (`runtimes/rust`)

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

---

## Node.js (`runtime-node`)

Node.js is the king of asynchronous, lightweight backend APIs. Node.js is fully supported as a **first-class standalone runtime** for building microservices, AI orchestrators, or middleware wrappers.

**Key Strengths:**
- **Zero-Dependency Sniffing Server** — Implements an efficient connection-sniffing engine mapping TCP/HTTP/gRPC/FlatBuffers requests on a single port.
- **Dynamic BatchScheduler** — Transparently batches parallel incoming requests into unified callback runs.
- **Built-in JWT & API Key Middlewares** — Standard cryptographic authorization helpers native in TypeScript.
- **Resilience Helpers** — Token Bucket rate limiter and Exponential Backoff retry policies.

*Use Node.js when writing serverless APIs or microservice routers inside the JavaScript/TypeScript ecosystem.*

---

## C++ (`runtimes/cpp`)

C++ is the ultimate language for performance-critical systems, system integration, and low-latency computing.

**Key Strengths:**
- **Zero-Copy Sniffing** — High-performance TCP parser inspecting bytes directly from input stream.
- **ConsistentHashBalancer** — Virtual node ring for distributed KVCache prompt routing.
- **MultiplexedServer** — Same-port connection multiplexing and routing natively integrated with Windows (Winsock2) and Unix.
- **SSE Streaming** — Chunk-writer utilities supporting `text/event-stream` format.
- **Tensor Integration** — Zero-copy memory boundaries for direct ML model serving wrappers.

*Use C++ when writing latency-critical microservices or model serving wrappers.*

---

## Java (`runtimes/java`)

Java is the bedrock of enterprise backends and big-data streaming.

**Key Strengths:**
- **ByteBuffer Sniffing** — Stream-sniffing utilizing zero-allocation ByteBuffers.
- **ConsistentHashBalancer** — High-efficiency thread-safe virtual node ring using `java.util.TreeMap`.
- **MultiplexedServer** — Non-blocking multi-protocol single-port server loop via modern Java NIO.
- **Async Sinks** — Deep integration with enterprise queues via `KafkaAsyncSink` and `RabbitMQAsyncSink`.
- **SSE Streaming** — Streaming chunk-writing helpers for generative response streams.

*Use Java when deploying Helix RPC inside enterprise JVM environments.*
