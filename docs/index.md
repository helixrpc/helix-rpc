<p align="center">
  <img src="assets/logo.png" alt="Helix RPC" width="130"/>
</p>

<h1 align="center">Helix RPC 🧬</h1>

<p align="center">
  <strong>The next-generation AI infrastructure framework built for maximum efficiency.</strong><br/>
  Zero-serialization · Multi-protocol · Zero-configuration
</p>

---

## The Vision

Modern AI inference is bottlenecked not just by GPUs, but by the **network and serialization overhead** between the user and the Python execution environment. Most architectures chain a Go/Rust gateway → gRPC → Python → gRPC response — every hop adding latency and CPU burn.

**Helix RPC's vision is to eliminate this completely.**

By embedding the Python interpreter directly into a multi-threaded Rust Tokio runtime via **PyO3**, we achieve **Zero-Serialization AI Execution**. The web server's memory *is* the AI model's memory.

## Goals

- **Absolute Maximum Throughput** — Achieve theoretical minimum latency by executing AI inferences inside the gateway process itself.
- **Flawless Concurrency** — Use Go goroutines to coalesce concurrent REST/gRPC requests into mathematically optimal GPU batch arrays (`@batch`).
- **Real-Time Streaming** — Yield tokens from a Python generator natively into HTTP Server-Sent Events with zero buffering.
- **Protocol Agnosticism** — Serve gRPC, Thrift, HTTP/JSON REST, and SSE on a **single TCP port**, zero extra proxies.
- **Zero Developer Friction** — `helix-gen init` scaffolds a production-grade service with auth, observability, containers, and Terraform — just write the handler.

## Getting Started

Jump into the [Tutorials](tutorials.md) to build your first AI Gateway, or explore the [Developer Guide](developer-guide.md) to understand every feature.

!!! tip "Zero Config"
    Run `helix-gen init my-service --lang go` and your entire deployment pipeline — Docker, Kubernetes, Prometheus, Terraform — is ready before you write a single line of business logic.
