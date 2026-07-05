package io.helixrpc.runtime;

import java.nio.charset.StandardCharsets;

public class Sniffer {
    public static Protocol sniffProtocol(byte[] peekBytes) {
        if (peekBytes == null || peekBytes.length == 0) {
            return Protocol.UNKNOWN;
        }

        // 1. Sniff gRPC (HTTP/2 Preface or HTTP headers)
        String peekStr = new String(peekBytes, 0, peekBytes.length, StandardCharsets.US_ASCII);
        if (peekStr.startsWith("PRI * HTTP/2.0")) {
            return Protocol.GRPC;
        }

        // 2. Sniff Thrift
        if (peekBytes.length >= 2) {
            // Binary: 0x8001
            if ((peekBytes[0] & 0xFF) == 0x80 && (peekBytes[1] & 0xFF) == 0x01) {
                return Protocol.THRIFT_BINARY;
            }
            // Compact: 0x82
            if ((peekBytes[0] & 0xFF) == 0x82) {
                return Protocol.THRIFT_COMPACT;
            }
        }

        // 3. Sniff REST / HTTP/1.1
        if (peekBytes.length >= 4) {
            if (peekStr.startsWith("GET ") ||
                peekStr.startsWith("POST") ||
                peekStr.startsWith("PUT ") ||
                peekStr.startsWith("DELE")) {
                return Protocol.REST;
            }
        }

        return Protocol.UNKNOWN;
    }
}
