import { CacheInterceptor } from './cache';
import * as Memcached from 'memcached';

jest.mock('memcached');

describe('CacheInterceptor', () => {
    let cache: CacheInterceptor;
    let mockMemcached: jest.Mocked<Memcached>;

    beforeEach(() => {
        (Memcached as unknown as jest.Mock).mockClear();
        cache = new CacheInterceptor('localhost:11211', 60);
        mockMemcached = (Memcached as unknown as jest.Mock).mock.instances[0] as jest.Mocked<Memcached>;
    });

    it('should generate a valid cache key', () => {
        const key = cache.generateCacheKey('myMethod', Buffer.from('payload'));
        expect(key).toBeDefined();
        expect(typeof key).toBe('string');
        expect(key.length).toBe(64); // sha256 hex length
    });

    it('should set a cache value', () => {
        cache.set('myKey', Buffer.from('myPayload'));
        expect(mockMemcached.set).toHaveBeenCalledWith('myKey', Buffer.from('myPayload'), 60, expect.any(Function));
    });

    it('should get a cache value', async () => {
        const mockData = Buffer.from('mockData');
        mockMemcached.get.mockImplementation((_key: string, callback: (err: Error | undefined, data: unknown) => void) => {
            callback(undefined, mockData);
        });

        const [data, hit] = await cache.get('myKey');
        expect(hit).toBe(true);
        expect(data).toEqual(mockData);
        expect(mockMemcached.get).toHaveBeenCalledWith('myKey', expect.any(Function));
    });

    it('should handle get miss', async () => {
        mockMemcached.get.mockImplementation((_key: string, callback: (err: Error | undefined, data: unknown) => void) => {
            callback(undefined, undefined);
        });

        const [data, hit] = await cache.get('myKey');
        expect(hit).toBe(false);
        expect(data).toBeNull();
    });
});
