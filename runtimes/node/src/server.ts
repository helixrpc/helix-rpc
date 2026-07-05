import * as net from 'net';
import * as http from 'http';
import * as http2 from 'http2';
import * as zlib from 'zlib';
import * as fs from 'fs';
import { globalRegistry } from './metrics.js';
import { getOrCreateTraceId, logStructured } from './telemetry.js';

export interface MethodInfo {
    Decoder: (dec: (val: any) => void) => any;
    Handler: (ctx: any, req: any) => Promise<any>;
    Binder?: (req: any, params: Record<string, string>) => void;
}

export interface RESTRoute {
    method: string;
    pattern: string;
    handlerPath: string;
}

export class HelixServer {
    private addr: string;
    private port: number = 0;
    private methods: Map<string, MethodInfo> = new Map();
    private restRoutes: RESTRoute[] = [];
    private tcpServer: net.Server | null = null;
    private httpServer1: http.Server | null = null;
    private httpServer2: http2.Http2Server | null = null;
    private requestBufferPool = new BufferPool(1024 * 1024, 10);
    private responseBufferPool = new BufferPool(1024 * 1024, 10);
    private headerBufferPool = new BufferPool(5, 10);

    constructor(addr: string) {
        this.addr = addr;
        // Default health checking service
        this.registerMethod('/grpc.health.v1.Health/Check', {
            Decoder: (dec) => {
                const req = { service: "" };
                dec(req);
                return req;
            },
            Handler: async (ctx, req) => {
                return { status: 1 }; // SERVING
            }
        });
        this.registerRESTRoute('POST', '/grpc.health.v1.Health/Check', '/grpc.health.v1.Health/Check');
        this.registerRESTRoute('GET', '/grpc.health.v1.Health/Check', '/grpc.health.v1.Health/Check');
    }

    public registerMethod(path: string, info: MethodInfo) {
        this.methods.set(path, info);
    }

    public registerRESTRoute(method: string, pattern: string, handlerPath: string) {
        this.restRoutes.push({ method: method.toUpperCase(), pattern, handlerPath });
    }

    public getAddr(): string {
        const host = this.addr.split(':')[0];
        return `${host}:${this.port}`;
    }

    public async start(): Promise<void> {
        // Start local internal servers
        this.httpServer1 = http.createServer((req, res) => this.handleHTTP1(req, res));
        this.httpServer1.on('connection', (socket) => {
            socket.setKeepAlive(true, 1000);
        });
        await new Promise<void>(resolve => this.httpServer1!.listen(0, '127.0.0.1', resolve));
        const http1Addr = this.httpServer1.address() as net.AddressInfo;

        this.httpServer2 = http2.createServer();
        this.httpServer2.on('connection', (socket) => {
            socket.setKeepAlive(true, 1000);
        });
        this.httpServer2.on('stream', (stream, headers) => this.handleHTTP2(stream as http2.ServerHttp2Stream, headers));
        await new Promise<void>(resolve => this.httpServer2!.listen(0, '127.0.0.1', resolve));
        const http2Addr = this.httpServer2.address() as net.AddressInfo;

        const isUnix = this.addr.startsWith('unix://');
        this.tcpServer = net.createServer((socket) => {
            socket.setKeepAlive(true, 1000);
            socket.once('data', (chunk) => {
                const preface = chunk.toString('utf8', 0, Math.min(chunk.length, 24));
                let targetAddr = http1Addr; // fallback to HTTP/1.1

                if (preface.startsWith('PRI * HTTP/2.0')) {
                    targetAddr = http2Addr;
                } else if (preface.startsWith('POST ') || preface.startsWith('GET ') || preface.startsWith('OPTIONS ')) {
                    targetAddr = http1Addr;
                }

                // Proxy socket to local target
                const targetSocket = net.connect(targetAddr.port, targetAddr.address, () => {
                    targetSocket.setKeepAlive(true, 1000);
                    targetSocket.write(chunk);
                    socket.pipe(targetSocket).pipe(socket);
                });

                targetSocket.on('error', () => socket.destroy());
                socket.on('error', () => targetSocket.destroy());
            });
        });

        if (isUnix) {
            const path = this.addr.slice(7);
            try { fs.unlinkSync(path); } catch (e) {}
            await new Promise<void>(resolve => this.tcpServer!.listen(path, resolve));
            fs.chmodSync(path, 0o600);
            this.port = 0;
        } else {
            const parts = this.addr.split(':');
            const host = parts[0];
            const portVal = parseInt(parts[1] || '0');
            await new Promise<void>(resolve => this.tcpServer!.listen(portVal, host, resolve));
            this.port = (this.tcpServer.address() as net.AddressInfo).port;
        }
    }

