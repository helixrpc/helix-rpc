# Examples Overview

The `examples/` directory in the repository contains three fully functional applications demonstrating the capabilities of Helix RPC.

## 1. Rust AI Gateway
**Path:** `examples/rust-ai-gateway`

This is a complete Rust binary that embeds a Python AI model using `PyO3`. It exposes a `/v1/chat/completions` endpoint that streams tokens natively via Server-Sent Events (SSE). It also acts as a basic static web server, hosting the Frontend Chat UI on `http://127.0.0.1:8081`.

## 2. Frontend Chat UI
**Path:** `examples/frontend-chat-ui`

A modern, glassmorphic UI built in Vanilla HTML/JS/CSS. It mimics the look and feel of premium chat interfaces. 
It uses the browser's native `fetch` API to connect to the Rust AI Gateway's SSE endpoint, parsing the JSON chunks and simulating the token-by-token typing effect perfectly.

## 3. Go Dynamic Batcher
**Path:** `examples/go-dynamic-batcher`

A high-performance Go API Gateway that demonstrates the power of the `BatchScheduler`. If you fire dozens of concurrent HTTP requests at this server, you will see in the logs that it elegantly buffers them and processes them as a single array, returning the perfectly scattered responses back to the original callers.

## 4. Python Dynamic Batcher
**Path:** `examples/python-dynamic-batcher`

A pure-Python equivalent to the Go Dynamic Batcher, using the `helix_rt` Python SDK. It leverages `asyncio` to natively bundle concurrent `await` calls into batch arrays. It also demonstrates native SSE streaming and strict execution deadlines using the built-in middlewares.

## 5. Node.js Parity Suite
**Path:** `integration-tests/node-node`

A TypeScript test suite demonstrating the full `helix-rt-node` runtime package. It boots a protocol-sniffing server, registers JSON REST transcoding routes, and executes requests passing through built-in JWT authorization middlewares, Token Bucket rate-limiters, dynamic batch schedulers, and exponential retry policies.
