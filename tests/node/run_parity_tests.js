import { HelixServer, TokenBucketRateLimiter, BatchScheduler, decodeAndVerifyJWT, withRetries, CircuitBreaker, RoundRobinBalancer, loadBpfSockmap, hasUnixPrefix, stripUnixPrefix } from 'helix-rt-node';
import { UserProfile, LazyUserProfile } from './generated.js';
import * as http from 'http';
function assert(condition, message) {
    if (!condition) {
        throw new Error(message || "Assertion failed");
    }
}
async function runParityTests() {
    console.log("--- 1. Testing JWT Token Verification ---");
    const secret = "supersecret";
    const payload = { sub: "1234", exp: Math.floor(Date.now() / 1000) + 10 };
    // Build a simple mock JWT
    const headerB64 = Buffer.from(JSON.stringify({ alg: "HS256", typ: "JWT" })).toString('base64url');
    const payloadB64 = Buffer.from(JSON.stringify(payload)).toString('base64url');
    const crypto = await import('crypto');
    const signature = crypto
        .createHmac('sha256', secret)
        .update(`${headerB64}.${payloadB64}`)
        .digest('base64url');
    const token = `${headerB64}.${payloadB64}.${signature}`;
    const verified = decodeAndVerifyJWT(token, secret);
    assert(verified !== null, "JWT should be verified successfully");
    assert(verified.sub === "1234", "Subject claim should match");
    const failedVerified = decodeAndVerifyJWT(token, "wrongsecret");
    assert(failedVerified === null, "JWT should fail verification with wrong secret");
    console.log("✅ JWT tests passed!");
    console.log("--- 2. Testing Token Bucket Rate Limiter ---");
    const limiter = new TokenBucketRateLimiter(2, 10); // 2 capacity, 10 refills per sec
    assert(limiter.allow() === true, "First request allowed");
    assert(limiter.allow() === true, "Second request allowed");
    assert(limiter.allow() === false, "Third request should be rate-limited");
    console.log("✅ Rate limiter tests passed!");
    console.log("--- 3. Testing Dynamic Batch Scheduler ---");
    const mockBatchHandler = async (reqs) => {
        return reqs.map(r => r * 2);
    };
    const scheduler = new BatchScheduler(mockBatchHandler, 3, 5);
    const p1 = scheduler.submit(1);
    const p2 = scheduler.submit(2);
    const p3 = scheduler.submit(3);
    const results = await Promise.all([p1, p2, p3]);
    assert(results[0] === 2 && results[1] === 4 && results[2] === 6, "Batch items processed correctly");
    console.log("✅ Batching scheduler tests passed!");
    console.log("--- 4. Testing Exponential Backoff Retry ---");
    let attempts = 0;
    const failingFn = async () => {
        attempts++;
        if (attempts < 3) {
            throw new Error("Temporary failure");
        }
        return "success";
    };
    const finalResult = await withRetries(failingFn, {
        maxAttempts: 4,
        initialBackoffMs: 10,
        maxBackoffMs: 100,
        multiplier: 2
    });
    assert(finalResult === "success", "Fn should eventually succeed");
    assert(attempts === 3, "Fn should be called 3 times");
    console.log("✅ Retry tests passed!");
    console.log("--- 5. Testing Sniffing Server ---");
    const server = new HelixServer("127.0.0.1:0");
    // Register a mock method path
    server.registerMethod("/helix_example.UserProfileService/GetUserProfile", {
        Decoder: (dec) => {
            const req = new UserProfile();
            dec(req);
            return req;
        },
        Handler: async (ctx, req) => {
            return new UserProfile({
                userId: req.userId,
                username: req.username + "-node-response",
                email: req.email
            });
        }
    });
    // Register a REST route
    server.registerRESTRoute("POST", "/v1/users", "/helix_example.UserProfileService/GetUserProfile");
    await server.start();
    const addrStr = server.getAddr();
    const [host, port] = addrStr.split(':');
    // Make an HTTP/1.1 POST request to trigger the sniffer + router
    const postData = JSON.stringify({ userId: 777, username: "node_user", email: "node@test.com" });
    const reqOptions = {
        hostname: host,
        port: parseInt(port),
        path: '/v1/users',
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Content-Length': Buffer.byteLength(postData)
        }
    };
    const responseBody = await new Promise((resolve, reject) => {
        const req = http.request(reqOptions, (res) => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => resolve(data));
        });
        req.on('error', reject);
        req.write(postData);
        req.end();
    });
    const parsedResponse = JSON.parse(responseBody);
    assert(parsedResponse.userId === 777, "Response userId should be 777");
    assert(parsedResponse.username === "node_user-node-response", "Response username should be node_user-node-response");
    server.shutdown();
    console.log("✅ Sniffing server tests passed!");
    console.log("--- 6. Testing Circuit Breaker ---");
    const cb = new CircuitBreaker(2, 50); // 2 failures, 50ms timeout
    let count = 0;
    const task = async () => {
        count++;
        if (count <= 2)
            throw new Error("fail");
        return "ok";
    };
    try {
        await cb.execute(task);
    }
    catch (e) { }
    try {
        await cb.execute(task);
    }
    catch (e) { }
    // Now circuit should be open
    try {
        await cb.execute(task);
        assert(false, "Should have failed on open circuit");
    }
    catch (e) {
        assert(e.message === "circuit breaker is open", "Should be open circuit error");
    }
    console.log("✅ Circuit breaker tests passed!");
    console.log("--- 7. Testing Round-Robin Balancer ---");
    const mockClient = (val) => ({
        invoke: async () => val
    });
    const balancer = new RoundRobinBalancer([mockClient("A"), mockClient("B")]);
    assert(await balancer.next().invoke("", {}) === "A");
    assert(await balancer.next().invoke("", {}) === "B");
    assert(await balancer.next().invoke("", {}) === "A");
    console.log("✅ Round-Robin balancer tests passed!");
    console.log("--- 8. Testing Health Checking & SSE Streaming Server ---");
    const sseServer = new HelixServer("127.0.0.1:0");
    sseServer.registerMethod("/stream", {
        Decoder: (d) => d({}),
        Handler: async () => {
            return (async function* () {
                yield "hello";
                yield "world";
            })();
        }
    });
    sseServer.registerRESTRoute("POST", "/stream", "/stream");
    await sseServer.start();
    const sseAddr = sseServer.getAddr();
    const ssePort = parseInt(sseAddr.split(':')[1]);
    // Test health check endpoint
    const healthResp = await new Promise((resolve) => {
        http.request({ hostname: "127.0.0.1", port: ssePort, path: "/grpc.health.v1.Health/Check", method: "POST" }, (res) => {
            let data = "";
            res.on("data", chunk => data += chunk);
            res.on("end", () => resolve(data));
        }).end("{}");
    });
    console.log("Health check response:", healthResp);
    assert(JSON.parse(healthResp).status === 1, "Health check should return status 1 (SERVING)");
    console.log("✅ Health check endpoint verified!");
    // Test SSE Stream endpoint
    const sseStream = await new Promise((resolve) => {
        http.request({ hostname: "127.0.0.1", port: ssePort, path: "/stream", method: "POST" }, (res) => {
            let data = "";
            res.on("data", chunk => data += chunk);
            res.on("end", () => resolve(data));
        }).end("{}");
    });
    assert(sseStream.includes("hello") && sseStream.includes("world"), "SSE stream should contain chunk outputs");
    sseServer.shutdown();
    console.log("✅ Health check & SSE stream server tests passed!");
    console.log("--- 9. Testing Advanced Performance Optimizations ---");
    // Build a sample Protobuf-encoded UserProfile
    const enc2 = new TextEncoder();
    const usernameBytes = enc2.encode('zero_copy_hero');
    const emailBytes = enc2.encode('hero@helix.rpc');
    const protoInput = new Uint8Array([
        0x08, 42,
        0x12, usernameBytes.length, ...usernameBytes,
        0x1A, emailBytes.length, ...emailBytes,
    ]);
    // Test LazyUserProfile
    const lazy = new LazyUserProfile(protoInput);
    assert(lazy.getUserId() === 42, `LazyUserProfile.getUserId() should be 42, got ${lazy.getUserId()}`);
    assert(lazy.getUsername() === 'zero_copy_hero', `LazyUserProfile.getUsername() should be 'zero_copy_hero', got '${lazy.getUsername()}'`);
    assert(lazy.getEmail() === 'hero@helix.rpc', `LazyUserProfile.getEmail() should be 'hero@helix.rpc', got '${lazy.getEmail()}'`);
    // Test transpileProtobufToThriftCompact
    const thriftBytes = UserProfile.transpileProtobufToThriftCompact(protoInput);
    assert(thriftBytes instanceof Uint8Array, 'transpile output should be Uint8Array');
    assert(thriftBytes.length > 0, 'transpile output should not be empty');
    assert(thriftBytes[thriftBytes.length - 1] === 0x00, 'last byte must be Thrift STOP (0x00)');
    // Verify string contents appear verbatim in Thrift output
    const thriftStr = Buffer.from(thriftBytes).toString('latin1');
    assert(thriftStr.includes('zero_copy_hero'), 'username should appear verbatim in Thrift compact output');
    assert(thriftStr.includes('hero@helix.rpc'), 'email should appear verbatim in Thrift compact output');
    // Test eBPF helpers from runtime
    assert(hasUnixPrefix('unix:///tmp/helix.sock') === true, 'hasUnixPrefix should return true for unix:// scheme');
    assert(hasUnixPrefix('127.0.0.1:9090') === false, 'hasUnixPrefix should return false for TCP addr');
    assert(stripUnixPrefix('unix:///tmp/helix.sock') === '/tmp/helix.sock', 'stripUnixPrefix should strip unix:// prefix');
    assert(stripUnixPrefix('127.0.0.1:9090') === '127.0.0.1:9090', 'stripUnixPrefix should not change TCP addr');
    // loadBpfSockmap should return false on non-Linux
    const ebpfResult = loadBpfSockmap('127.0.0.1:9090');
    assert(typeof ebpfResult === 'boolean', 'loadBpfSockmap should return boolean');
    // On non-Linux, should be false (graceful fallback)
    if (process.platform !== 'linux') {
        assert(ebpfResult === false, 'loadBpfSockmap should return false on non-Linux');
    }
    console.log("--- 10. Testing gRPC-Web and gRPC-Web-Text ---");
    const webServer = new HelixServer("127.0.0.1:0");
    webServer.registerMethod("/test.Service/SayHello", {
        Decoder: (dec) => {
            const req = { name: "" };
            dec(req);
            return req;
        },
        Handler: async (ctx, req) => {
            return { message: `Hello ${req.name}` };
        }
    });
    await webServer.start();
    const webAddr = webServer.getAddr();
    const webPort = parseInt(webAddr.split(':')[1]);
    // Construct gRPC-Web payload
    // format: 1 byte compressed-flag (0), 4 bytes length, then data
    const requestJsonStr = JSON.stringify({ name: "World" });
    const payloadBytes = Buffer.from(requestJsonStr, 'utf8');
    const headerBytes = Buffer.alloc(5);
    headerBytes[0] = 0;
    headerBytes.writeUInt32BE(payloadBytes.length, 1);
    const grpcWebPayload = Buffer.concat([headerBytes, payloadBytes]);
    // Test 10a: application/grpc-web
    const responseBinary = await new Promise((resolve, reject) => {
        const req = http.request({
            hostname: "127.0.0.1",
            port: webPort,
            path: "/test.Service/SayHello",
            method: "POST",
            headers: {
                'Content-Type': 'application/grpc-web',
                'Content-Length': grpcWebPayload.length
            }
        }, (res) => {
            assert(res.headers['content-type'] === 'application/grpc-web', `Expected content-type application/grpc-web, got ${res.headers['content-type']}`);
            const chunks = [];
            res.on('data', chunk => chunks.push(chunk));
            res.on('end', () => resolve(Buffer.concat(chunks)));
        });
        req.on('error', reject);
        req.write(grpcWebPayload);
        req.end();
    });
    // Verify binary response
    // Message frame length
    assert(responseBinary.length >= 5, "Response too short");
    const respCompressed = responseBinary[0];
    assert(respCompressed === 0, "Expected compressed flag to be 0");
    const respLen = responseBinary.readUInt32BE(1);
    const respPayload = responseBinary.subarray(5, 5 + respLen);
    const decodedResp = JSON.parse(respPayload.toString('utf8'));
    assert(decodedResp.message === "Hello World", `Expected "Hello World", got ${decodedResp.message}`);
    // Trailer frame
    const trailerOffset = 5 + respLen;
    assert(responseBinary.length >= trailerOffset + 5, "Trailer frame missing or too short");
    const trailerType = responseBinary[trailerOffset];
    assert(trailerType === 0x80, `Expected trailer type 0x80, got ${trailerType}`);
    const trailerLen = responseBinary.readUInt32BE(trailerOffset + 1);
    const trailerPayload = responseBinary.subarray(trailerOffset + 5, trailerOffset + 5 + trailerLen);
    const trailerStr = trailerPayload.toString('ascii');
    assert(trailerStr.includes("grpc-status: 0"), `Expected grpc-status: 0 in trailer, got ${trailerStr}`);
    // Test 10b: application/grpc-web-text
    const grpcWebTextPayload = grpcWebPayload.toString('base64');
    const responseText = await new Promise((resolve, reject) => {
        const req = http.request({
            hostname: "127.0.0.1",
            port: webPort,
            path: "/test.Service/SayHello",
            method: "POST",
            headers: {
                'Content-Type': 'application/grpc-web-text',
                'Content-Length': Buffer.byteLength(grpcWebTextPayload)
            }
        }, (res) => {
            assert(res.headers['content-type'] === 'application/grpc-web-text', `Expected content-type application/grpc-web-text, got ${res.headers['content-type']}`);
            let data = "";
            res.on('data', chunk => data += chunk);
            res.on('end', () => resolve(data));
        });
        req.on('error', reject);
        req.write(grpcWebTextPayload);
        req.end();
    });
    // Verify base64 decoded response matches binary response
    const responseTextBinary = Buffer.from(responseText, 'base64');
    assert(responseTextBinary.equals(responseBinary), "Decoded grpc-web-text response should be equal to binary response");
    webServer.shutdown();
    console.log("✅ gRPC-Web and gRPC-Web-Text tests passed!");
    console.log("🎉 ALL PARITY TESTS COMPLETED SUCCESSFULLY!");
}
runParityTests().catch(err => {
    console.error("❌ Test run failed:", err);
    process.exit(1);
});
