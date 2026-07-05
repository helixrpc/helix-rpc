package io.helixrpc.runtime;

import java.util.Arrays;
import java.util.List;
import java.nio.charset.StandardCharsets;
import java.time.Duration;

public class RuntimeTest {
    public static void testConsistentHashBalancer() {
        ConsistentHashBalancer balancer = new ConsistentHashBalancer(50);
        List<String> targets = Arrays.asList("127.0.0.1:8081", "127.0.0.1:8082", "127.0.0.1:8083");

        String key1 = "system-prompt-llm-1";
        String choice1 = balancer.nextWithKey(targets, key1);

        for (int i = 0; i < 20; i++) {
            String choice = balancer.nextWithKey(targets, key1);
            if (!choice.equals(choice1)) {
                throw new RuntimeException("consistent hash violated");
            }
        }

        String key2 = "different-prompt";
        String choice2 = balancer.nextWithKey(targets, key2);

        for (int i = 0; i < 20; i++) {
            String choice = balancer.nextWithKey(targets, key2);
            if (!choice.equals(choice2)) {
                throw new RuntimeException("consistent hash violated for key2");
            }
        }

        System.out.println("✓ testConsistentHashBalancer passed!");
    }

    public static void testSniffer() {
        byte[] grpcPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n".getBytes(StandardCharsets.US_ASCII);
        if (Sniffer.sniffProtocol(grpcPreface) != Protocol.GRPC) {
            throw new RuntimeException("failed to sniff gRPC");
        }

        byte[] thriftBin = { (byte)0x80, 0x01, 0x00, 0x01 };
        if (Sniffer.sniffProtocol(thriftBin) != Protocol.THRIFT_BINARY) {
            throw new RuntimeException("failed to sniff Thrift Binary");
        }

        byte[] thriftCompact = { (byte)0x82, 0x15, 0x01 };
        if (Sniffer.sniffProtocol(thriftCompact) != Protocol.THRIFT_COMPACT) {
            throw new RuntimeException("failed to sniff Thrift Compact");
        }

        byte[] restGet = "GET /index.html".getBytes(StandardCharsets.US_ASCII);
        if (Sniffer.sniffProtocol(restGet) != Protocol.REST) {
            throw new RuntimeException("failed to sniff REST");
        }

        System.out.println("✓ testSniffer passed!");
    }

    public static void testDeadline() {
        if (Deadline.parseGRPCTimeout("100m").toMillis() != 100) {
            throw new RuntimeException("failed deadline parse m");
        }
        if (Deadline.parseGRPCTimeout("500u").toNanos() != 500000) {
            throw new RuntimeException("failed deadline parse u");
        }
        if (Deadline.parseGRPCTimeout("2S").getSeconds() != 2) {
            throw new RuntimeException("failed deadline parse S");
        }
        System.out.println("✓ testDeadline passed!");
    }

    public static void testCompression() throws Exception {
        Compression.GzipCompressor compressor = new Compression.GzipCompressor();
        byte[] original = "hello gzip".getBytes(StandardCharsets.UTF_8);
        byte[] compressed = compressor.compress(original);
        byte[] decompressed = compressor.decompress(compressed);
        if (!Arrays.equals(decompressed, original)) {
            throw new RuntimeException("failed compression roundtrip");
        }
        System.out.println("✓ testCompression passed!");
    }

    public static void testHealth() {
        Health.HealthChecker checker = new Health.HealthChecker();
        if (checker.check("my-service") != Health.HealthStatus.UNKNOWN) {
            throw new RuntimeException("health check default failed");
        }
        checker.setServingStatus("my-service", Health.HealthStatus.SERVING);
        if (checker.check("my-service") != Health.HealthStatus.SERVING) {
            throw new RuntimeException("health check serving failed");
        }
        checker.setServingStatus("my-service", Health.HealthStatus.NOT_SERVING);
        if (checker.check("my-service") != Health.HealthStatus.NOT_SERVING) {
            throw new RuntimeException("health check not serving failed");
        }
        System.out.println("✓ testHealth passed!");
    }

    public static void testQuicTransport() throws Exception {
        QuicTransport.QuicListener listener = new QuicTransport.QuicListener(0);
        int port = listener.getPort();
        if (port <= 0) {
            throw new RuntimeException("failed to bind Java UDP socket");
        }
        listener.close();
        System.out.println("✓ testQuicTransport passed!");
    }

    public static void testGateway() {
        Gateway.MultiTenantRateLimiter limiter = new Gateway.MultiTenantRateLimiter(2.0, 10.0);

        // Default config allows 2 and rejects 3rd
        if (!limiter.allow("tenant-1", 1.0)) {
            throw new RuntimeException("failed gateway allow 1");
        }
        if (!limiter.allow("tenant-1", 1.0)) {
            throw new RuntimeException("failed gateway allow 2");
        }
        if (limiter.allow("tenant-1", 1.0)) {
            throw new RuntimeException("failed gateway block 3");
        }

        // Custom config overrides
        limiter.setTenantLimit("tenant-2", 5.0, 50.0);
        if (!limiter.allow("tenant-2", 5.0)) {
            throw new RuntimeException("failed gateway custom allow");
        }
        if (limiter.allow("tenant-2", 1.0)) {
            throw new RuntimeException("failed gateway custom block");
        }

        System.out.println("✓ testGateway passed!");
    }

    public static void testMultiplexer() throws Exception {
        try (MultiplexedServer server = new MultiplexedServer(0)) {
            int port = server.getPort();
            if (port <= 0) {
                throw new RuntimeException("failed to bind Java multiplexer socket");
            }
            server.start(sock -> {}, sock -> {});
        }
        System.out.println("✓ testMultiplexer passed!");
    }

    public static void main(String[] args) throws Exception {
        testConsistentHashBalancer();
        testSniffer();
        testDeadline();
        testCompression();
        testHealth();
        testQuicTransport();
        testGateway();
        testMultiplexer();
        System.out.println("All Java Parity tests passed successfully!");
    }
}
