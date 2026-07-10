import * as Memcached from 'memcached';
import { createHash } from 'crypto';

export class CacheInterceptor {
    private client: Memcached;
    private ttl: number;

    constructor(memcachedServers: string | string[], ttlSeconds: number) {
        this.client = new Memcached(memcachedServers);
        this.ttl = ttlSeconds;
    }

    public generateCacheKey(method: string, payload: Buffer): string {
        const h = createHash('sha256');
        h.update(method);
        h.update(payload);
        return h.digest('hex');
    }

    public get(key: string): Promise<[Buffer | null, boolean]> {
        return new Promise((resolve) => {
            this.client.get(key, (err: Error | undefined, data: unknown) => {
                if (err || !data) {
                    resolve([null, false]);
                } else {
                    resolve([Buffer.isBuffer(data) ? data : Buffer.from(data as string), true]);
                }
            });
        });
    }

    public set(key: string, payload: Buffer): void {
        this.client.set(key, payload, this.ttl, (err: Error | undefined) => {
            if (err) {
                console.error(`Failed to set cache for key ${key}: ${err}`);
            }
        });
    }
}
