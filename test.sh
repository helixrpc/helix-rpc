#!/usr/bin/env bash
set -eo pipefail

echo "========================================="
echo "   Helix RPC - Integration Test Harness  "
echo "========================================="

echo "[1/4] Running Go Runtime Unit Tests (with -race)..."
cd runtime-go
go test -v -race ./...
cd ..

echo "[2/4] Running Rust Runtime Checks (Clippy & Tests)..."
cd runtime-rust
cargo clippy -- -D warnings
cargo test
cd ..

echo "[3/4] Running Go-Go & Cross-Language Matrix Tests (with -race)..."
cd integration-tests/go-go
go test -v -race ./...
cd ../..

echo "[4/4] Running Rust-Rust Matrix Tests..."
cd integration-tests/rust-rust
cargo test
cd ../..

echo "========================================="
echo "   ALL TESTS PASSED SUCCESSFULLY!        "
echo "========================================="
