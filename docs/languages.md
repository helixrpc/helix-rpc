# Language Support

Helix RPC is designed around a multi-language philosophy, recognizing that different languages excel at different parts of the AI stack.

## Go (`runtime-go`)
Go is the king of highly concurrent, networked routing. We use Go for the Gateway layer where horizontal scaling and request fan-in/fan-out are paramount.

**Supported Features:**
*   Dynamic Batching (via `@batch` scheduling algorithm)
*   HTTP/2 and gRPC Multiplexing
*   Middlewares (Interceptors, Health Checking, Timeout Propagation, mTLS)

*Use Go when you are building a fleet of load balancers or highly concurrent API Gateways.*

## Rust (`runtime-rust`)
Rust is the king of memory safety and zero-cost abstractions. We use Rust where deep system integration and raw compute speed are paramount.

**Supported Features:**
*   PyO3 Zero-Serialization Python Embedding
*   Server-Sent Events (SSE) Native Streaming
*   High-Performance Tokio Asynchronous IO

*Use Rust when you want absolute minimum latency by running the AI model in the exact same memory space as your API Gateway.*

## Python
Python is the king of AI modeling (PyTorch, Transformers, vLLM). Helix RPC does not attempt to replace Python. Instead, it embeds Python directly into the Rust runtime, giving you the best of both worlds: Rust's networking speed and Python's AI ecosystem.
