package io.helixrpc.runtime;

import java.util.Arrays;
import java.util.List;
import java.nio.charset.StandardCharsets;

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
        // Test gRPC Preface
        byte[] grpcPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n".getBytes(StandardCharsets.US_ASCII);
        if (Sniffer.sniffProtocol(grpcPreface) != Protocol.GRPC) {
            throw new RuntimeException("failed to sniff gRPC");
        }

        // Test Thrift Binary
        byte[] thriftBin = { (byte)0x80, 0x01, 0x00, 0x01 };
        if (Sniffer.sniffProtocol(thriftBin) != Protocol.THRIFT_BINARY) {
            throw new RuntimeException("failed to sniff Thrift Binary");
        }

        // Test Thrift Compact
        byte[] thriftCompact = { (byte)0x82, 0x15, 0x01 };
        if (Sniffer.sniffProtocol(thriftCompact) != Protocol.THRIFT_COMPACT) {
            throw new RuntimeException("failed to sniff Thrift Compact");
        }

        // Test REST GET
        byte[] restGet = "GET /index.html".getBytes(StandardCharsets.US_ASCII);
        if (Sniffer.sniffProtocol(restGet) != Protocol.REST) {
            throw new RuntimeException("failed to sniff REST");
        }

        System.out.println("✓ testSniffer passed!");
    }

    public static void main(String[] args) {
        testConsistentHashBalancer();
        testSniffer();
        System.out.println("All Java Runtime tests passed successfully!");
    }
}
