package io.helixrpc.runtime;

import io.helixrpc.generated.UserProfile;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;

public class OptimizationTest {
    public static void testJavaPerformanceOptimizations() throws Exception {
        // Construct Protobuf binary:
        // Field 1 (user_id = 42): tag = 0x08, value = 42
        // Field 2 (username = "zero_copy_hero"): tag = 0x12, length = 14, bytes
        // Field 3 (email = "hero@helix.rpc"): tag = 0x1a, length = 14, bytes
        byte[] usernameBytes = "zero_copy_hero".getBytes(StandardCharsets.UTF_8);
        byte[] emailBytes = "hero@helix.rpc".getBytes(StandardCharsets.UTF_8);

        ByteBuffer protoBuf = ByteBuffer.allocate(100);
        protoBuf.put((byte)0x08);
        protoBuf.put((byte)42);
        protoBuf.put((byte)0x12);
        protoBuf.put((byte)usernameBytes.length);
        protoBuf.put(usernameBytes);
        protoBuf.put((byte)0x1a);
        protoBuf.put((byte)emailBytes.length);
        protoBuf.put(emailBytes);
        protoBuf.flip();

        // 1. Verify Lazy Smart Fields
        UserProfile.Lazy lazy = new UserProfile.Lazy(protoBuf.duplicate());
        if (lazy.getUserId() != 42) {
            throw new RuntimeException("failed lazy getUserId");
        }
        if (!"zero_copy_hero".equals(lazy.getUsername())) {
            throw new RuntimeException("failed lazy getUsername");
        }
        if (!"hero@helix.rpc".equals(lazy.getEmail())) {
            throw new RuntimeException("failed lazy getEmail");
        }

        // 2. Verify Zero-Alloc Transpilation
        ByteBuffer output = ByteBuffer.allocate(200);
        ByteBuffer thriftBuf = UserProfile.transpileProtobufToThriftCompact(protoBuf.duplicate(), output);
        
        byte[] resultBytes = new byte[thriftBuf.remaining()];
        thriftBuf.get(resultBytes);

        if (resultBytes.length == 0 || resultBytes[resultBytes.length - 1] != 0x00) {
            throw new RuntimeException("missing Thrift STOP byte");
        }

        String resultStr = new String(resultBytes, StandardCharsets.UTF_8);
        if (!resultStr.contains("zero_copy_hero") || !resultStr.contains("hero@helix.rpc")) {
            throw new RuntimeException("missing expected strings in thrift output");
        }

        System.out.println("✓ testJavaPerformanceOptimizations passed!");
    }

    public static void main(String[] args) throws Exception {
        testJavaPerformanceOptimizations();
        System.out.println("All Java Optimization tests passed successfully!");
    }
}
