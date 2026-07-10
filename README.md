<p align="center">
  <img src="docs/assets/logo.png" alt="Helix RPC Logo" width="160"/>
</p>

<h1 align="center">Helix RPC 🧬</h1>

<p align="center">
  <strong>The Unified Multi-Protocol Meta-Framework for Modern Microservices and AI Inference.</strong><br/>
  Seamlessly unifying gRPC, Thrift, JSON/REST, WebSockets, and FlatBuffers under a single high-performance runtime.
</p>

<p align="center">
  <a href="https://github.com/helixrpc/helix-rpc/actions/workflows/ci-integration.yml">
    <img src="https://github.com/helixrpc/helix-rpc/actions/workflows/ci-integration.yml/badge.svg" alt="CI"/>
  </a>
  <a href="https://github.com/helixrpc/helix-rpc/actions/workflows/docs.yml">
    <img src="https://github.com/helixrpc/helix-rpc/actions/workflows/docs.yml/badge.svg" alt="Docs"/>
  </a>
  <img src="https://img.shields.io/badge/go-1.23+-00ADD8?logo=go&logoColor=white" alt="Go"/>
  <img src="https://img.shields.io/badge/rust-stable-orange?logo=rust&logoColor=white" alt="Rust"/>
  <img src="https://img.shields.io/badge/python-3.12+-3776AB?logo=python&logoColor=white" alt="Python"/>
  <img src="https://img.shields.io/badge/license-MIT-green" alt="MIT License"/>
</p>

---

## 🧬 What is Helix RPC?

Helix RPC is not a new competing transport protocol. Instead, it is a **unified multi-protocol meta-framework** designed to run on top of your existing schemas and systems. 

By utilizing a **Same-Port Multiplexer** and **Zero-Allocation Transpilers**, Helix sniffs, routes, and transcodes incoming traffic dynamically, letting them coexist seamlessly in your cluster.

### Why Helix?
- **Unified Multi-Protocol Server**: Accept gRPC, Thrift, JSON/REST, Bidirectional WebSockets, and zero-copy FlatBuffers on a **single TCP port**.
- **Zero-Downtime Migration**: Maintain legacy clients and migrate backend services gradually without writing translators or deploying sidecar proxies.
- **AI-Native Optimization**: Pass massive multi-gigabyte ML Tensors using the `application/grpc+flatbuffers` codec directly into numpy/PyTorch memory views without deserialization.
- **Service Mesh Ready**: First-class Envoy Wasm Filter support and automatic OpenTelemetry TraceContext propagation.

## 🚀 Performance & Packages

Helix RPC has been benchmarked against industry standards for HTTP/1.1 JSON REST workloads under high concurrency (100 concurrent connections):
- **16.8× faster than FastAPI** (130,993 req/sec vs 7,811 req/sec)
- **Matches raw Go `net/http` within 2%** — proving our multi-protocol sniffing and transcoding layers add essentially zero overhead.

Read the full reproducible [Performance Benchmarks](docs/benchmarks.md).

**Install the Runtime Packages:**
```bash
# Rust
cargo add helix-rt

# Python
pip install helix-rt
```

---

## ⚡ Key Features

| Feature | Description |
|---|---|
| **Same-Port Sniffer** | Sniffs packets to route gRPC, Thrift, HTTP/REST, gRPC-Web, WebSockets, and SSE on one port |
| **Zero-Copy FlatBuffers** | `application/x-flatbuffers` routes skip deserialization completely for huge ML tensor payloads |
| **Zero-Allocation Transpiling**| Directly translates Protobuf binary to Thrift Compact in-memory |
| **OpenAPI Generation** | Automatically generates `openapi.json` from your `.proto` / `.thrift` files |
| **Java Interoperability** | Generates Zero-Dependency Java Stubs that implement `TBase` and `MessageLite` out of the box |
| **Dynamic Batching** | Coalesces highly concurrent individual requests into optimal batches for GPU/LLM backends |
| **Service Mesh Wasm Filter** | Envoy Proxy WebAssembly plugin that normalizes multi-protocol telemetry metrics |
| **Single-File Scaffolding** | `helix-gen init` generates schema, stubs, configs, containers, and deployment plans |

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
```
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

## 🏗 Architectural Blueprint

```
                     Client Traffic
                            │
  ┌─────────────────────────┼────────────────────────┐
  ▼ (gRPC/HTTP2)            ▼ (Thrift Compact)       ▼ (HTTP/JSON REST/WS)
┌─────────────────────────────────────────────────────────────┐
│                 Helix Same-Port Sniffing Server             │
├─────────────────────────────────────────────────────────────┤
│                     Direct Transpilation                    │
│             (Zero-Allocation Protobuf ↔ Thrift)             │
├─────────────────────────────────────────────────────────────┤
│                 Zero-Copy Codec / Memory View               │
│                (FlatBuffers Tensor Extraction)              │
└───────────────────────────┬─────────────────────────────────┘
                            ▼
                  ┌───────────────────┐
                  │   Your Handler    │
                  └───────────────────┘
```

---

## 🤝 Contributing & License

We welcome contributions to the compiler and multi-language runtimes. Please check out our [Contributing Guide](CONTRIBUTING.md).

Distributed under the MIT License. © Helix RPC Contributors.
