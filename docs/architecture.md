# Technical Architecture

Helix RPC employs a **Polyglot Meta-Framework Architecture** leveraging the unique strengths of **six distinct runtimes** (Go, Rust, Python, Node.js, C++, and Java) to deliver a seamless, high-performance API ecosystem.

Instead of forcing a "one size fits all" language constraint, Helix groups runtimes into two primary tiers based on their optimal use cases in the AI and microservices stack.

---

## 1. The Gateway / Routing Tier (Go, Node.js, Java)

These runtimes are optimized for **horizontal scaling, fan-in/fan-out networking, and massive IO multiplexing**. They typically act as the front-door proxy or API gateway.

### Go (`helix-rt`)
The Go runtime is the king of concurrency. It uses `golang.org/x/net/http2/h2c` to seamlessly multiplex gRPC HTTP/2 calls alongside standard REST HTTP/1.1 calls on the exact same port. It heavily utilizes Interceptors to perform dynamic request batching, deadline propagation, health checking, and payload compression.

### Node.js (`runtime-node`)
The Node.js runtime leverages the V8 asynchronous event loop to provide an ultra-lightweight, zero-dependency sniffing server. It is perfect for serverless edge deployments or wrapping external APIs where JavaScript ecosystem integration is paramount.

### Java (`runtimes/java`)
The Java runtime leverages NIO ByteBuffers to build zero-allocation connection sniffers. It provides high-throughput multiplexing inside enterprise JVM environments, allowing legacy Spring/Tomcat services to migrate to Helix seamlessly.

---

## 2. The Inference / Compute Tier (Rust, Python, C++)

These runtimes are optimized for **vertical scaling, memory-safety, zero-copy tensors, and deep kernel integration**. They typically run directly alongside (or embed) the AI models.

### Rust (`helix_rt`)
The Rust runtime is optimized for extreme latency and safety. Using the `pyo3` crate, the Rust binary **embeds the CPython interpreter directly inside itself**.
When an HTTP request comes in, Rust passes the memory pointer of the prompt string directly to the Python function `generate_stream()`. There is **zero network hop and zero serialization overhead**.

### Python (`runtime-python`)
The standalone Python runtime is built on `asyncio` and is explicitly designed for seamless integration with PyTorch and vLLM. It features a native `BatchScheduler` that transparently aggregates concurrent HTTP requests into vectorized GPU batches without any extra proxy infrastructure.

### C++ (`runtimes/cpp`)
The C++ runtime provides absolute bare-metal control. It is distributed as a high-performance header-only library (`INTERFACE` in CMake) offering Zero-Copy Sniffing, QUIC Transport, Consistent Hash Balancing, and SSE Streaming. It is ideal for ultra-low latency model serving wrappers and custom tensor inference engines.

---

## Core Primitives Across Tiers

To bind these six runtimes into a unified mesh, Helix RPC implements shared architectural primitives:

1. **Same-Port Multiplexing**: Every runtime features a protocol sniffer that inspects the first few bytes of a TCP stream. This allows gRPC, REST, Apache Thrift, and SSE to all coexist on a single port (e.g., `8080`) across every language.
2. **eBPF Kernel Bypass**: When a Gateway (e.g., Go) and a Compute node (e.g., Python) are co-located on the same machine, the Helix eBPF Sockmap bypasses the TCP/IP stack entirely, routing packets directly through shared memory.
3. **Zero-Copy Views**: Memory-slicing encoders/decoders in all runtimes consume up to 70% less memory by avoiding heap allocations during serialization.
4. **SSE Streaming via MPSC / Queues**: For chat streams, the gateways utilize non-blocking MPSC channels (Rust/Go) or async queues (Python/Node) to transcode raw string tokens into OpenAI-formatted JSON chunks (`data: {"choices":[{"delta":{"content":"..."}}]}`) over Server-Sent Events without blocking the main event loop.
