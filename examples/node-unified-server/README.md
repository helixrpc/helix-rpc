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
