# Core Features

## Dynamic Batching (Go & Python)
AI models process batches of data much faster than individual requests. However, HTTP requests come in individually and asynchronously.

Helix RPC's `BatchScheduler` intercepts individual REST requests, holds them open for a tiny window (e.g., 50ms), collapses them into an array `[]interface{}`, and dispatches the array to the AI model. When the AI model finishes, the scheduler fans the results back out to the correct, waiting HTTP connections.
This single feature can increase your GPU throughput by up to 10,000% under high load.

## Zero-Serialization Engine (Rust)
JSON unmarshaling is incredibly CPU-intensive. Under heavy load, an API gateway can spend 60% of its CPU cycles just formatting JSON and Protobufs to talk to a Python backend.
Helix RPC completely bypasses the network stack between the Gateway and the Model. It uses Rust's `PyO3` to invoke Python C-bindings natively in-memory. 

## Native SSE Streaming
Chat UIs require real-time token streaming. Helix RPC implements native Server-Sent Events (SSE) across **all three runtimes** (Go, Rust, and Python). 
When a client requests `Accept: text/event-stream`, the gateway will automatically detect the stream, launch the generator, and asynchronously proxy the yields into standard `Server-Sent Events`.

## Production Middlewares
*   **mTLS:** Mutually authenticated TLS is supported out of the box.
*   **Health Checking:** Standard `grpc.health.v1` is automatically mounted, allowing orchestrators like Kubernetes to know if your GPU is healthy.
*   **Interceptors:** Full support for Unary Request Interception to allow you to inject logging, tracing, and metric collection effortlessly.
*   **Deadline Propagation (`grpc-timeout`):** The gateways automatically extract `grpc-timeout` headers, converting them into native `context.Context` deadlines (Go), `tokio::time::timeout` limits (Rust), or `asyncio.wait_for` (Python). If the AI model hangs, the gateway will flawlessly cancel the request and free the resources.
*   **Per-Message Compression:** Out-of-the-box support for `gzip` compression across Go, Rust, and Python runtimes. If a client sends a `grpc-encoding: gzip` header, Helix RPC natively decompresses the payload, routes it to the AI, and compresses the response back over the wire.

## Multi-Protocol Multiplexing
Unlike standard frameworks that force you to choose between gRPC, REST, or GraphQL, Helix RPC is a **true multi-protocol gateway**. 

Using `golang.org/x/net/http2/h2c`, the Go server multiplexes multiple protocols on the exact same port:
*   **gRPC**: First-class support with Protobuf framing.
*   **Thrift**: Deep native support for the Apache Thrift protocol.
*   **REST/JSON**: Standard HTTP/1.1 REST endpoints with automatic JSON unmarshaling.
*   **Server-Sent Events (SSE)**: Built-in support for streaming real-time JSON chunks (`text/event-stream`).
