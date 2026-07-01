# Helix RPC - Chat UI

A modern, aesthetically premium chat application demonstrating the real-time SSE token streaming capabilities of the Helix RPC Gateway.

## Features
- **Glassmorphism Design**: Sleek, modern aesthetics using vanilla HTML/CSS.
- **Zero-Dependency**: No Node.js, React, or build steps required. Pure Vanilla JS.
- **SSE Streaming**: Connects natively to `rust-ai-gateway` to render OpenAI-formatted JSON chunks token-by-token in real-time.

## How to Run
This frontend is automatically hosted by the `rust-ai-gateway` example!

1. Start the Rust Gateway:
```bash
cd ../rust-ai-gateway
cargo run
```

2. Open your browser to `http://127.0.0.1:8081`. The UI will load and you can chat with the embedded Python model natively!
