# LLM Serving Tutorial (Zero-Serialization Token Streaming)

This tutorial walks you through deploying a production-ready LLM token streaming service. 

By embedding Python directly in Helix's Rust/PyO3 gateway, we bypass network hops and JSON serialization entirely. The web gateway reads raw tokens directly from Python memory buffers and streams them instantly to the client via Server-Sent Events (SSE).

---

## Architecture Diagram

```
[Web Client] 
     │ (HTTP GET /stream?prompt=...)
     ▼
┌────────────────────────────────────────────────────────┐
│ Helix Rust Gateway (Hyper + PyO3)                      │
│                                                        │
│  1. Receives SSE Connection Request                    │
│  2. Calls Python generator in-memory via PyO3 FFI      │
│  3. Receives yielded tokens directly in Rust memory    │
│  4. Flushes SSE chunks immediately to connection       │
└──────────────────┬─────────────────────────────────────┘
                   │
                   ▼ (Zero-Serialization / In-Memory)
┌────────────────────────────────────────────────────────┐
│ CPython Runtime (PyTorch / Transformers / HuggingFace) │
└────────────────────────────────────────────────────────┘
```

---

## 1. The Python Model (`llm.py`)
First, define your model inference script. In a real environment, you would load a model using `transformers` or `llama.cpp`. Here, we mock the generator to yield tokens with slight sleep delays to simulate GPU inference latency:

```python
import asyncio
import time

class LLMModel:
    def __init__(self):
        # In production: self.model = AutoModelForCausalLM.from_pretrained(...)
        print("🧬 LLM model successfully loaded in Python memory.")

    def generate(self, prompt: str):
        """
        Yields tokens one-by-one. Helix's Rust engine will 
        iterate over this generator natively via PyIterator.
        """
        response_text = f"This is a streamed completion from the Helix PyO3 runtime for the prompt: '{prompt}'."
        for token in response_text.split():
            time.sleep(0.08)  # Simulate GPU evaluation delay
            yield token + " "
```

---

## 2. The Rust Gateway Server (`src/main.rs`)
Next, initialize the Helix Rust gateway. It configures the embedded Python runtime, spins up the server, and maps incoming HTTP GET requests to the Python LLM generator.

```rust
use helix_rt::server::{HelixServer, RestRoute};
use helix_rt::pyo3_runner::PyModelHandler;
use std::sync::Arc;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // 1. Point PyModelHandler to CPython module 'llm' and class 'LLMModel'
    let py_handler = Arc::new(
        PyModelHandler::new(
            ".",         // Module search path (current directory)
            "llm",       // Python filename (llm.py)
            "LLMModel"   // Python Class name
        ).map_err(|e| format!("Python init error: {}", e))?
    );

    // 2. Define the REST SSE Streaming Route
    let routes = vec![
        RestRoute::new("GET", "/stream", "generate")
    ];

    // 3. Start the Helix multiplexed gateway
    let addr = "127.0.0.1:8080";
    let mut server = HelixServer::new(addr, py_handler, routes);
    
    println!("🚀 Helix LLM Server listening on http://{}", addr);
    println!("💡 Stream tokens via: curl -N http://127.0.0.1:8080/stream?prompt=Hello");

    server.start().await?;
    Ok(())
}
```

---

## 3. The Client Interface (`index.html`)
To display the streamed tokens in real-time on a web application, use the standard browser `EventSource` API:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Helix LLM Stream</title>
    <style>
        body { font-family: sans-serif; max-width: 600px; margin: 40px auto; line-height: 1.6; }
        #output { border: 1px solid #ddd; padding: 20px; border-radius: 6px; background: #fafafa; min-height: 100px; white-space: pre-wrap; }
        button { padding: 10px 20px; font-weight: bold; background: #1AABB8; color: white; border: none; border-radius: 4px; cursor: pointer; }
    </style>
</head>
<body>
    <h1>Helix LLM Token Stream</h1>
    <textarea id="prompt" rows="3" style="width: 100%;">Write a story about a fast RPC compiler...</textarea>
    <br/><br/>
    <button onclick="startStream()">Generate Completion</button>
    <p><strong>Response:</strong></p>
    <div id="output"></div>

    <script>
        function startStream() {
            const prompt = encodeURIComponent(document.getElementById('prompt').value);
            const outputDiv = document.getElementById('output');
            outputDiv.innerText = ""; // Clear screen

            // Listen to Server-Sent Events stream from Helix
            const eventSource = new EventSource(`http://127.0.0.1:8080/stream?prompt=${prompt}`);

            eventSource.onmessage = function(event) {
                // Append token chunk to UI
                outputDiv.innerText += event.data;
            };

            eventSource.onerror = function() {
                // Connection closed successfully at end of generation
                eventSource.close();
            };
        }
    </script>
</body>
</html>
```

---

## Running the Tutorial

1. Make sure Python is available in your shell environment and your CPython shared libraries are accessible to Rust.
2. Compile and start the Rust server:
   ```bash
   cargo run --release
   ```
3. Test from another terminal using `curl`:
   ```bash
   curl -N "http://127.0.0.1:8080/stream?prompt=Helix"
   ```
4. Or open `index.html` in your web browser, input your prompt, and watch the completion stream into the page!
