import * as net from 'net';

export class AgenticStream {
    private callId = 0;
    private buffer = '';
    private readonly pendingResolvers: Map<number, (res: any) => void> = new Map();

    constructor(private readonly socket: net.Socket) {
        this.socket.on('data', (chunk) => {
            this.buffer += chunk.toString('utf-8');
            let boundary = this.buffer.indexOf('\n');
            while (boundary !== -1) {
                const line = this.buffer.slice(0, boundary).trim();
                this.buffer = this.buffer.slice(boundary + 1);
                if (line) {
                    try {
                        const frame = JSON.parse(line);
                        if (frame.type === 'tool_response') {
                            const resolver = this.pendingResolvers.get(frame.id);
                            if (resolver) {
                                resolver(frame.result || {});
                                this.pendingResolvers.delete(frame.id);
                            }
                        }
                    } catch (e) {
                        // ignore malformed JSON
                    }
                }
                boundary = this.buffer.indexOf('\n');
            }
        });
    }

    public sendToken(token: string): Promise<void> {
        return new Promise((resolve, reject) => {
            const payload = JSON.stringify({ type: 'token', content: token }) + '\n';
            this.socket.write(payload, 'utf-8', (err) => {
                if (err) reject(err);
                else resolve();
            });
        });
    }

    public callTool(toolName: string, args: Record<string, any>): Promise<Record<string, any>> {
        this.callId++;
        const currentId = this.callId;

        return new Promise((resolve, reject) => {
            this.pendingResolvers.set(currentId, resolve);

            const payload = JSON.stringify({
                type: 'tool_call',
                id: currentId,
                name: toolName,
                arguments: args
            }) + '\n';

            this.socket.write(payload, 'utf-8', (err) => {
                if (err) {
                    this.pendingResolvers.delete(currentId);
                    reject(err);
                }
            });
        });
    }
}
