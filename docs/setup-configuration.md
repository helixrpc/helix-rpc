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

### Python Runtime
To install the pure-Python runtime, run:
```bash
pip install helix-rt
```

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

### Dynamic Batching Configuration
The `@batch` scheduling algorithm requires careful tuning based on your specific GPU hardware and AI model constraints.

**Go:**
```go
dispatcher := runtime.NewBatchScheduler(
    100, // Maximum Batch Size
    50 * time.Millisecond, // Batch Window
    myBatchHandler,
)
```

**Python:**
```python
from helix_rt.batching import BatchScheduler

scheduler = BatchScheduler(
    100, # Maximum Batch Size
    50,  # Batch Window in milliseconds
    my_batch_handler,
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

## 4. File-Based Configuration (helix.json)

Helix supports configuring all operations via a central JSON file named `helix.json`. This file is generated automatically during project scaffolding (`helix-gen init`).

### Configuration Schema
```json
{
  "host": "127.0.0.1",
  "port": 8080,
  "disable_metrics": false,
  "disable_health": false,
  "disable_gzip": false,
  "disable_deadline": false,
  "rate_limit_rate": 100.0,
  "rate_limit_burst": 10
}
```

### Dynamic Hot-Reloading
All runtimes support watching the `helix.json` file for changes and dynamically hot-reloading configurables on the fly without needing to restart the process.

- **Go**: `runtime.WatchConfig("helix.json", callback)`
- **Rust**: `watch_config("helix.json".to_string(), callback)`
- **Python**: `watch_config("helix.json", callback)`

