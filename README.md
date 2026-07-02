<p align="center">
  <img src="docs/assets/logo.png" alt="Helix RPC Logo" width="160"/>
</p>

<h1 align="center">Helix RPC 🧬</h1>

<p align="center">
  <strong>The next-generation AI infrastructure framework built for maximum efficiency.</strong><br/>
  Zero-serialization. Multi-protocol. Zero-configuration.
</p>

<p align="center">
  <a href="https://github.com/helixrpc/helix-rpc/actions/workflows/ci.yml">
    <img src="https://github.com/helixrpc/helix-rpc/actions/workflows/ci.yml/badge.svg" alt="CI"/>
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

## 🎯 Focus on Business Logic, Not Plumbing

Helix RPC is architected so developers spend **zero time** on network stacks, deployment pipelines, TLS, observability, or database connection tuning.

Run `helix-gen init` and get a **fully production-grade**, highly available microservice — with structured logging, Prometheus metrics, health checks, gzip compression, JWT auth, and Kubernetes manifests all pre-configured.

**Your only task is to write the core business logic handler.**

---

## 🚀 Key Features

| Feature | Description |
|---|---|
| **Zero-Serialization PyO3** | Embeds CPython directly in the Rust gateway — tensors pass over FFI, never JSON |
| **Dynamic Batching** | `@batch` interceptor coalesces 100 concurrent requests → 1 GPU call |
| **Multi-Protocol Gateway** | gRPC · Thrift Binary/Compact · HTTP/JSON REST · SSE — one TCP port |
| **Single-File Scaffolding** | `helix-gen init` generates schema, stubs, Dockerfile, Terraform & `helix.json` |
| **Hot-Reload Config** | `helix.json` watched at runtime — auth, rate limits, features toggled without restart |
| **Secure Containers** | Statically-linked `scratch` Docker + Firecracker microVM blueprints included |
| **Service Discovery** | Built-in DNS/SRV, Consul, and Kubernetes resolvers |
| **Cloud Deployment** | One-command Terraform for AWS ECS · GCP Cloud Run · Azure Container Apps |
| **Enterprise Auth** | JWT (HS256/RS256/ES256) + API key validation — toggle via `helix.json` |
| **Observability** | Prometheus `/metrics`, Grafana dashboard JSON, HPA-ready Kubernetes manifests |

---

## ⚡ Quick Start

### 1. Install the compiler

```bash
go install github.com/helixrpc/helix-rpc/compiler/helix-gen@latest
```

### 2. Scaffold a new service (Go)

```bash
helix-gen init my-service --lang go
cd my-service
```

This generates:
```
my-service/
├── schema.proto          # your IDL
├── server/main.go        # handler stub — write your logic here
├── generated/            # auto-generated stubs
├── helix.json            # full config (auth, rate limits, features)
├── Makefile              # gen · build · test · docker-build · deploy-aws/gcp/azure
├── containers/           # optimised Dockerfile + docker-compose
├── deployments/          # Terraform for AWS/GCP/Azure + k8s manifests
└── README.md
```

### 3. Define your schema

```protobuf
// schema.proto
message PredictRequest  { string prompt = 1; }
message PredictResponse { string completion = 1; }

service ModelService {
  rpc Predict(PredictRequest) returns (PredictResponse);
}
```

### 4. Generate stubs

```bash
make gen
```

### 5. Write your handler (the only code you write)

```go
// server/main.go
server.RegisterMethod("/v1.ModelService/Predict", runtime.MethodInfo{
    Handler: func(ctx context.Context, req interface{}) (interface{}, error) {
        // ← your business logic here
        return &PredictResponse{Completion: "Hello from Helix!"}, nil
    },
})
```

### 6. Run it

```bash
make dev           # local dev with hot-reload
make docker-build  # optimised scratch container
make deploy-aws    # push to AWS ECS Fargate via Terraform
```

---

## 🌐 Supported Languages

| Runtime | Protocols | Key Capabilities |
|---|---|---|
| **Go** | gRPC · Thrift · HTTP/JSON | Dynamic batching, circuit breaker, JWT auth, Prometheus |
| **Rust** | gRPC · HTTP/JSON · SSE | PyO3 zero-copy FFI, TLS, keepalive, Firecracker |
| **Python** | HTTP/JSON · SSE | asyncio, streaming generators, asyncpg pool |

---

## 📦 What `helix.json` Controls

```jsonc
{
  "host": "0.0.0.0",
  "port": 8080,
  "disable_metrics": false,    // Prometheus /metrics endpoint
  "disable_health": false,     // /healthz liveness probe
  "disable_gzip": false,       // per-message compression
  "disable_deadline": false,   // grpc-timeout propagation
  "rate_limit_rate": 100.0,    // token-bucket refill rate
  "rate_limit_burst": 10,
  "enable_jwt_auth": false,    // toggle JWT validation
  "jwt_secret": "...",
  "enable_api_key_auth": false, // toggle API key validation
  "api_key": "..."
}
```

Changes take effect **without restart** — the runtime watches and hot-reloads automatically.

---

## 🏗 Architecture Overview

```
Client
  │
  ▼
┌─────────────────────────────────────────────────────────┐
│              Single TCP Port (protocol sniffing)        │
│   gRPC / HTTP2  │  Thrift Binary/Compact  │  REST/JSON  │
└────────────────────────┬────────────────────────────────┘
                         │
              ┌──────────▼──────────┐
              │   Helix Gateway     │
              │  (Go / Rust)        │
              │  ┌───────────────┐  │
              │  │  Interceptors │  │  JWT · Rate Limit · Circuit Breaker
              │  │  Middlewares  │  │  Logging · Metrics · Deadline
              │  └───────┬───────┘  │
              │          │          │
              │  ┌───────▼───────┐  │
              │  │  Your Handler │  │  ← write this only
              │  └───────────────┘  │
              └─────────────────────┘
                         │
             ┌───────────┼───────────┐
             ▼           ▼           ▼
          Database    Kafka/MQ    AI Model (PyO3)
```

---

## 📚 Documentation

Full documentation is available at **[helixrpc.github.io/helix-rpc](https://helixrpc.github.io/helix-rpc)**

- [Setup & Configuration](https://helixrpc.github.io/helix-rpc/setup-configuration/)
- [Developer Guide](https://helixrpc.github.io/helix-rpc/developer-guide/)
- [Tutorials](https://helixrpc.github.io/helix-rpc/tutorials/)
- [Integrations](https://helixrpc.github.io/helix-rpc/integrations/)
- [Enterprise Concerns](https://helixrpc.github.io/helix-rpc/enterprise-concerns/)

---

## 🤝 Contributing

We welcome contributions! Please read our [Contributing Guide](CONTRIBUTING.md) and open a pull request.

---

## 📄 License

MIT © Helix RPC Contributors