    public shutdown() {
        this.tcpServer?.close();
        this.httpServer1?.close();
        this.httpServer2?.close();
    }

    private async handleHTTP1(req: http.IncomingMessage, res: http.ServerResponse) {
        res.setHeader('Access-Control-Allow-Origin', '*');
        res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
        res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, grpc-timeout');

        const urlObj = new URL(req.url || '', `http://${req.headers.host}`);
        const path = urlObj.pathname;
        const method = req.method || 'GET';

        if (method === 'OPTIONS') {
            res.writeHead(204);
            res.end();
            return;
        }

        // Prometheus exporter endpoint
        if (path === '/metrics') {
            res.writeHead(200, { 'Content-Type': 'text/plain' });
            res.end(globalRegistry.exportPrometheus());
            return;
        }

        const contentType = req.headers['content-type'] || '';
        const isGrpcWebText = contentType.startsWith('application/grpc-web-text');
        const isGrpcWeb = isGrpcWebText || contentType.startsWith('application/grpc-web');

        if (isGrpcWeb) {
            const methodInfo = this.methods.get(path);
            if (!methodInfo) {
                const respContentType = isGrpcWebText ? 'application/grpc-web-text' : 'application/grpc-web';
                const trailersStr = 'grpc-status: 12\r\ngrpc-message: unimplemented\r\n';
                const trailersPayload = Buffer.from(trailersStr, 'ascii');
                const trailersHeader = Buffer.alloc(5);
                trailersHeader[0] = 0x80;
                trailersHeader.writeUInt32BE(trailersPayload.length, 1);
                const trailersFrame = Buffer.concat([trailersHeader, trailersPayload]);
                
                let responseBody: Buffer | string = trailersFrame;
                if (isGrpcWebText) {
                    responseBody = trailersFrame.toString('base64');
                }
                res.writeHead(200, { 'Content-Type': respContentType });
                res.end(responseBody);
                return;
            }

            const chunks: Buffer[] = [];
            let totalLength = 0;
            req.on('data', chunk => {
                chunks.push(chunk);
                totalLength += chunk.length;
            });
            req.on('end', async () => {
                try {
                    let bodyBytes: Buffer;
                    if (isGrpcWebText) {
                        const fullBuf = Buffer.concat(chunks);
                        bodyBytes = Buffer.from(fullBuf.toString('utf8'), 'base64');
                    } else {
                        bodyBytes = Buffer.concat(chunks);
                    }

                    if (bodyBytes.length < 5) {
                        throw new Error('grpc frame too small');
                    }
                    const compressedFlag = bodyBytes[0];
                    const len = bodyBytes.readUInt32BE(1);
                    let payload = bodyBytes.subarray(5, 5 + len);

                    if (compressedFlag === 1) {
                        payload = zlib.gunzipSync(payload);
                    }

                    let decodedReq: any;
                    if (payload[0] === 4) {
                        decodedReq = methodInfo.Decoder((target: any) => {
                            if (target.constructor && typeof target.constructor.unmarshalFlatBuffers === 'function') {
                                Object.assign(target, target.constructor.unmarshalFlatBuffers(new Uint8Array(payload)));
                            }
                        });
                    } else {
                        const str = payload.toString('utf8');
                        const json = str ? JSON.parse(str) : {};
                        decodedReq = methodInfo.Decoder((target: any) => {
                            Object.assign(target, json);
                        });
                    }

                    const traceId = getOrCreateTraceId(req.headers);
                    globalRegistry.recordRequest(path);
                    const startTime = Date.now();

                    const resp = await methodInfo.Handler({ traceId }, decodedReq);

                    const duration = Date.now() - startTime;
                    globalRegistry.recordLatency(path, duration);
                    logStructured('INFO', 'gRPC-Web request processed', {
                        method: path,
                        latencyMs: duration,
                        traceId
                    });

                    let respBytes: Uint8Array;
                    if (resp.marshalFlatBuffers && typeof resp.marshalFlatBuffers === 'function') {
                        respBytes = resp.marshalFlatBuffers();
                    } else {
                        const respStr = JSON.stringify(resp);
                        respBytes = Buffer.from(respStr, 'utf8');
                    }

                    let finalBytes = respBytes;
                    let finalCompressed = 0;
                    if (req.headers['grpc-accept-encoding'] && req.headers['grpc-accept-encoding'].toString().includes('gzip')) {
                        finalBytes = zlib.gzipSync(respBytes);
                        finalCompressed = 1;
                    }

                    const messageHeader = Buffer.alloc(5);
                    messageHeader[0] = finalCompressed;
                    messageHeader.writeUInt32BE(finalBytes.length, 1);
                    const messageFrame = Buffer.concat([messageHeader, Buffer.from(finalBytes)]);

                    const trailersStr = 'grpc-status: 0\r\ngrpc-message: \r\n';
                    const trailersPayload = Buffer.from(trailersStr, 'ascii');
                    const trailersHeader = Buffer.alloc(5);
                    trailersHeader[0] = 0x80;
                    trailersHeader.writeUInt32BE(trailersPayload.length, 1);
                    const trailersFrame = Buffer.concat([trailersHeader, trailersPayload]);

                    const combinedBuffer = Buffer.concat([messageFrame, trailersFrame]);

                    let responseBody: Buffer | string = combinedBuffer;
                    if (isGrpcWebText) {
                        responseBody = combinedBuffer.toString('base64');
                    }

                    const respContentType = isGrpcWebText ? 'application/grpc-web-text' : 'application/grpc-web';
                    res.writeHead(200, { 'Content-Type': respContentType });
                    res.end(responseBody);
                } catch (err: any) {
                    try {
                        const trailersStr = `grpc-status: 13\r\ngrpc-message: ${err.message || 'unknown'}\r\n`;
                        const trailersPayload = Buffer.from(trailersStr, 'ascii');
                        const trailersHeader = Buffer.alloc(5);
                        trailersHeader[0] = 0x80;
                        trailersHeader.writeUInt32BE(trailersPayload.length, 1);
                        const trailersFrame = Buffer.concat([trailersHeader, trailersPayload]);

                        let responseBody: Buffer | string = trailersFrame;
                        if (isGrpcWebText) {
                            responseBody = trailersFrame.toString('base64');
                        }

                        const respContentType = isGrpcWebText ? 'application/grpc-web-text' : 'application/grpc-web';
                        res.writeHead(200, { 'Content-Type': respContentType });
                        res.end(responseBody);
                    } catch (innerErr) {
                        res.writeHead(500, { 'Content-Type': 'application/json' });
                        res.end(JSON.stringify({ error: err.message }));
                    }
                }
            });
            return;
        }

        const match = matchREST(method, path, this.restRoutes);
        if (!match) {
            res.writeHead(404, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ error: `Not found: ${method} ${path}` }));
            return;
        }

        const methodInfo = this.methods.get(match.route.handlerPath);
        if (!methodInfo) {
            res.writeHead(501, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ error: 'Unimplemented method handler' }));
            return;
        }

        // Buffer the request body
        const chunks: Buffer[] = [];
        let totalLength = 0;
        req.on('data', chunk => {
            chunks.push(chunk);
            totalLength += chunk.length;
        });
        req.on('end', async () => {
            let bodyBytes: Buffer | null = null;
            let rentedReqBuf: Buffer | null = null;
            let rentedRespBuf: Buffer | null = null;
            try {
                if (totalLength <= 1024 * 1024) {
                    rentedReqBuf = this.requestBufferPool.acquire();
                    let offset = 0;
                    for (const chunk of chunks) {
                        chunk.copy(rentedReqBuf, offset);
                        offset += chunk.length;
                    }
                    bodyBytes = rentedReqBuf.subarray(0, totalLength);
                } else {
                    bodyBytes = Buffer.concat(chunks);
                }
                const reqJson = bodyBytes.length > 0 ? JSON.parse(bodyBytes.toString('utf8')) : {};

                if (rentedReqBuf) {
                    this.requestBufferPool.release(rentedReqBuf);
                    rentedReqBuf = null;
                }

                // Path & query params extraction
                const params: Record<string, string> = { ...match.params };
                for (const [k, v] of urlObj.searchParams) {
                    params[k] = v;
                }

                const decodedReq = methodInfo.Decoder((target: any) => {
                    Object.assign(target, reqJson);
                    if (methodInfo.Binder) {
                        methodInfo.Binder(target, params);
                    }
                });

                const startTime = Date.now();
                const traceId = getOrCreateTraceId(req.headers);
                globalRegistry.recordRequest(match.route.handlerPath);

                const resp = await methodInfo.Handler({ traceId }, decodedReq);

                const duration = Date.now() - startTime;
                globalRegistry.recordLatency(match.route.handlerPath, duration);
                logStructured('INFO', 'HTTP/1.1 REST request processed', {
                    method: match.route.handlerPath,
                    latencyMs: duration,
                    traceId
                });

                if (resp && (typeof resp[Symbol.asyncIterator] === 'function' || typeof (resp as any).next === 'function')) {
                    res.writeHead(200, {
                        'Content-Type': 'text/event-stream',
                        'Cache-Control': 'no-cache',
                        'Connection': 'keep-alive',
                    });
                    for await (const chunk of resp as any) {
                        res.write(`data: ${JSON.stringify(chunk)}\n\n`);
                    }
                    res.write("data: [DONE]\n\n");
                    res.end();
                    return;
                }

                const respStr = JSON.stringify(resp);
                const byteLength = Buffer.byteLength(respStr, 'utf8');
                let respBuffer: Buffer;
                if (byteLength <= 1024 * 1024) {
                    rentedRespBuf = this.responseBufferPool.acquire();
                    rentedRespBuf.write(respStr, 0, byteLength, 'utf8');
                    respBuffer = rentedRespBuf.subarray(0, byteLength);
                } else {
                    respBuffer = Buffer.from(respStr, 'utf8');
                }

                res.writeHead(200, { 'Content-Type': 'application/json' });
                res.end(respBuffer, () => {
                    if (rentedRespBuf) {
                        this.responseBufferPool.release(rentedRespBuf);
                    }
                });
            } catch (err: any) {
                if (rentedReqBuf) {
                    this.requestBufferPool.release(rentedReqBuf);
                }
                if (rentedRespBuf) {
                    this.responseBufferPool.release(rentedRespBuf);
                }
                res.writeHead(500, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({ error: err.message }));
            }
        });
    }

    private handleHTTP2(stream: http2.ServerHttp2Stream, headers: http2.IncomingHttpHeaders) {
        const path = headers[':path'] || '';
        const methodInfo = this.methods.get(path);

        if (!methodInfo) {
            stream.respond({
                ':status': 200,
                'content-type': 'application/grpc',
                'grpc-status': '12',
                'grpc-message': 'unimplemented'
            });
            stream.end();
            return;
        }

        const chunks: any[] = [];
        let totalLength = 0;
        stream.on('data', chunk => {
            chunks.push(chunk);
            totalLength += chunk.length;
        });
        stream.on('end', async () => {
            let rawFrame: Buffer;
            let rentedReqBuf: Buffer | null = null;
            let rentedRespBuf: Buffer | null = null;
            let rentedHeaderBuf: Buffer | null = null;
            try {
                if (totalLength <= 1024 * 1024) {
                    rentedReqBuf = this.requestBufferPool.acquire();
                    let offset = 0;
                    for (const chunk of chunks) {
                        chunk.copy(rentedReqBuf, offset);
                        offset += chunk.length;
                    }
                    rawFrame = rentedReqBuf.subarray(0, totalLength);
                } else {
                    rawFrame = Buffer.concat(chunks);
                }

                // gRPC frame parsing: byte 0 = compressed-flag, bytes 1-4 = message length
                if (rawFrame.length < 5) {
                    throw new Error('grpc frame too small');
                }
                const compressedFlag = rawFrame[0];
                const len = rawFrame.readUInt32BE(1);
                let payload = rawFrame.subarray(5, 5 + len);

                if (compressedFlag === 1) {
                    payload = zlib.gunzipSync(payload);
                }

                // Decode request using FlatBuffers or JSON fallback (for simplicity we check type)
                let decodedReq: any;
                if (payload[0] === 4) {
                    // Looks like FlatBuffers root pointer!
                    decodedReq = methodInfo.Decoder((target: any) => {
                        if (target.constructor && typeof target.constructor.unmarshalFlatBuffers === 'function') {
                            Object.assign(target, target.constructor.unmarshalFlatBuffers(new Uint8Array(payload)));
                        }
                    });
                } else {
                    // Fallback to JSON or protobuf decoding
                    const str = payload.toString('utf8');
                    const json = str ? JSON.parse(str) : {};
                    decodedReq = methodInfo.Decoder((target: any) => {
                        Object.assign(target, json);
                    });
                }

                if (rentedReqBuf) {
                    this.requestBufferPool.release(rentedReqBuf);
                    rentedReqBuf = null;
                }

                const traceId = getOrCreateTraceId(headers);
                globalRegistry.recordRequest(path);
                const startTime = Date.now();

                const resp = await methodInfo.Handler({ traceId }, decodedReq);

                const duration = Date.now() - startTime;
                globalRegistry.recordLatency(path, duration);
                logStructured('INFO', 'gRPC request processed', {
                    method: path,
                    latencyMs: duration,
                    traceId
                });

                // Write response frame (we support FlatBuffers or JSON string fallback)
                let respBytes: Uint8Array;
                if (resp.marshalFlatBuffers && typeof resp.marshalFlatBuffers === 'function') {
                    respBytes = resp.marshalFlatBuffers();
                } else {
                    const respStr = JSON.stringify(resp);
                    const byteLength = Buffer.byteLength(respStr, 'utf8');
                    if (byteLength <= 1024 * 1024) {
                        rentedRespBuf = this.responseBufferPool.acquire();
                        rentedRespBuf.write(respStr, 0, byteLength, 'utf8');
                        respBytes = rentedRespBuf.subarray(0, byteLength);
                    } else {
                        respBytes = Buffer.from(respStr, 'utf8');
                    }
                }

                let finalBytes = respBytes;
                let finalCompressed = 0;
                if (headers['grpc-accept-encoding'] && headers['grpc-accept-encoding'].toString().includes('gzip')) {
                    finalBytes = zlib.gzipSync(respBytes);
                    finalCompressed = 1;
                    if (rentedRespBuf) {
                        this.responseBufferPool.release(rentedRespBuf);
                        rentedRespBuf = null;
                    }
                }

                rentedHeaderBuf = this.headerBufferPool.acquire();
                rentedHeaderBuf[0] = finalCompressed;
                rentedHeaderBuf.writeUInt32BE(finalBytes.length, 1);

                stream.respond({
                    ':status': 200,
                    'content-type': 'application/grpc',
                    'grpc-status': '0'
                });
                
                stream.write(rentedHeaderBuf, () => {
                    if (rentedHeaderBuf) {
                        this.headerBufferPool.release(rentedHeaderBuf);
                        rentedHeaderBuf = null;
                    }
                });

                stream.end(finalBytes, () => {
                    if (rentedRespBuf) {
                        this.responseBufferPool.release(rentedRespBuf);
                    }
                });
            } catch (err: any) {
                if (rentedReqBuf) {
                    this.requestBufferPool.release(rentedReqBuf);
                }
                if (rentedRespBuf) {
                    this.responseBufferPool.release(rentedRespBuf);
                }
                if (rentedHeaderBuf) {
                    this.headerBufferPool.release(rentedHeaderBuf);
                }
                stream.respond({
                    ':status': 200,
                    'content-type': 'application/grpc',
                    'grpc-status': '13',
                    'grpc-message': err.message
                });
                stream.end();
            }
        });
    }
}

