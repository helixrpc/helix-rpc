import * as http from 'http';

function makeRequest(options: http.RequestOptions, body?: string): Promise<string> {
    return new Promise((resolve, reject) => {
        const req = http.request(options, (res) => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => resolve(data));
        });
        req.on('error', reject);
        if (body) {
            req.write(body);
        }
        req.end();
    });
}

async function runClient() {
    console.log('📡 Sending [SET] request to store key/value via HTTP/JSON REST...');
    const setResp = await makeRequest({
        hostname: '127.0.0.1',
        port: 9090,
        path: '/v1/kv',
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        }
    }, JSON.stringify({ key: 'hello', value: 'Unified RPC & REST works!' }));

    console.log('📥 SET Response from Server:', setResp);

    console.log('\n📡 Sending [GET] request to fetch key/value via HTTP/JSON REST...');
    const getResp = await makeRequest({
        hostname: '127.0.0.1',
        port: 9090,
        path: '/v1/kv/hello',
        method: 'GET'
    });

    console.log('📥 GET Response from Server:', getResp);
}

runClient().catch(console.error);
