import * as dgram from 'dgram';
import { EventEmitter } from 'events';

export class QuicVirtualStream extends EventEmitter {
    constructor(
        public readonly streamId: number,
        public readonly remoteAddress: string,
        public readonly remotePort: number,
        private readonly sendFn: (data: Uint8Array) => void
    ) {
        super();
    }

    public write(data: Uint8Array): void {
        this.sendFn(data);
    }
}

export class QuicListener extends EventEmitter {
    private readonly socket: dgram.Socket;
    private readonly streams: Map<string, QuicVirtualStream> = new Map();

    constructor(port: number = 0, address: string = '127.0.0.1') {
        super();
        this.socket = dgram.createSocket('udp4');
        this.socket.on('message', (msg, rinfo) => this.handleMessage(msg, rinfo));
        this.socket.bind(port, address);
    }

    public getAddress(): { address: string; port: number } {
        return this.socket.address();
    }

    public close(): void {
        this.socket.close();
    }

    private handleMessage(msg: Buffer, rinfo: dgram.RemoteInfo): void {
        if (msg.length < 4) return;
        const streamId = msg.readUInt32BE(0);
        const key = `${rinfo.address}:${rinfo.port}:${streamId}`;

        let stream = this.streams.get(key);
        if (!stream) {
            stream = new QuicVirtualStream(streamId, rinfo.address, rinfo.port, (data) => {
                const packet = Buffer.alloc(4 + data.length);
                packet.writeUInt32BE(streamId, 0);
                Buffer.from(data).copy(packet, 4);
                this.socket.send(packet, rinfo.port, rinfo.address);
            });
            this.streams.set(key, stream);
            this.emit('connection', stream);
        }

        const payload = msg.subarray(4);
        stream.emit('data', new Uint8Array(payload));
    }
}
