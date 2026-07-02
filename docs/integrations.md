# System Integrations

Helix RPC integrates seamlessly with standard microservice infrastructure, service discovery agents, service mesh sidecars, and popular web/RPC frameworks.

---

## 1. Service Discovery

### HashiCorp Consul (Go & Rust)
Helix RPC supports direct service registration and discovery querying against HashiCorp Consul catalog endpoints.

**Go Usage:**
```go
// Create resolver targeting Consul agent http endpoint
consulResolver := runtime.NewConsulResolver("http://localhost:8500")

// Pass resolver into client pools for dynamic load balancing
pool := runtime.NewClientConnPool(consulResolver, "my-model-service")
```

---

## 2. Service Mesh Integration

Because Helix RPC multiplexes all protocols (REST, gRPC, Thrift, SSE) on a **single TCP port**, it is natively compatible with modern sidecar service meshes (e.g., **Istio**, **Linkerd**, **Envoy**) without requiring any modifications or custom mesh plugins:

1. **Protocol Auto-Detection**: Envoy sidecars automatically sniff and intercept HTTP/1.1 and HTTP/2 cleartext (`h2c`) traffic, which matches Helix's protocol format exactly.
2. **mTLS Delegation**: You can disable Helix's built-in TLS and delegate transport encryption entirely to the Istio/Linkerd sidecar (mutual TLS mode).
3. **Observability**: Standard mesh proxies will extract HTTP status codes and gRPC headers from Helix RPC traffic out of the box.

---

## 3. Co-existence with Other Frameworks

Helix RPC is designed not to interfere with standard application stacks. You can run Helix RPC side-by-side with other web servers:

- **Python**: You can co-locate a Helix server with a **FastAPI** or **Flask** application. Helix RPC can handle high-throughput, low-latency AI inference via standard loops, while FastAPI manages user logins, database transactions, and frontend assets.
- **Go**: Helix's zero-reflection stub registration allows you to serve standard HTTP endpoints (e.g., **Gin** or **Echo**) on one port, and mount Helix RPC on another port to handle streaming AI model completions.
