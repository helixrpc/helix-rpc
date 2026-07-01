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

## Python (`runtime-python`)
Python is the king of AI modeling (PyTorch, Transformers, vLLM). While Helix RPC natively embeds Python into the Rust runtime via PyO3, Python is also fully supported as a **first-class language**.

**Supported Features:**
*   Code Generation (`helix-gen -lang python` generates `@dataclass` and Service ABCs)
*   Native Dynamic Batching (via `asyncio` BatchScheduler)
*   Native Server-Sent Events (SSE) Streaming
*   Middlewares, Deadline Propagation (`grpc-timeout`), and Gzip Compression

*Use Python when you want to write your entire server using pure Python (`aiohttp` or FastAPI) but still need the high-performance AI Dynamic Batching algorithms that Helix RPC provides.*
