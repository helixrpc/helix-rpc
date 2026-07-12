#include "test_helix_generated.h"
#include <iostream>
#include <cassert>
#include <algorithm>

void TestCPPPerformanceOptimizations() {
    // Construct Protobuf binary:
    // Field 1 (user_id = 42): tag = 0x08, value = 42
    // Field 2 (username = "zero_copy_hero"): tag = 0x12, length = 14, bytes
    // Field 3 (email = "hero@helix.rpc"): tag = 0x1a, length = 14, bytes
    std::vector<uint8_t> proto_bytes = {
        0x08, 42,
        0x12, 14, 'z', 'e', 'r', 'o', '_', 'c', 'o', 'p', 'y', '_', 'h', 'e', 'r', 'o',
        0x1a, 14, 'h', 'e', 'r', 'o', '@', 'h', 'e', 'l', 'i', 'x', '.', 'r', 'p', 'c'
    };

    // 1. Verify Lazy Smart Fields
    helix::LazyUserProfile lazy(proto_bytes.data(), proto_bytes.size());
    assert(lazy.get_user_id() == 42);
    assert(lazy.get_username() == "zero_copy_hero");
    assert(lazy.get_email() == "hero@helix.rpc");

    // 2. Verify Zero-Alloc Transpilation
    auto thrift_bytes = helix::UserProfile::TranspileProtobufToThriftCompact(proto_bytes.data(), proto_bytes.size());
    assert(!thrift_bytes.empty());
    assert(thrift_bytes.back() == 0x00); // Ends with Thrift STOP byte

    // Check that strings are embedded in the output verbatim
    std::string thrift_str(reinterpret_cast<const char*>(thrift_bytes.data()), thrift_bytes.size());
    assert(thrift_str.find("zero_copy_hero") != std::string::npos);
    assert(thrift_str.find("hero@helix.rpc") != std::string::npos);

    std::cout << "✓ TestCPPPerformanceOptimizations passed!" << std::endl;
}

int main() {
    TestCPPPerformanceOptimizations();
    std::cout << "All C++ Optimization tests passed successfully!" << std::endl;
    return 0;
}
