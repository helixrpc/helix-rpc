# Contributing to Helix RPC 🧬

We welcome contributions of all kinds! Whether you are writing code, improving documentation, opening issues, or suggesting new protocols, your help is appreciated.

---

## Code of Conduct
By participating in this project, you agree to abide by our code of conduct. Please treat all contributors with respect.

---

## 🛠 Setting Up Your Development Environment

To compile and verify all components of Helix RPC, you will need:
- **Go 1.23+**
- **Rust (Stable)**
- **Python 3.12+**
- **Protoc** (Protobuf compiler, version 23+)

### 1. Clone the Repository
```bash
git clone https://github.com/helixrpc/helix-rpc.git
cd helix-rpc
```

### 2. Verify System Dependencies
Ensure all runtimes are installed and in your environment path:
```bash
go version
cargo --version
python3 --version
protoc --version
```

---

## 🧪 Running the Test Harness

Helix RPC includes a unified cross-language test harness that compiles the Go/Rust runtimes, runs the Python code generators, and launches end-to-end integration tests (multiplexing, dynamic batching, compression, auth, and service discovery).

Before submitting a pull request, ensure all tests pass locally:
```bash
chmod +x ./test.sh
./test.sh
```

---

## 🎨 Coding Style Guidelines

To keep the codebase clean and maintainable, please format your code before committing:

### Go
Use the standard formatting and liveness checkers:
```bash
cd runtime-go
go fmt ./...
go vet ./...
```

### Rust
Ensure cargo formatting is applied and clippy checks pass without warnings:
```bash
cd runtime-rust
cargo fmt
cargo clippy -- -D warnings
```

### Python
Use PEP 8 compliant formatting:
```bash
cd runtime-python
python3 -m pip install black flake8
black helix_rt/ tests/
flake8 helix_rt/ tests/
```

---

## 🚀 Proposing Major Features (RFC Process)

Because Helix RPC supports multiple languages and protocols under a single core compiler, architectural changes must be coordinated.

For any major change (e.g. adding a new serialization format, adding a language runtime, or extending the AST), please open an issue with the prefix `[RFC]` detailing:
1. **Problem Statement:** What limitation or use-case are you addressing?
2. **Proposed Specification:** How does the schema change? How does this impact Go, Rust, and Python runtime layers?
3. **Backward Compatibility:** How does the compiler handle older schema versions?
