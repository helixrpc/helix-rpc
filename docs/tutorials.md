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
