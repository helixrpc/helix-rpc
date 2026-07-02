# Helix RPC 🧬

Helix RPC is a next-generation AI infrastructure framework designed from the ground up for the absolute highest efficiency in deploying LLMs and machine learning models. 

Built in **Go** and **Rust**, it features a multi-protocol gateway (gRPC, HTTP/2, REST, SSE) and leverages PyO3 to completely eliminate the serialization bottleneck between the network gateway and the Python AI execution environment.

## 🎯 Focus on Business Logic, Zero-Config

Helix RPC is architected so developers spend **zero time** configuring network stacks, deployment pipelines, telemetry, or security middlewares. 

Scaffolding a project with `helix-gen init` yields a fully production-grade, highly available microservice out-of-the-box. Middlewares like structured JSON logging, Prometheus metrics, K8s health checks, TLS, gzip compression, and rate limiting are pre-configured and active automatically. 

**Your only task is to write the core business logic handler.**

## 🚀 Key Features

*   **Zero-Serialization PyO3 Runtime (Rust)**: Embeds the CPython interpreter directly inside the Rust Async Gateway. Tensors and prompts are passed in-memory using zero-copy FFI, bypassing JSON/gRPC serialization entirely.
*   **Dynamic Batching (Go)**: A powerful `@batch` interceptor that buffers concurrent incoming REST/gRPC requests and dispatches them mathematically as a single array to the underlying AI model to maximize GPU utilization and prevent OOMs.
*   **Native SSE Streaming (Rust/Python)**: The Rust Hyper server bridges directly into Python `yield` generators using `PyIterator`, allowing for real-time token streaming natively to clients via Server-Sent Events (SSE).
*   **Multi-Protocol Gateway**: Supports gRPC, HTTP/2 multiplexing, REST API fallbacks, and SSE out-of-the-box.
*   **Production-Ready Middlewares**: Includes Deadline Propagation (gRPC Timeout), Health Checking, and gzip Per-Message Compression.

---

## 📖 Usage Examples

Here are the code snippets demonstrating how to integrate and use the core features of Helix RPC.

### 1. Go: Setting up Dynamic Batching

Dynamic Batching allows your API server to absorb 100 concurrent HTTP requests, and send them as 1 single batched request to your GPU.

```go
package main

import (
	"context"
	"time"
	runtime "github.com/helixrpc/helix-rt"
)

// 1. Define your batch processing logic
type MyAIModel struct{}

func (m *MyAIModel) PredictBatch(ctx context.Context, reqs []interface{}) ([]interface{}, error) {
	// Execute your model on the batched array here!
	var resps []interface{}
	for _, req := range reqs {
		resps = append(resps, map[string]string{"completion": "Done"})
	}
	return resps, nil
}

func main() {
	// 2. Initialize the Batch Scheduler (Max 100 requests, 50ms wait window)
	dispatcher := runtime.NewBatchScheduler(100, 50*time.Millisecond, func(ctx context.Context, reqs []interface{}) ([]interface{}, error) {
		model := &MyAIModel{}
		return model.PredictBatch(ctx, reqs)
	})

	// 3. Create Server and Register Route
	server := runtime.NewServer(":8080")
	server.RegisterMethod("/v1/models/predict", runtime.MethodInfo{
		Decoder: func(dec func(interface{}) error) (interface{}, error) {
			var req map[string]interface{}
			err := dec(&req)
			return req, err
		},
		Handler: func(ctx context.Context, req interface{}) (interface{}, error) {
			// This blocks until the batch window closes and resolves!
			return dispatcher.Invoke(ctx, req) 
		},
	})
	
	server.RegisterRESTRoute("POST", "/v1/models/predict", "/v1/models/predict")
	server.Start()
}
```

### 2. Rust: Zero-Serialization PyO3 Server

Embed a Python model in your Rust server. Data is passed without any JSON/Protocol Buffer serialization.

