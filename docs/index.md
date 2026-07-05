<p align="center">
  <img src="assets/logo.png" alt="Helix RPC Logo" width="160"/>
</p>

<h1 align="center">Helix RPC 🧬</h1>

<p align="center">
  <strong>The Unified Multi-Protocol Meta-Framework for Modern Microservices and AI Inference.</strong><br/>
  Seamlessly unifying gRPC, Thrift, and JSON/REST under a single high-performance runtime.
</p>

---

## 🧬 What is Helix RPC?

Helix RPC is not a new competing transport protocol. Instead, it is a **unified multi-protocol meta-framework** designed to run on top of your existing schemas and systems. 

By utilizing a **Same-Port Multiplexer** and **Zero-Allocation Transpilers**, Helix sniffs, routes, and transcodes incoming traffic (gRPC over HTTP/2, legacy Apache Thrift compact/binary, or HTTP/JSON REST) dynamically, letting them coexist seamlessly in your cluster.

### Why Helix?
- **Unified Multi-Protocol Server**: Accept gRPC, Thrift, HTTP/JSON REST, gRPC-Web, and SSE on a single TCP port.
- **Zero-Downtime Migration**: Maintain legacy clients and migrate backend services gradually without writing translators or deploying sidecar proxies.
- **Direct Kernel Bypass**: Automatically bypasses the TCP/IP stack using **eBPF Sockmaps** for co-located microservices on loopback.
- **Zero-Copy Views**: Memory-slicing encoders/decoders in Go, Rust, Node.js, and Python consume up to 70% less memory under high throughput.

---

## 🚀 Performance & Packages

Helix RPC has been benchmarked against industry standards for HTTP/1.1 JSON REST workloads under high concurrency (100 concurrent connections):
- **16.8× faster than FastAPI** (130,993 req/sec vs 7,811 req/sec)
- **Matches raw Go `net/http` within 2%** — proving our multi-protocol sniffing and transcoding layers add essentially zero overhead.

Read the full reproducible [Performance Benchmarks](benchmarks.md).

**Install the Runtime Packages:**
```bash
# Rust
cargo add helix-rt

# Python
pip install helix-rt
pip install "helix-rt[tensor]" # For zero-copy numpy support
```

---

## ⚡ Key Features

| Feature | Description |
|---|---|
| **Same-Port Sniffer** | Sniffs incoming packets to route gRPC, Thrift, HTTP/REST, gRPC-Web, and SSE on one port |
| **Zero-Copy View** | Memory-slicing parser views avoid heap copies during serialization |
| **Zero-Allocation Transpiling**| Directly translates Protobuf binary to Thrift Compact in-memory |
| **Dynamic Batching** | Coalesces highly concurrent individual requests into optimal batches for GPU/LLM backends |
| **eBPF Kernel Bypass** | Kernel-level Sockmap redirection for local loopback routing |
| **Single-File Scaffolding** | `helix-gen init` generates schema, stubs, configs, containers, and deployment plans |
| **Hot-Reload Config** | `helix.json` watched and hot-reloaded dynamically for auth, rate limits, and compression |

---

## ⚡ Quick Start

### 1. Install the CLI tool
```bash
go install github.com/helixrpc/helix-rpc/compiler/helix-gen@latest
```

### 2. Scaffold a new unified service
```bash
helix-gen init my-service --lang go
cd my-service
```

This sets up:
```text
my-service/
├── schema.proto          # Unified IDL schema
├── server/main.go        # Server setup and business handlers
├── generated/            # Sniffers and multi-protocol stubs
├── helix.json            # Dynamic hot-reload configuration file
└── Makefile              # Commands for local build, test, and container run
```

### 3. Write your handler (the only code you write)
```go
// server/main.go
server.RegisterMethod("/v1.ModelService/Predict", runtime.MethodInfo{
    Handler: func(ctx context.Context, req interface{}) (interface{}, error) {
        // Core business logic goes here
        return &PredictResponse{Completion: "Processed by Helix Unified Server!"}, nil
    },
})
```

---

## 📦 Dynamic Configuration (`helix.json`)

Tweak system behaviors at runtime without restarting your instances:

```jsonc
{
  "host": "0.0.0.0",
  "port": 8080,
  "disable_gzip": false,        // Per-message compression toggle
  "disable_deadline": false,    // Timeout propagation toggle
  "rate_limit_rate": 200.0,     // Token-bucket refill rate
  "rate_limit_burst": 20,
  "enable_jwt_auth": true,       // Live JWT authentication
  "jwt_secret": "my-secret-key"
}
```
