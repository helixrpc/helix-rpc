<p align="center">
  <img src="docs/assets/logo.png" alt="Helix RPC Logo" width="160"/>
</p>

<h1 align="center">Helix RPC 🧬</h1>

<p align="center">
  <strong>The Unified Multi-Protocol Meta-Framework for Modern Microservices and AI Inference.</strong><br/>
  Seamlessly unifying gRPC, Thrift, and JSON/REST under a single high-performance runtime.
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

By utilizing a **Same-Port Multiplexer** and **Zero-Allocation Transpilers**, Helix sniffs, routes, and transcodes incoming traffic (gRPC over HTTP/2, legacy Apache Thrift compact/binary, or HTTP/JSON REST) dynamically, letting them coexist seamlessly in your cluster.

### Why Helix?
- **Unified Multi-Protocol Server**: Accept gRPC, Thrift, and JSON/REST on a single TCP port.
- **Zero-Downtime Migration**: Maintain legacy clients and migrate backend services gradually without writing translators or deploying sidecar proxies.
- **Direct Kernel Bypass**: Automatically bypasses the TCP/IP stack using **eBPF Sockmaps** for co-located microservices on loopback.
- **Zero-Copy Views**: Memory-slicing encoders/decoders in Go, Rust, Node.js, and Python consume up to 70% less memory under high throughput.

---

## 🚀 Key Features

| Feature | Description |
|---|---|
| **Same-Port Sniffer** | Sniffs incoming packets to route gRPC, Thrift, and HTTP/REST on one port |
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
  ┌────────────────────────┼────────────────────────┐
  ▼ (gRPC/HTTP2)           ▼ (Thrift Compact)       ▼ (HTTP/JSON REST)
┌─────────────────────────────────────────────────────────────┐
│                 Helix Same-Port Sniffing Server             │
├─────────────────────────────────────────────────────────────┤
│                     Direct Transpilation                    │
│             (Zero-Allocation Protobuf ↔ Thrift)             │
├─────────────────────────────────────────────────────────────┤
│                   eBPF Sockmap Redirect                     │
│                (Kernel loopback bypass)                     │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
                 ┌───────────────────┐
                 │   Your Handler    │
                 └───────────────────┘
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

---

## 🤝 Contributing & License

We welcome contributions to the compiler and multi-language runtimes. Please check out our [Contributing Guide](CONTRIBUTING.md).

Distributed under the MIT License. © Helix RPC Contributors.