```rust
use helix_rt::server::{HelixServer, RestRoute};
use helix_rt::pyo3_runner::PyModelHandler;
use std::sync::Arc;

#[tokio::main]
async fn main() {
    // 1. Load the Python file `model.py` and instantiate `DummyModel` natively via PyO3
    let py_handler = PyModelHandler::new(".", "model", "DummyModel").unwrap();

    // 2. Wrap it in a Server
    let mut server = HelixServer::new(
        "127.0.0.1:8080",
        Arc::new(py_handler),
        vec![RestRoute::new("POST", "/v1/predict", "/v1/predict")],
    );

    server.start().await.unwrap();
}
```

### 3. Python/Rust: Native SSE Token Streaming

To build a ChatGPT-like streaming experience, define a generator in Python. Helix RPC will natively bridge the generator across the FFI boundary and transcode it to SSE JSON over the network.

**Python Model (`model.py`):**
```python
import time

class DummyModel:
    def generate_stream(self, prompt: str):
        words = ["This", " is", " streaming", " natively", "!"]
        for word in words:
            yield word
            time.sleep(0.1)
```

**Rust Server Setup:**
```rust
use helix_rt::server::HttpSseHandler;
use async_trait::async_trait;

struct StreamSseHandler {
    model: Arc<PyModelHandler>,
}

#[async_trait]
impl HttpSseHandler for StreamSseHandler {
    fn is_sse(&self, path: &str) -> bool {
        path == "/v1/chat/completions"
    }

    async fn handle_sse(&self, path: &str, body: Vec<u8>, is_json: bool) 
    -> Result<tokio::sync::mpsc::Receiver<Result<String, String>>, String> {
        let (tx, rx) = tokio::sync::mpsc::channel(64);
        
        // Spawn blocking Python generator iteration, piping yields to the MPSC channel
        self.model.clone().generate_stream("Hello!".to_string(), tx);
        
        Ok(rx)
    }
}

// In your server setup:
// server.set_sse_handler(Arc::new(StreamSseHandler { model: py_handler }));
```

### 4. Production Features (Go)

Helix RPC ships with built-in primitives for building production-grade services.

#### 4a. Health Checking (gRPC Health v1)
Helix automatically mounts the standard gRPC health checking protocol (`/grpc.health.v1.Health/Check`). You can programmatically control the serving status of your services.
```go
server := runtime.NewServer(":8080")

// Mark a service as Serving
server.Health.SetServingStatus("ai.generation.Service", runtime.HealthServing)

// Later, mark as Not Serving if the GPU goes offline
server.Health.SetServingStatus("ai.generation.Service", runtime.HealthNotServing)
```

#### 4b. mTLS (Mutual TLS)
You can easily secure your Helix Gateway using standard Go TLS Configurations.
```go
import "crypto/tls"

server := runtime.NewServer(":8443")

// Load your certificates and configure mTLS
cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
server.TLSConfig = &tls.Config{
	Certificates: []tls.Certificate{cert},
	ClientAuth:   tls.RequireAndVerifyClientCert,
}

server.Start()
```

#### 4c. Middleware Injection (Interceptors)
Helix supports gRPC-style Unary Server Interceptors for implementing logging, tracing, auth, and deadline propagation.
```go
server := runtime.NewServer(":8080")

server.AddInterceptor(func(ctx context.Context, req interface{}, info *runtime.UnaryServerInfo, handler runtime.UnaryHandler) (interface{}, error) {
	fmt.Printf("Incoming request to: %s\n", info.FullMethod)
	start := time.Now()
	
	resp, err := handler(ctx, req)
	
	fmt.Printf("Completed in %v\n", time.Since(start))
	return resp, err
})
```

## 🎮 Demo Applications

We've provided 3 fully functional demo applications out of the box in the `examples/` directory! 

1. `examples/rust-ai-gateway`: An end-to-end PyO3 server with SSE streaming.
2. `examples/go-dynamic-batcher`: A high-concurrency Go server demonstrating request batching.
3. `examples/frontend-chat-ui`: A sleek, glassmorphic UI built in Vanilla JS to visualize the SSE stream natively!
