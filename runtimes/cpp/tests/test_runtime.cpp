#include "helix/balancer.h"
#include "helix/sniffer.h"
#include <iostream>
#include <cassert>

void TestConsistentHashBalancer() {
    helix::ConsistentHashBalancer balancer(50);
    std::vector<std::string> targets = {"127.0.0.1:8081", "127.0.0.1:8082", "127.0.0.1:8083"};

    std::string key1 = "system-prompt-llm-1";
    std::string choice1 = balancer.NextWithKey(targets, key1);

    for (int i = 0; i < 20; ++i) {
        std::string choice = balancer.NextWithKey(targets, key1);
        assert(choice == choice1);
    }

    std::string key2 = "different-prompt";
    std::string choice2 = balancer.NextWithKey(targets, key2);

    for (int i = 0; i < 20; ++i) {
        std::string choice = balancer.NextWithKey(targets, key2);
        assert(choice == choice2);
    }

    std::cout << "✓ TestConsistentHashBalancer passed!" << std::endl;
}

void TestSniffer() {
    // Test gRPC Preface
    std::vector<uint8_t> grpc_preface = {'P', 'R', 'I', ' ', '*', ' ', 'H', 'T', 'T', 'P', '/', '2', '.', '0', '\r', '\n'};
    assert(helix::SniffProtocol(grpc_preface) == helix::Protocol::GRPC);

    // Test Thrift Binary
    std::vector<uint8_t> thrift_bin = {0x80, 0x01, 0x00, 0x01};
    assert(helix::SniffProtocol(thrift_bin) == helix::Protocol::THRIFT_BINARY);

    // Test Thrift Compact
    std::vector<uint8_t> thrift_compact = {0x82, 0x15, 0x01};
    assert(helix::SniffProtocol(thrift_compact) == helix::Protocol::THRIFT_COMPACT);

    // Test REST GET
    std::vector<uint8_t> rest_get = {'G', 'E', 'T', ' ', '/', 'i', 'n', 'd', 'e', 'x', '.', 'h', 't', 'm', 'l'};
    assert(helix::SniffProtocol(rest_get) == helix::Protocol::REST);

    std::cout << "✓ TestSniffer passed!" << std::endl;
}

int main() {
    TestConsistentHashBalancer();
    TestSniffer();
    std::cout << "All C++ Runtime tests passed successfully!" << std::endl;
    return 0;
}
