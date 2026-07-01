# Helix RPC Rust AI Gateway

This example demonstrates the absolute apex of AI serving efficiency: a 100% Zero-Serialization Rust Gateway embedding the Python interpreter natively in-process via PyO3, combined with hyper-optimized Server-Sent Events (SSE) streaming directly from the Python Generator loop.

## Architecture

1.  **Tokio/Hyper (Rust)**: High-concurrency TCP/HTTP listener handling incoming connections.
2.  **PyO3 (Rust <-> Python)**: Embeds CPython inside the Rust binary.
3.  **Tokio MPSC Bridge**: Safely bridges the blocking Python interpreter `yield` generator to the asynchronous Hyper response streaming task.
4.  **Static Serving**: Also hosts the frontend chat UI statically!

## Running the Server

```bash
cd examples/rust-ai-gateway
cargo run
```

The server will automatically start on `http://127.0.0.1:8081`.

Open `http://127.0.0.1:8081` in your browser to interact with the streaming Chat UI!
