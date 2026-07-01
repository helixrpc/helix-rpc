# Technical Architecture

Helix RPC employs a dual-language architecture leveraging the concurrency of Go and the zero-overhead memory safety of Rust.

## The Go Runtime (`helix-rt`)
The Go runtime is optimized for extreme concurrency and horizontal scaling. It uses `golang.org/x/net/http2/h2c` to seamlessly multiplex gRPC HTTP/2 calls alongside standard REST HTTP/1.1 calls on the exact same port.

It heavily utilizes **Interceptors** to perform:
1. Dynamic Request Batching
2. Deadline Propagation (Timeout handling)
3. Health Checking
4. Payload Compression

## The Rust Runtime (`helix_rt`)
The Rust runtime is optimized for vertical scaling and deep system integration. It utilizes `tokio` and `hyper` to handle massive IO workloads.

### The PyO3 Bridge (Zero-Serialization)
In standard AI deployments, a Go server must serialize JSON/gRPC, send it over a Unix Socket to a Python server, which then parses the JSON/gRPC. 
Helix RPC's Rust runtime circumvents this entirely by compiling against `libpython`. 

Using the `pyo3` crate, the Rust binary **embeds the CPython interpreter directly inside itself**.
When an HTTP request comes in, Rust passes the memory pointer of the prompt string directly to the Python function `generate_stream()`. There is zero network hop and zero serialization overhead.

### SSE Streaming via MPSC
When a Python Generator yields a token, the Global Interpreter Lock (GIL) is involved. To prevent blocking the entire async web server, Helix spawns a `tokio::task::spawn_blocking` thread that iterates the Python generator. 
Each yielded string token is sent down a `tokio::sync::mpsc` channel. 
The main async HTTP task reads from this channel, transcodes the raw string into OpenAI-formatted JSON chunks (`data: {"choices":[{"delta":{"content":"..."}}]}`), and streams it to the client over Server-Sent Events (SSE).
