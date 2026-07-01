# Welcome to Helix RPC 🧬

Helix RPC is a next-generation AI infrastructure framework designed for the absolute highest efficiency in deploying LLMs and machine learning models. Built in **Go** and **Rust**, it eliminates the massive serialization bottlenecks that plague modern AI deployments.

## The Vision

Modern AI inference is bottlenecked not just by GPUs, but by the network and data serialization overhead between the user and the Python execution environment. Most architectures use a Go/Rust Gateway that serializes JSON, proxies it over gRPC to a Python server, which deserializes the protobuf, parses it, runs inference, serializes the response back to protobuf, and so on.

**Helix RPC's vision is to eliminate this completely.** 

By embedding the Python interpreter directly into a multi-threaded Rust Tokio runtime via PyO3, we achieve **Zero-Serialization AI Execution**. The memory of the web server is the memory of the AI model. 

## Goals
- **Absolute Maximum Throughput:** Achieve the theoretical minimum latency by executing AI inferences inside the gateway itself.
- **Flawless Concurrency:** Use Go's goroutines to mathematically optimal batch REST requests (`@batch`) into massively parallel GPU arrays.
- **Real-Time Streaming:** Seamlessly yield tokens from a Python Generator natively out to an HTTP Server-Sent Events (SSE) stream.
- **Protocol Agnosticism:** Serve gRPC, HTTP/2 multiplexing, REST API fallbacks, and SSE out of the box with zero configuration.

## Getting Started

Check out the [Tutorials](tutorials.md) to build your first AI Gateway, or dive into the [Architecture](architecture.md) to see how it works under the hood!
