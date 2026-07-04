# Setup & Configuration

Helix RPC is designed to be embedded directly into your application as a library, giving you total control over the server environment, routing, and deployment.

## System Requirements

Before getting started, ensure your development environment matches the minimum language runtime requirements:

| Language | Minimum Version | Recommended / Verified | Package Manager |
|:---|:---:|:---:|:---|
| **Go** | `1.25.0` | `1.25.0` | Go Modules (`go get`) |
| **Rust** | Rust Edition `2021` | Rust `1.75.0` or newer | Cargo (`cargo add`) |
| **Python** | `3.10` | `3.10` - `3.13` | Pip (`pip install`) |
| **Node.js** | `18.0.0` | `20.0.0` or newer | NPM (`npm install`) |

---

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

## 5. Command-Line Interface (helix-gen)

The `helix-gen` CLI manages scaffolding, code generation, and compatibility checks.

### Scaffolding a New Service (`init`)
Creates a complete boilerplate directory structure, configuration file, and Makefile for a new service.

```bash
helix-gen init <service-name> [flags]
```

**Flags:**
- `--lang <go|rust|python>`: Primary service language (default: `go`).
- `--disable-metrics`: Disable Prometheus metrics reporting.
- `--disable-health`: Disable standard gRPC health checks.
- `--disable-gzip`: Disable default response gzip compression.
- `--disable-deadline`: Disable automatic deadline propagation.

**Example:**
```bash
# Scaffold a python service with metrics and health checks disabled
helix-gen init my-model-service --lang python --disable-metrics --disable-health
```

### Code Generation (`generate`)
Compiles a Protobuf (`.proto`) or Apache Thrift (`.thrift`) IDL file into statically typed client and server stubs.

```bash
helix-gen generate -idl <schema-path> -lang <go|rust|python> -out <output-path> [flags]
```

**Flags:**
- `--watch`: Watch the IDL file and automatically regenerate code on save.

**Example:**
```bash
helix-gen generate -idl schema.proto -lang go -out generated/generated.go --watch
```

### Schema Compatibility Checking (`diff`)
Validates that changes between two schema definitions do not introduce breaking client modifications.

```bash
helix-gen diff <old-schema> <new-schema>
```

Returns exit code `2` if breaking modifications are detected, allowing easy integration with CI/CD gates.

## 6. Secure & High-Performance Containers

Helix RPC provides optimized packaging blueprints for deployment into production clouds, microVM environments, and bare-metal environments.

### Custom Docker Scratch Containers (`containers/`)
These containers use multi-stage builds to compile statically linked binaries, running them on top of a bare `scratch` image (approx. 10MB) for maximum security and minimal latency.

1. **Build & Run via Docker Compose:**
   ```bash
   docker-compose -f containers/docker-compose.yaml up --build
   ```
   This automatically:
   - Configures optimized Linux TCP/IP network kernel parameters (sysctls).
   - Mounts the host's `/dev/shm` shared memory partition inside the container to enable zero-copy POSIX shared-memory IPC.

### Firecracker microVM Packaging (`firecracker/`)
For multi-tenant systems requiring secure hardware-level isolation with fast spin-up times, you can compile and package the Helix service into a Firecracker microVM running directly as PID 1 (init) on the Linux kernel.

1. **Build the RootFS Image:**
   ```bash
   # Compiles Go/Rust service statically and builds a minimal ext4 rootfs disk
   ./firecracker/setup_microvm.sh <service-directory> [go|rust]
   ```
2. **Download Kernel & Launch VM:**
   ```bash
   # Download KVM-compatible vmlinux kernel
   curl -fsSL -o firecracker/vmlinux https://s3.amazonaws.com/spec.ccfc.min/firecracker-kernels/vmlinux-5.10.0

   # Run Firecracker microVM (boots in under 5ms)
   firecracker --config-file firecracker/config.json
   ```



