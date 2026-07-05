import { HelixServer } from 'helix-rt-node';
import { GetRequest, GetResponse, SetRequest, SetResponse } from './generated.js';

const db = new Map<string, string>();

// Listen on localhost port 9090
const server = new HelixServer('127.0.0.1:9090');

// Register GET method
server.registerMethod('/keyval.KVService/Get', {
    Decoder: (dec) => {
        const req = new GetRequest();
        dec(req);
        return req;
    },
    Binder: (req, params) => {
        req.key = params.key;
    },
    Handler: async (ctx, req) => {
        console.log(`[GET] Key: "${req.key}" requested.`);
        const val = db.get(req.key);
        return new GetResponse({
            value: val || "",
            found: val !== undefined
        });
    }
});

// Register SET method
server.registerMethod('/keyval.KVService/Set', {
    Decoder: (dec) => {
        const req = new SetRequest();
        dec(req);
        return req;
    },
    Handler: async (ctx, req) => {
        console.log(`[SET] Key: "${req.key}" => Value: "${req.value}" saved.`);
        db.set(req.key, req.value);
        return new SetResponse({
            success: true
        });
    }
});

// Register REST Routes (Transcoded to KVService endpoints)
server.registerRESTRoute('GET', '/v1/kv/:key', '/keyval.KVService/Get');
server.registerRESTRoute('POST', '/v1/kv', '/keyval.KVService/Set');

console.log('🧬 Starting Helix Unified Multi-Protocol Server on 127.0.0.1:9090...');
await server.start();
console.log('🚀 Server is running and listening on 127.0.0.1:9090!');

// Start a secondary HTTP server on port 9091 to serve the dashboard UI static files
import * as http from 'http';
import * as fs from 'fs';
import * as path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const staticServer = http.createServer((req, res) => {
    let filePath = path.join(__dirname, '../public', req.url === '/' ? 'index.html' : req.url || 'index.html');
    fs.readFile(filePath, (err, content) => {
        if (err) {
            res.writeHead(404, { 'Content-Type': 'text/plain' });
            res.end('Not Found');
        } else {
            res.writeHead(200, { 'Content-Type': 'text/html' });
            res.end(content);
        }
    });
});

staticServer.listen(9091, '127.0.0.1', () => {
    console.log('🖥️  Dashboard UI available at http://127.0.0.1:9091');
});

