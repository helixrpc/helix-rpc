# Tutorials

## Building a Python AI Gateway in Rust

This tutorial walks you through setting up a Rust Gateway that natively executes a Python script.

### 1. The Python Model
First, create your AI logic in `model.py`:
```python
class MyModel:
    def process(self, prompt: str) -> str:
        return f"AI Processed: {prompt}"
```

### 2. The Rust Server
Next, in your Rust project, add `helix-rt` and `tokio`:
```toml
[dependencies]
helix-rt = "0.1.0"
tokio = { version = "1.0", features = ["full"] }
```

In `src/main.rs`:
```rust
use helix_rt::server::{HelixServer, RestRoute};
use helix_rt::pyo3_runner::PyModelHandler;
use std::sync::Arc;

#[tokio::main]
async fn main() {
    // Point the PyModelHandler to `model.py`, and the class `MyModel`
    let py_handler = PyModelHandler::new(".", "model", "MyModel").unwrap();

    let mut server = HelixServer::new(
        "127.0.0.1:8080",
        Arc::new(py_handler),
        vec![RestRoute::new("POST", "/predict", "/predict")],
    );

    println!("Starting Rust AI Gateway...");
    server.start().await.unwrap();
}
```

### 3. Run It!
```bash
cargo run
```
You can now `curl -X POST http://127.0.0.1:8080/predict -d '{"prompt": "Hello"}'` and it will securely and instantly cross the FFI boundary, execute the Python code, and return the result!

---

## Setting up Advanced Features

### Deadline Propagation (`grpc-timeout`)

When building microservices, you want to ensure your AI model stops processing if the client has already timed out. Helix RPC automatically extracts the `grpc-timeout` header and applies it to the Go `context.Context` and Rust `tokio::time::timeout`.

**Client Side (cURL):**
```bash
# Send a 50-millisecond timeout
curl -H "grpc-timeout: 50m" -X POST http://127.0.0.1:8080/predict -d '{"prompt": "Hello"}'
```

**Server Side (Go):**
```go
// Inside your handler, listen for context cancellation
func(ctx context.Context, req interface{}) (interface{}, error) {
    select {
    case <-ctx.Done():
        // Client timed out! Abort GPU processing to save resources.
        return nil, ctx.Err()
    case res := <-processOnGPU(req):
        return res, nil
    }
}
```

### Per-Message Compression (Gzip)

To enable Gzip compression on large JSON/Protobuf responses, register the compressor when starting your server.

**Go:**
```go
import runtime "github.com/helixrpc/helix-rt"

server := runtime.NewServer(":8080")

// Register the Gzip Compressor
server.RegisterCompressor(runtime.NewGzipCompressor())

server.Start()
```
Now, any client sending `grpc-encoding: gzip` will automatically have their request decompressed, and the response compressed natively by the gateway!

### Multi-Protocol Endpoints

The Go Runtime is capable of natively binding gRPC, Thrift, and REST JSON onto the exact same port and the exact same route logic using `h2c`.

```go
server := runtime.NewServer(":8080")

// 1. Register the underlying handler (Used by gRPC and Thrift natively)
server.RegisterMethod("/v1.ModelService/Predict", runtime.MethodInfo{
    Decoder: myProtobufDecoder,
    Handler: myPredictionLogic,
})

// 2. Map a REST Route to the EXACT SAME METHOD
server.RegisterRESTRoute(
    "POST",                  // HTTP Method
    "/v1/models/predict",    // REST Path
    "/v1.ModelService/Predict", // Target Method
)

server.Start()
```
Your server can now seamlessly handle standard HTTP `POST /v1/models/predict` requests with JSON, as well as high-performance `HTTP/2` Protobuf frames sent by gRPC clients!

---

## Setting up Node.js / TypeScript Server

Here is a quick-start guide to setting up a TypeScript-based Helix server using the Node.js runtime.

```typescript
import { HelixServer } from 'helix-rt-node';

const server = new HelixServer('127.0.0.1:8080');

// Register the underlying method handler
server.registerMethod('/helix.example.UserProfileService/GetUserProfile', {
    Decoder: (dec) => {
        const req = { userId: 0, username: '' };
        dec(req);
        return req;
    },
    Handler: async (ctx, req) => {
        return {
            userId: req.userId,
            username: req.username + '-response',
            email: 'verified@example.com'
        };
    }
});

// Map REST HTTP/1.1 endpoint route to the same method
server.registerRESTRoute('POST', '/v1/users', '/helix.example.UserProfileService/GetUserProfile');

console.log('Starting Node.js Helix Server...');
await server.start();
```
