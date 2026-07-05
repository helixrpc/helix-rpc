package io.helixrpc.runtime;

import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

public class Gateway {
    private static class TenantLimiter {
        private final double capacity;
        private final double rate;
        private double tokens;
        private long lastRefill;

        public TenantLimiter(double capacity, double rate) {
            this.capacity = capacity;
            this.rate = rate;
            this.tokens = capacity;
            this.lastRefill = System.nanoTime();
        }

        public synchronized boolean consume(double count) {
            long now = System.nanoTime();
            double elapsed = (now - lastRefill) / 1e9;
            tokens = Math.min(capacity, tokens + elapsed * rate);
            lastRefill = now;

            if (tokens >= count) {
                tokens -= count;
                return true;
            }
            return false;
        }
    }

    public static class MultiTenantRateLimiter {
        private final Map<String, TenantLimiter> limiters = new ConcurrentHashMap<>();
        private final double defaultCapacity;
        private final double defaultRate;

        public MultiTenantRateLimiter(double defaultCapacity, double defaultRate) {
            this.defaultCapacity = defaultCapacity;
            this.defaultRate = defaultRate;
        }

        public void setTenantLimit(String tenantId, double capacity, double rate) {
            limiters.put(tenantId, new TenantLimiter(capacity, rate));
        }

        public boolean allow(String tenantId, double count) {
            TenantLimiter limiter = limiters.computeIfAbsent(tenantId, 
                id -> new TenantLimiter(defaultCapacity, defaultRate));
            return limiter.consume(count);
        }
    }
}
