#pragma once

#include <vector>
#include <string>
#include <cstdint>

namespace helix {

enum class Protocol {
    UNKNOWN,
    GRPC,
    THRIFT_BINARY,
    THRIFT_COMPACT,
    REST
};

inline Protocol SniffProtocol(const std::vector<uint8_t>& peek_bytes) {
    if (peek_bytes.empty()) {
        return Protocol::UNKNOWN;
    }

    // 1. Sniff gRPC (HTTP/2 Preface or HTTP headers)
    std::string_view peek_str(reinterpret_cast<const char*>(peek_bytes.data()), peek_bytes.size());
    if (peek_str.rfind("PRI * HTTP/2.0", 0) == 0) {
        return Protocol::GRPC;
    }

    // 2. Sniff Thrift
    if (peek_bytes.size() >= 2) {
        // Binary: 0x8001
        if (peek_bytes[0] == 0x80 && peek_bytes[1] == 0x01) {
            return Protocol::THRIFT_BINARY;
        }
        // Compact: 0x82
        if (peek_bytes[0] == 0x82) {
            return Protocol::THRIFT_COMPACT;
        }
    }

    // 3. Sniff REST / HTTP/1.1
    if (peek_bytes.size() >= 4) {
        if (peek_str.rfind("GET ", 0) == 0 ||
            peek_str.rfind("POST", 0) == 0 ||
            peek_str.rfind("PUT ", 0) == 0 ||
            peek_str.rfind("DELE", 0) == 0) {
            return Protocol::REST;
        }
    }

    return Protocol::UNKNOWN;
}

} // namespace helix