function matchREST(method: string, path: string, routes: RESTRoute[]): { route: RESTRoute, params: Record<string, string> } | null {
    const reqParts = splitPath(path);
    method = method.toUpperCase();

    for (const r of routes) {
        const routeParts = splitPath(r.pattern);
        if (r.method !== method || routeParts.length !== reqParts.length) {
            continue;
        }
        let match = true;
        const params: Record<string, string> = {};
        for (let i = 0; i < routeParts.length; i++) {
            const part = routeParts[i];
            if (part.startsWith('{') && part.endsWith('}')) {
                const paramName = part.slice(1, -1);
                params[paramName] = reqParts[i];
            } else if (part !== reqParts[i]) {
                match = false;
                break;
            }
        }
        if (match) {
            return { route: r, params };
        }
    }
    return null;
}

function splitPath(path: string): string[] {
    const trimmed = path.replace(/^\/+|\/+$/g, '');
    return trimmed === '' ? [] : trimmed.split('/');
}

class BufferPool {
    private pool: Buffer[] = [];
    private bufferSize: number;

    constructor(bufferSize: number, initialCount: number = 10) {
        this.bufferSize = bufferSize;
        for (let i = 0; i < initialCount; i++) {
            this.pool.push(Buffer.alloc(bufferSize));
        }
    }

    public acquire(): Buffer {
        return this.pool.pop() || Buffer.alloc(this.bufferSize);
    }

    public release(buf: Buffer) {
        if (buf.length === this.bufferSize) {
            this.pool.push(buf);
        }
    }
}
