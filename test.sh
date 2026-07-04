#!/usr/bin/env bash
set -eo pipefail

echo "========================================="
echo "   Helix RPC - Integration Test Harness  "
echo "========================================="

echo "[1/5] Running Go Runtime Unit Tests (with -race)..."
cd runtimes/go
go test -v -race ./...
cd ../..

echo "[2/5] Running Rust Runtime Checks (Clippy & Tests)..."
cd runtimes/rust
cargo clippy -- -D warnings
cargo test
cd ../..

echo "[3/5] Testing Compiler & Python Code Generation..."
cd compiler
go build -o helix-gen .
./helix-gen -idl ../tests/schema/chat_completion.proto -lang python -out ../tests/schema/generated.py
if [ $? -ne 0 ]; then
    echo "❌ Python Code Generation Failed"
    exit 1
fi
echo "✅ Python codegen succeeded!"
cd ..

echo "[4/5] Running Go-Go & Cross-Language Matrix Tests (with -race)..."
cd tests/go
go test -v -race ./...
cd ../..

echo "[4/5] Running Rust-Rust Matrix Tests..."
cd tests/rust
cargo test
cd ../..

echo "[5/5] Running Node.js E2E Parity Tests..."
cd tests/node
npm run test
cd ../..

echo "========================================="
echo "   ALL TESTS PASSED SUCCESSFULLY!        "
echo "========================================="
