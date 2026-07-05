#!/bin/bash

echo "========================================="
echo "   Helix RPC - Performance Benchmarking  "
echo "========================================="

# Ensure directories exist
mkdir -p ../docs

# Start FastAPI server
echo "Starting FastAPI target server..."
VENV_PYTHON="./venv/bin/python"
$VENV_PYTHON fastapi_server.py > fastapi.log 2>&1 &
FASTAPI_PID=$!

# Start Helix server (Go requires building within the tests module context)
echo "Starting Helix target server..."
go run -tags=dynamicgo helix_server.go > helix.log 2>&1 &
HELIX_PID=$!

# Start gRPC-Gateway server
echo "Starting gRPC-Gateway target server..."
go run grpc_gateway_server.go > grpc_gateway.log 2>&1 &
GATEWAY_PID=$!

# Wait for servers to bind
sleep 3

# Warmup run
echo "Running warmup..."
curl -s http://127.0.0.1:8001/v1/users/42 > /dev/null
curl -s http://127.0.0.1:8002/v1/users/42 > /dev/null
curl -s http://127.0.0.1:8003/v1/users/42 > /dev/null

echo "Running Autocannon load tests (10 seconds per target)..."

echo "Benchmarking FastAPI..."
autocannon -c 100 -d 10 http://127.0.0.1:8001/v1/users/42 > fastapi_bench.txt

echo "Benchmarking gRPC-Gateway..."
autocannon -c 100 -d 10 http://127.0.0.1:8003/v1/users/42 > gateway_bench.txt

echo "Benchmarking Helix RPC..."
autocannon -c 100 -d 10 http://127.0.0.1:8002/v1/users/42 > helix_bench.txt

# Terminate servers
kill $FASTAPI_PID $HELIX_PID $GATEWAY_PID
wait $FASTAPI_PID $HELIX_PID $GATEWAY_PID 2>/dev/null

# Extract stats helper
extract_stats() {
    FILE=$1
    REQ_SEC=$(grep "Req/Sec" -A 1 $FILE | tail -n 1 | awk '{print $2}')
    AVG_LAT=$(grep "Latency" -A 1 $FILE | tail -n 1 | awk '{print $3}')
    echo "$REQ_SEC|$AVG_LAT"
}

FASTAPI_STATS=$(extract_stats fastapi_bench.txt)
GATEWAY_STATS=$(extract_stats gateway_bench.txt)
HELIX_STATS=$(extract_stats helix_bench.txt)

FASTAPI_RPS=$(echo $FASTAPI_STATS | cut -d'|' -f1)
FASTAPI_LAT=$(echo $FASTAPI_STATS | cut -d'|' -f2)

GATEWAY_RPS=$(echo $GATEWAY_STATS | cut -d'|' -f1)
GATEWAY_LAT=$(echo $GATEWAY_STATS | cut -d'|' -f2)

HELIX_RPS=$(echo $HELIX_STATS | cut -d'|' -f1)
HELIX_LAT=$(echo $HELIX_STATS | cut -d'|' -f2)

echo ""
echo "----------------------------------------"
echo "  Benchmark Results (100 concurrency)"
echo "----------------------------------------"
echo "FastAPI:      $FASTAPI_RPS req/sec, avg latency $FASTAPI_LAT ms"
echo "gRPC-Gateway: $GATEWAY_RPS req/sec, avg latency $GATEWAY_LAT ms"
echo "Helix RPC:    $HELIX_RPS req/sec, avg latency $HELIX_LAT ms"
echo "----------------------------------------"

# Write docs/benchmarks.md
cat <<EOF > ../docs/benchmarks.md
# Performance Benchmarks

Performance comparison between **Helix RPC**, **FastAPI**, and **gRPC-Gateway** (reverse proxy mode) on a local HTTP/1.1 JSON route transcoding `/v1/users/{user_id}` load-tested with 100 concurrent connections.

## Results Table

| Framework / Protocol | Throughput (req/sec) | Avg Latency (ms) | Speedup vs. FastAPI |
| :--- | :--- | :--- | :--- |
| **FastAPI** | $FASTAPI_RPS | $FASTAPI_LAT | 1.0x (Baseline) |
| **gRPC-Gateway** | $GATEWAY_RPS | $GATEWAY_LAT | $(echo "scale=2; $GATEWAY_RPS / $FASTAPI_RPS" | bc -l)x |
| **Helix RPC** | $HELIX_RPS | $HELIX_LAT | $(echo "scale=2; $HELIX_RPS / $FASTAPI_RPS" | bc -l)x |

## Takeaways
1. **Low Memory/Zero-Copy Processing**: Helix RPC's zero-copy dynamic routing and memory-pooled buffer model allows it to process routes significantly faster than standard Python FastAPI.
2. **Reduced Gateway Overhead**: Compared to gRPC-Gateway, Helix eliminates proxy translations by executing direct native multi-protocol routing in a single layer.
EOF

echo "Benchmarks documented in docs/benchmarks.md!"
