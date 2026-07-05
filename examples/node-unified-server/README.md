# Helix RPC - Node.js Unified Server Example

This is a zero-infrastructure, end-to-end example showing how to run a Helix RPC Same-Port Sniffing Server in Node.js.

The server sniffs incoming traffic, routes JSON/REST requests, binds path variables, and executes the target service handlers defined in the Protobuf IDL.

---

## Prerequisites
- Node.js installed locally.

---

## How to Run

### 1. Install dependencies and compile
```bash
npm install
npm run build
```

### 2. Start the Unified Server
```bash
npm start
```
This runs the compiled `server.js` listening on `127.0.0.1:9090`.

### 3. Run the Client
In a new terminal window:
```bash
npm run client
```

This sends:
- A `POST` request to `/v1/kv` with payload `{ "key": "hello", "value": "Unified RPC & REST works!" }`
- A `GET` request to `/v1/kv/hello` to retrieve it.

---

## 🌐 Testing the 3 Protocols

Helix sniffs incoming traffic on port `9090` and routes it dynamically to the corresponding decoder/handler depending on the protocol preamble:

### 1. JSON over HTTP (REST)
Test this by making a curl call (or using the Dashboard):
```bash
curl -X POST http://127.0.0.1:9090/v1/kv \
  -H "Content-Type: application/json" \
  -d '{"key": "test_key", "value": "Hello REST!"}'
```

### 2. gRPC / HTTP/2 (Protobuf)
Helix detects the HTTP/2 preface and routes to standard gRPC. Test this using `grpcurl`:
```bash
grpcurl -plaintext -d '{"key": "test_key"}' 127.0.0.1:9090 keyval.KVService/Get
```

### 3. Apache Thrift Compact
Thrift clients can connect directly to the same port `9090`. Under the hood, Helix transpiles the Thrift compact structure to the target Protobuf model with **zero allocations** in memory before handing it off to the service function.

