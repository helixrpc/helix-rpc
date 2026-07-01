# Setup & Configuration

Helix RPC is designed to be embedded directly into your application as a library, giving you total control over the server environment, routing, and deployment.

## 1. Installation

### Go Runtime
To install the Go runtime, run:
```bash
go get github.com/helixrpc/helix-rt
```

### Rust Runtime
To install the Rust runtime, run:
```bash
cargo add helix-rt
```
*Note: Because the Rust runtime embeds CPython natively, you must ensure that your host system has `python3-dev` or the equivalent Python development headers installed so that `pyo3` can compile.*

## 2. Server Configuration

### Port Binding
Both the Go and Rust runtimes allow you to explicitly bind to any network interface and port during initialization.

**Go:**
```go
// Binds to all network interfaces on port 8080
server := runtime.NewServer(":8080")
```

**Rust:**
```rust
// Binds to localhost on port 8081
let mut server = HelixServer::new("127.0.0.1:8081", handler, routes);
```

### Dynamic Batching Configuration (Go)
The `@batch` interceptor requires careful tuning based on your specific GPU hardware and AI model constraints.

```go
dispatcher := runtime.NewBatchScheduler(
    100, // Maximum Batch Size (e.g., maximum concurrent requests the GPU can handle in one array)
    50 * time.Millisecond, // Batch Window (How long to wait for more requests before dispatching)
    myBatchHandler,
)
```
*   **Max Batch Size:** If you set this too high, you risk Out-Of-Memory (OOM) errors on the GPU. If you set it too low, you leave GPU compute capacity on the table.
*   **Batch Window:** If you set this too high, single requests suffer high latency. If you set it too low, you lose the benefits of batching under light load.

### Thread Pooling (Rust)
Because the Rust runtime runs within the `tokio` asynchronous environment, it is highly recommended to configure Tokio for multi-threading to maximize throughput, especially when managing multiple concurrent blocking Python generators.

Ensure you start your Rust application with the `tokio` multi-thread flavor:
```rust
#[tokio::main(flavor = "multi_thread", worker_threads = 4)]
async fn main() {
    // ...
}
```

## 3. Python Environment Setup (Rust PyO3)

When using the Rust AI Gateway, the Rust binary will natively invoke the Python interpreter. It is crucial that the environment where you run your compiled Rust binary has access to the Python modules your model needs (e.g., `torch`, `transformers`).

The standard way to configure this is to run your Rust binary *inside* an activated Python virtual environment:

```bash
# 1. Create a virtual environment
python3 -m venv venv

# 2. Activate it
source venv/bin/activate

# 3. Install your AI dependencies
pip install torch transformers

# 4. Run your compiled Rust AI Gateway!
./target/release/rust-ai-gateway
```
Because the virtual environment is activated, `pyo3` will automatically locate your installed dependencies and seamlessly load them into the embedded interpreter!
