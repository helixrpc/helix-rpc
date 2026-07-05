# Migration Guide: Moving to Helix RPC

Migrating from legacy frameworks like **gRPC (HTTP/2)**, **Apache Thrift**, or **CloudWeGo (Kitex)** to Helix RPC is designed to be seamless. 

By leveraging Helix's **Same-Port Multiplexer** and **Zero-Allocation Transpilers**, you can migrate with minimal configuration changes and zero downtime.

---

## 1. Zero-Downtime Transition using Config Changes

Instead of modifying all client and server code at once, you can transition services by configuring Helix's **Same-Port Multiplexed Server** inside your server configurations.

### Configuration (`config.yaml`)
By adding these few lines of configuration, your Helix server will automatically sniff, route, and serve legacy incoming traffic:

```yaml
server:
  addr: "0.0.0.0:8080"
  multiplexing:
    enabled: true
    protocols:
      - grpc       # Handles standard gRPC / HTTP/2 clients
      - http       # Handles REST/JSON endpoints
      - thrift     # Handles legacy Apache Thrift compact/binary clients
      - sse        # Handles Server-Sent Events (SSE) stream endpoints
```

*   **How it works:** When a request arrives, Helix peeks the first few bytes. If it matches a legacy protocol (like Thrift or standard gRPC), it routes it directly to the corresponding legacy handler. If it's a Helix client, it routes via optimized paths.

---

## 2. Migrating from gRPC / Protobuf

### Step 1: Regenerate Stubs
Replace your old `protoc` generation commands with `helix-gen`:

```bash
# Before (Legacy gRPC)
protoc --go_out=. --go-grpc_out=. schema.proto

# After (Helix RPC)
./helix-gen generate -idl schema.proto -lang go -out ./generated/
```

### Step 2: Update Server Initialisation
Instead of importing standard gRPC packages, initialize the Helix server runner:

```go
// Before (Legacy gRPC)
s := grpc.NewServer()
pb.RegisterPredictServiceServer(s, &myServer{})

// After (Helix RPC)
s := runtime.NewServer("0.0.0.0:8080")
// Register handlers dynamically
s.RegisterMethod("/predict.PredictService/Predict", handler)
```

---

## 3. Migrating from CloudWeGo (Kitex) & Thrift

CloudWeGo (Kitex) uses high-performance Thrift codecs. Helix provides **Zero-Allocation Transpilers** to translate incoming Protobuf payloads into Thrift compact structures without extra allocations, allowing you to bridge Kitex clients to modern backend services.

### Bridging Kitex to Helix:
1.  **Configure Transpilation in Config:**
    ```yaml
    transpile:
      enabled: true
      from: protobuf
      to: thrift_compact
    ```
2.  **No Client Changes Needed:** The Kitex clients continue sending legacy Thrift compact payloads. The Helix Gateway intercepts them, transpiles them in-place with zero heap allocations, and forwards them as gRPC/Protobuf internally.
