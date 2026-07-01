# Helix RPC — Developer Guide

> **Helix RPC** is a high-performance, multi-protocol RPC framework with unified code generation for **Go** and **Rust**. Define your service once in Protobuf or Thrift and Helix generates zero-reflection stubs that serve gRPC, Thrift Binary/Compact, HTTP/JSON REST, and bidirectional streaming — all on a single TCP port.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Project Structure](#project-structure)
3. [Quick Start](#quick-start)
4. [Single-Port Protocol Sniffing](#single-port-protocol-sniffing)
5. [Service Registration](#service-registration)
6. [REST Transcoding](#rest-transcoding)
7. [Bidirectional Streaming](#bidirectional-streaming)
8. [Interceptors & Middleware](#interceptors--middleware)
9. [Metadata Propagation](#metadata-propagation)
10. [Error Mapping](#error-mapping)
11. [Client Pooling & Load Balancing](#client-pooling--load-balancing)
12. [Service Discovery](#service-discovery)
13. [Shared-Memory (SHM) Transport](#shared-memory-shm-transport)
14. [Production Features](#production-features)
15. [Performance Tuning](#performance-tuning)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Single TCP Port                          │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │              Allocation-Free Protocol Sniffer             │  │
│  │   (Peeks first bytes: gRPC H2 preface / Thrift / HTTP)    │  │
│  └────────┬──────────────┬──────────────┬────────────────────┘  │
│           │              │              │                        │
│     ┌─────▼─────┐  ┌────▼────┐  ┌──────▼──────┐                │
│     │  gRPC/H2  │  │ Thrift  │  │ HTTP/1.1    │                │
│     │  Handler  │  │ Handler │  │ REST Handler│                │
│     └─────┬─────┘  └────┬────┘  └──────┬──────┘                │
│           │              │              │                        │
│     ┌─────▼──────────────▼──────────────▼─────┐                 │
│     │       Method Registry (zero-reflection)  │                │
│     │       Interceptor Chain                  │                │
│     │       Metadata Context                   │                │
│     └─────────────────────────────────────────┘                 │
└─────────────────────────────────────────────────────────────────┘
```

### Key Design Principles

| Principle | Description |
|---|---|
| **Zero-Reflection** | Generated stubs use static type registration — no `reflect` package, no runtime schema lookup |
| **Single-Port** | One TCP listener sniffs the wire protocol and routes to the correct handler |
| **Dual-Runtime** | Identical semantics in Go (stdlib `net/http` + `h2c`) and Rust (`tokio` + `hyper`) |
| **Architecture-Neutral** | No x86 intrinsics; runs at full speed on ARM64 / Apple Silicon |

---

## Project Structure

```
helix-rpc/
├── compiler/              # Helix IDL compiler
│   ├── ast/               # Unified AST (Protobuf + Thrift → common IR)
│   ├── parser/            # .proto and .thrift parsers
│   └── gen/               # Code generators (Go, Rust)
│       ├── go_gen.go
│       └── rust_gen.go
├── runtime-go/            # Go runtime library
│   ├── grpc_handler.go    # gRPC/HTTP handler, streaming, interceptors
│   ├── sniffer.go         # Protocol sniffing listener
│   ├── metadata.go        # Request-scoped metadata context
│   ├── errors.go          # Unified error codes
│   ├── client_pool.go     # Connection pooling & load balancing
│   ├── resolver.go        # Service discovery interface
│   └── shm.go             # POSIX shared-memory transport
├── runtime-rust/          # Rust runtime library
│   └── src/
│       ├── server.rs      # HTTP service, streaming, REST routing
│       ├── sniffer.rs     # Protocol sniffing
│       ├── metadata.rs    # Task-local metadata context
│       ├── errors.rs      # Unified error codes
│       ├── client_pool.rs # Connection pooling & load balancing
│       ├── resolver.rs    # Service discovery trait
│       ├── shm_transport.rs # POSIX shared-memory transport
│       └── lib.rs         # Crate root re-exports
└── integration-tests/
    ├── go-go/             # Go-to-Go E2E tests
    ├── rust-rust/          # Rust-to-Rust E2E tests
    └── schema/            # Shared .proto / .thrift definitions
```

---

## Quick Start

### Go Server

```go
package main

import (
    "context"
    "log"

    runtime "github.com/helix-rpc/helix/runtime-go"
    generated "github.com/helix-rpc/helix/generated"
)

func main() {
    server := runtime.NewServer("0.0.0.0:9000")

    // Register a unary RPC method
    server.RegisterMethod("/myapp.UserService/GetUser", runtime.MethodInfo{
        Decoder: func(dec func(interface{}) error) (interface{}, error) {
            req := &generated.UserRequest{}
            return req, dec(req)
        },
        Handler: func(ctx context.Context, req interface{}) (interface{}, error) {
            r := req.(*generated.UserRequest)
            return &generated.UserResponse{Name: "Hello, " + r.Name}, nil
        },
    })

    log.Fatal(server.Start())
}
```

### Rust Server

```rust
use std::sync::Arc;
use helix_rt::{HttpServiceHandler, handle_http_connection};

struct MyService;

#[async_trait::async_trait]
impl HttpServiceHandler for MyService {
    async fn handle_request(&self, path: &str, body: Vec<u8>, is_json: bool)
        -> Result<(Vec<u8>, String), String>
    {
        match path {
            "/myapp.UserService/GetUser" => {
                // Decode, process, encode...
                Ok((response_bytes, content_type))
            }
            _ => Err(format!("unknown path: {}", path)),
        }
    }
}
```

---

## Single-Port Protocol Sniffing

Helix uses an **allocation-free sniffer** that peeks at the first bytes of each TCP connection to determine the wire protocol:

| Byte Pattern | Protocol |
|---|---|
| `PRI * HTTP/2.0` (HTTP/2 preface) | gRPC over HTTP/2 |
| `0x82` (Compact) or `0x80 0x01` (Binary) | Thrift |
| `GET`, `POST`, `PUT`, `DELETE`, `PATCH` | HTTP/1.1 REST |

### Go

```go
// The sniffer is built into the Server. Just call Start():
server := runtime.NewServer(":9000")
server.Start()
// All protocols are served on port 9000
```

### Rust

```rust
let protocol = helix_rt::sniff_protocol(&stream).await?;
match protocol {
    Protocol::Grpc => { /* HTTP/2 handler */ }
    Protocol::ThriftCompact | Protocol::ThriftBinary => { /* Thrift handler */ }
    Protocol::Http => { /* REST handler */ }
    _ => { /* unsupported */ }
}
```

---

## Service Registration

### Go — Static Method Registration

```go
server.RegisterMethod("/pkg.Service/Method", runtime.MethodInfo{
    Decoder: func(dec func(interface{}) error) (interface{}, error) {
        req := &pb.MyRequest{}
        return req, dec(req)
    },
    Handler: func(ctx context.Context, req interface{}) (interface{}, error) {
        r := req.(*pb.MyRequest)
        return handler.Process(ctx, r)
    },
    // Optional: bind REST path parameters
    Binder: func(req interface{}, params map[string]string) error {
        r := req.(*pb.MyRequest)
        r.Id = params["id"]
        return nil
    },
})
```

---

## REST Transcoding

Map HTTP verbs and URL paths to gRPC methods:

### Go

```go
server.RegisterRESTRoute("GET", "/v1/users/{user_id}", "/pkg.UserService/GetUser")
server.RegisterRESTRoute("POST", "/v1/users", "/pkg.UserService/CreateUser")
```

### Rust

```rust
let rest_routes = vec![
    RestRoute::new("GET", "/v1/users/{user_id}", "/pkg.UserService/GetUser"),
    RestRoute::new("POST", "/v1/users", "/pkg.UserService/CreateUser"),
];
handle_http_connection(stream, handler, rest_routes, is_http2).await;
```

Path parameters like `{user_id}` are automatically extracted and merged into the JSON request body (or bound via the `Binder` function in Go).

---

## Bidirectional Streaming

Helix supports full-duplex gRPC bidirectional streaming over HTTP/2.

### Go

```go
server.RegisterMethod("/pkg.Service/StreamData", runtime.MethodInfo{
    IsStreaming: true,
    StreamHandler: func(stream runtime.ServerStream) error {
        for {
            var req pb.DataRequest
            if err := stream.Recv(&req); err == io.EOF {
                return nil  // client done
            } else if err != nil {
                return err
            }

            resp := &pb.DataResponse{Result: process(req)}
            if err := stream.Send(resp); err != nil {
                return err
            }
        }
    },
})
```

### Rust

```rust
struct MyStreamHandler;

#[async_trait::async_trait]
impl HttpStreamingHandler for MyStreamHandler {
    fn is_streaming(&self, path: &str) -> bool {
        path == "/pkg.Service/StreamData"
    }

    async fn handle_stream(
        &self,
        _path: &str,
        mut stream: Box<dyn ServerStream>,
    ) -> Result<(), String> {
        loop {
            match stream.recv().await? {
                None => return Ok(()),  // client done
                Some(payload) => {
                    let req = MyRequest::decode(&payload[..]).unwrap();
                    let resp_bytes = process_and_encode(req);
                    stream.send(resp_bytes).await?;
                }
            }
        }
    }
}

// Wire it up:
handle_http_connection_streaming(stream, handler, routes, streaming, true).await;
```

### Wire Protocol

Each frame follows the standard gRPC length-prefixed format:

```
┌─────────┬──────────────┬─────────────────┐
│ 1 byte  │   4 bytes    │  N bytes        │
│ compress│   length     │  protobuf       │
│ flag    │   (big-end.) │  payload        │
└─────────┴──────────────┴─────────────────┘
```

---

## Interceptors & Middleware

### Go — Unary Server Interceptors

```go
// Logging interceptor
server.AddInterceptor(func(
    ctx context.Context,
    req interface{},
    info *runtime.UnaryServerInfo,
    handler runtime.UnaryHandler,
) (interface{}, error) {
    start := time.Now()
    resp, err := handler(ctx, req)
    log.Printf("[%s] %v", info.FullMethod, time.Since(start))
    return resp, err
})
```

Interceptors chain in registration order. Each interceptor wraps the next, forming a middleware pipeline.

---

## Metadata Propagation

Request headers are automatically extracted into a typed metadata context.

### Go

```go
// Inside a handler:
if md, ok := runtime.FromContext(ctx); ok {
    traceID := md.Get("x-trace-id")
    // ...
}
```

### Rust

```rust
// Inside an async handler (uses tokio task-local storage):
if let Some(trace_ids) = helix_rt::get_metadata("x-trace-id") {
    println!("trace: {}", trace_ids[0]);
}
```

---

## Error Mapping

Helix defines a unified `ErrorCode` enum that maps across protocols:

| Helix Code | gRPC Status | HTTP Status | Thrift Exception |
|---|---|---|---|
| `NotFound` | 5 (NOT_FOUND) | 404 | ApplicationError |
| `InvalidArgument` | 3 (INVALID_ARGUMENT) | 400 | ApplicationError |
| `Internal` | 13 (INTERNAL) | 500 | ApplicationError |
| `Unauthenticated` | 16 (UNAUTHENTICATED) | 401 | ApplicationError |
| `PermissionDenied` | 7 (PERMISSION_DENIED) | 403 | ApplicationError |

---

## Client Pooling & Load Balancing

### Go

```go
pool := runtime.NewClientConnPool([]string{
    "127.0.0.1:9001",
    "127.0.0.1:9002",
    "127.0.0.1:9003",
})
pool.SetBalancer(&runtime.RoundRobinBalancer{})

conn, err := pool.Get()
// Use conn...
pool.Put(conn)
```

### Rust

```rust
let pool = ClientConnPool::new(vec![
    "127.0.0.1:9001".parse().unwrap(),
    "127.0.0.1:9002".parse().unwrap(),
]);
pool.set_balancer(Box::new(RoundRobinBalancer::new()));

let conn = pool.get().await?;
// Use conn...
pool.put(conn).await;
```

---

## Service Discovery

Helix provides a `Resolver` interface for pluggable service discovery:

### Go

```go
// Static resolver (built-in)
resolver := runtime.NewStaticResolver([]string{
    "10.0.1.1:9000",
    "10.0.1.2:9000",
})
addrs := resolver.Resolve("my-service")

// Custom resolver (e.g., Consul, etcd):
type MyResolver struct{}
func (r *MyResolver) Resolve(service string) []string {
    // Query your service registry...
}
```

### Rust

```rust
let resolver = StaticResolver::new(vec![
    "10.0.1.1:9000".parse().unwrap(),
    "10.0.1.2:9000".parse().unwrap(),
]);
let addrs = resolver.resolve("my-service").await;
```

---

## Shared-Memory (SHM) Transport

For same-host IPC, Helix provides a POSIX shared-memory transport that bypasses the kernel network stack entirely.

### Go

```go
// Server side
shmListener, err := runtime.NewShmListener("/helix-shm-region", 4096)
defer shmListener.Close()
conn, _ := shmListener.Accept()

// Client side
conn, err := runtime.NewShmConn("/helix-shm-region", 4096)
conn.Write(data)
```

### Rust

```rust
use helix_rt::ShmConn;

// Open or create a shared-memory region
let shm = ShmConn::new("/helix-shm-region", 4096)?;
shm.write(b"hello from rust")?;
let data = shm.read()?;
```

> [!WARNING]
> SHM transport requires both processes to run on the same host. The memory region name must match between client and server. Ensure proper cleanup of `/dev/shm/` entries.

---

---

## Production Features

Helix RPC includes built-in support for standard production features.

### 1. Deadline / Timeout Propagation

Helix automatically extracts the `grpc-timeout` header and applies it to the context, terminating long-running requests early and returning a `DEADLINE_EXCEEDED` (status code 4) error if the deadline is reached.

#### Go
In Go, the deadline is automatically injected into the request `context.Context`. You can check for deadline expiration using standard Go practices:
```go
func (s *myService) GetUserProfile(ctx context.Context, req *generated.UserProfile) (*generated.UserProfile, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    case <-time.After(10 * time.Millisecond):
        // do work...
    }
}
```

#### Rust
In Rust, the handler future is automatically wrapped in a `tokio::time::timeout` block if a deadline is present. Standard `tokio::select!` or `.await` on downstream calls automatically propagates the cancellation.

---

### 2. Health Checking Service

Helix RPC registers a standard gRPC health-checking service at `/grpc.health.v1.Health/Check`.

#### Go
To set service-specific health status on the server:
```go
server := runtime.NewServer("127.0.0.1:8080")
server.Health.SetServingStatus("my.Service", runtime.HealthServing)
```

#### Rust
```rust
// In Rust, HealthChecker is cloned and passed to connection handlers:
let hc = helix_rt::HealthChecker::new();
hc.set_serving_status("my.Service", helix_rt::HealthStatus::Serving).await;
```

---

### 3. Per-Message Compression

Helix RPC supports gzip compression on gRPC message frames.

#### Go
Client requests can specify `grpc-encoding: gzip` to compress message payloads. The server will dynamically decode compressed payloads and write compressed responses if `grpc-encoding` is requested by the client.

#### Rust
```rust
// GzipCompressor is exported by default:
use helix_rt::{Compressor, GzipCompressor};
let compressor = GzipCompressor;
let compressed = compressor.compress(&payload)?;
```

---

## Performance Tuning

### HTTP/2 Window Sizes

Helix configures generous HTTP/2 flow control windows by default:

| Setting | Default | Description |
|---|---|---|
| Connection window | 2 MB | Total buffered data per connection |
| Stream window | 1 MB | Buffered data per individual stream |
| Max concurrent streams | 250 | Parallel RPCs per connection |

### Benchmark Results (vs. CloudWeGo Kitex)

| Metric | Helix | Kitex | Improvement |
|---|---|---|---|
| Thrift Serialization | ~95% faster | baseline | Static stubs vs. reflection |
| Memory per RPC | Lower | Higher | Zero-reflection, no schema cache |
| ARM64 Performance | Native speed | Degraded* | No x86-only codepaths |

*Kitex's `dynamicgo` fast-path is x86-only; falls back to reflection on ARM64.

---

> [!TIP]
> For production deployments, consider:
> - Using the SHM transport for sidecar/proxy patterns on the same host
> - Enabling interceptors for OpenTelemetry tracing propagation
> - Using the `StaticResolver` as a starting point, then implement a custom `Resolver` for Consul/etcd/Kubernetes service discovery
