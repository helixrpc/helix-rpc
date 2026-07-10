package io.helixrpc.runtime;

import net.spy.memcached.MemcachedClient;
import org.apache.commons.codec.digest.DigestUtils;
import java.net.InetSocketAddress;
import java.io.IOException;

public class MemcachedCache {
    private final MemcachedClient client;
    private final int ttlSeconds;

    public MemcachedCache(String host, int port, int ttlSeconds) throws IOException {
        this.client = new MemcachedClient(new InetSocketAddress(host, port));
        this.ttlSeconds = ttlSeconds;
    }

    public String generateCacheKey(String method, byte[] payload) {
        byte[] methodBytes = method.getBytes();
        byte[] combined = new byte[methodBytes.length + payload.length];
        System.arraycopy(methodBytes, 0, combined, 0, methodBytes.length);
        System.arraycopy(payload, 0, combined, methodBytes.length, payload.length);
        return DigestUtils.sha256Hex(combined);
    }

    public byte[] get(String key) {
        Object val = client.get(key);
        if (val instanceof byte[]) {
            return (byte[]) val;
        }
        return null;
    }

    public void set(String key, byte[] payload) {
        client.set(key, ttlSeconds, payload);
    }
}
