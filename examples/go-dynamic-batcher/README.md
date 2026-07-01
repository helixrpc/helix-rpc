# Helix RPC Go Dynamic Batcher

This example demonstrates Helix RPC's revolutionary **Dynamic Batching** feature!

## How it works

When exposing AI models behind standard REST APIs, if 100 concurrent users hit the endpoint, 100 separate requests hit your GPU. This shatters performance and causes Out-Of-Memory (OOM) errors.

Helix RPC's `@batch` interceptor solves this.
It intercepts all incoming REST requests, holds them in a buffer for a brief window (e.g., 50ms), and then bundles them into a SINGLE array. It dispatches the batched array to the underlying model for massively parallel execution, and perfectly scatters the results back to the original incoming HTTP connections!

## Running the Server

```bash
cd examples/go-dynamic-batcher
go run main.go
```

The server will start on `http://127.0.0.1:8080`.

## Testing the Batcher

In a separate terminal, simulate high concurrency using `curl` background jobs:

```bash
curl -s -X POST http://127.0.0.1:8080/v1/models/predict -d '{"prompt": "Hello 1"}' &
curl -s -X POST http://127.0.0.1:8080/v1/models/predict -d '{"prompt": "Hello 2"}' &
curl -s -X POST http://127.0.0.1:8080/v1/models/predict -d '{"prompt": "Hello 3"}' &
curl -s -X POST http://127.0.0.1:8080/v1/models/predict -d '{"prompt": "Hello 4"}' &
wait
```

Watch the server logs! You'll see the server capture all 4 isolated requests and group them into a single `PredictBatch()` invocation!
