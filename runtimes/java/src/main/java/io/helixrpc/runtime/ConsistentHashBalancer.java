package io.helixrpc.runtime;

import java.util.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;

public class ConsistentHashBalancer {
    private final int replicas;
    private final TreeMap<Long, String> ring = new TreeMap<>();
    private final Set<String> registered = new HashSet<>();

    public ConsistentHashBalancer(int replicas) {
        this.replicas = replicas <= 0 ? 50 : replicas;
    }

    private long hash(String key) {
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-256");
            byte[] bytes = md.digest(key.getBytes(StandardCharsets.UTF_8));
            // Combine first 8 bytes into a long
            long h = 0;
            for (int i = 0; i < 8; i++) {
                h = (h << 8) | (bytes[i] & 0xFF);
            }
            return h;
        } catch (NoSuchAlgorithmException e) {
            throw new RuntimeException(e);
        }
    }

    public synchronized void add(String node) {
        if (registered.contains(node)) return;
        registered.add(node);

        for (int i = 0; i < replicas; i++) {
            long h = hash(node + "#" + i);
            ring.put(h, node);
        }
    }

    public synchronized void remove(String node) {
        if (!registered.contains(node)) return;
        registered.remove(node);

        List<Long> toRemove = new ArrayList<>();
        for (Map.Entry<Long, String> entry : ring.entrySet()) {
            if (entry.getValue().equals(node)) {
                toRemove.add(entry.getKey());
            }
        }
        for (Long k : toRemove) {
            ring.remove(k);
        }
    }

    public synchronized String nextWithKey(List<String> targets, String key) {
        if (targets == null || targets.isEmpty()) {
            throw new RuntimeException("no targets available for load balancing");
        }

        // Lazily register targets
        for (String target : targets) {
            if (!registered.contains(target)) {
                add(target);
            }
        }

        if (ring.isEmpty()) {
            return targets.get(0);
        }

        long h = hash(key);
        // Find the first key on the ring greater than or equal to h
        SortedMap<Long, String> tailMap = ring.tailMap(h);
        long ringKey = tailMap.isEmpty() ? ring.firstKey() : tailMap.firstKey();

        Set<String> targetSet = new HashSet<>(targets);
        long startKey = ringKey;
        
        do {
            String node = ring.get(ringKey);
            if (targetSet.contains(node)) {
                return node;
            }
            // Move to next key
            Long nextKey = ring.higherKey(ringKey);
            ringKey = nextKey == null ? ring.firstKey() : nextKey;
        } while (ringKey != startKey);

        return targets.get(0);
    }
}
