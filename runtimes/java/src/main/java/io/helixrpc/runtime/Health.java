package io.helixrpc.runtime;

import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

public class Health {
    public enum HealthStatus {
        UNKNOWN(0),
        SERVING(1),
        NOT_SERVING(2);

        private final int value;
        HealthStatus(int value) { this.value = value; }
        public int getValue() { return value; }
    }

    public static class HealthChecker {
        private final Map<String, HealthStatus> statuses = new ConcurrentHashMap<>();

        public void setServingStatus(String service, HealthStatus status) {
            statuses.put(service, status);
        }

        public HealthStatus check(String service) {
            return statuses.getOrDefault(service, HealthStatus.UNKNOWN);
        }
    }
}
