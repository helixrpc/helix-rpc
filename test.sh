#!/usr/bin/env bash
set -eo pipefail

echo "========================================="
echo "   Helix RPC - Integration Test Harness  "
echo "========================================="

echo "[1/8] Running Go Runtime Unit Tests (with -race)..."
cd runtimes/go
go test -v -race ./...
cd ../..

echo "[2/8] Running Rust Runtime Checks (Clippy & Tests)..."
cd runtimes/rust
cargo clippy -- -D warnings
cargo test
cd ../..

echo "[3/8] Running Python Runtime Tests..."
cd runtimes/python
if [ -d venv ]; then source venv/bin/activate; fi
python -m pip install --upgrade pip
pip install pytest pytest-asyncio aiohttp opentelemetry-api opentelemetry-sdk aiomcache redis aio-pika aiokafka
python -m pytest tests/test_cache.py tests/test_ratelimit.py tests/test_rabbitmq.py tests/test_kafka.py -v
cd ../..

echo "[4/8] Running Java Runtime Tests..."
cd runtimes/java
mvn clean compile
cd ../..

echo "[5/8] Running C++ Runtime Tests..."
./compiler/bin/helixc generate -idl tests/schema/test.proto -lang cpp -out runtimes/cpp/tests/test_helix_generated.h
cd runtimes/cpp
mkdir -p build && cd build
cmake ..
cmake --build .
if [ -f "Debug/test_runtime.exe" ]; then
  ./Debug/test_runtime.exe
  ./Debug/test_optimization.exe
else
  ./test_runtime
  ./test_optimization
fi
cd ../../..

echo "[6/8] Testing Compiler & Python Code Generation..."
cd compiler
go build -o helix-gen .
./helix-gen -idl ../tests/schema/chat_completion.proto -lang python -out ../tests/schema/generated.py
if [ $? -ne 0 ]; then
    echo "❌ Python Code Generation Failed"
    exit 1
fi
echo "✅ Python codegen succeeded!"
cd ..

echo "[7/8] Running Go-Go & Cross-Language Matrix Tests (with -race)..."
cd tests/go
go test -v -race ./...
cd ../..

echo "[7/8] Running Rust-Rust Matrix Tests..."
cd tests/rust
cargo test
cd ../..

echo "[8/8] Running Node.js E2E Parity Tests..."
cd tests/node
npm run test
cd ../..

echo "========================================="
echo "   ALL TESTS PASSED SUCCESSFULLY!        "
echo "========================================="
