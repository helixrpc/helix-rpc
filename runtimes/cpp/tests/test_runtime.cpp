#include "helix/balancer.h"
#include "helix/sniffer.h"
#include "helix/deadline.h"
#include "helix/compression.h"
#include "helix/health.h"
#include "helix/quic_transport.h"
#include "helix/gateway.h"
#include "helix/multiplexer.h"
#include "helix/tensor.h"
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
    std::vector<uint8_t> grpc_preface = {'P', 'R', 'I', ' ', '*', ' ', 'H', 'T', 'T', 'P', '/', '2', '.', '0', '\r', '\n'};
    assert(helix::SniffProtocol(grpc_preface) == helix::Protocol::GRPC);

    std::vector<uint8_t> thrift_bin = {0x80, 0x01, 0x00, 0x01};
    assert(helix::SniffProtocol(thrift_bin) == helix::Protocol::THRIFT_BINARY);

    std::vector<uint8_t> thrift_compact = {0x82, 0x15, 0x01};
    assert(helix::SniffProtocol(thrift_compact) == helix::Protocol::THRIFT_COMPACT);

    std::vector<uint8_t> rest_get = {'G', 'E', 'T', ' ', '/', 'i', 'n', 'd', 'e', 'x', '.', 'h', 't', 'm', 'l'};
    assert(helix::SniffProtocol(rest_get) == helix::Protocol::REST);

    std::cout << "✓ TestSniffer passed!" << std::endl;
}

void TestDeadline() {
    assert(helix::ParseGRPCTimeout("100m").count() == 100000);
    assert(helix::ParseGRPCTimeout("500u").count() == 500);
    assert(helix::ParseGRPCTimeout("2S").count() == 2000000);
    std::cout << "✓ TestDeadline passed!" << std::endl;
}

void TestCompression() {
    helix::GzipCompressor compressor;
    std::vector<uint8_t> original = {1, 2, 3, 4, 5};
    auto compressed = compressor.Compress(original);
    assert(compressed.size() == 9);
    assert(compressed[0] == 0x1f && compressed[1] == 0x8b);
    auto decompressed = compressor.Decompress(compressed);
    assert(decompressed == original);
    std::cout << "✓ TestCompression passed!" << std::endl;
}

void TestHealth() {
    helix::HealthChecker checker;
    assert(checker.Check("my-service") == helix::HealthStatus::UNKNOWN);
    checker.SetServingStatus("my-service", helix::HealthStatus::SERVING);
    assert(checker.Check("my-service") == helix::HealthStatus::SERVING);
    checker.SetServingStatus("my-service", helix::HealthStatus::NOT_SERVING);
    assert(checker.Check("my-service") == helix::HealthStatus::NOT_SERVING);
    std::cout << "✓ TestHealth passed!" << std::endl;
}

void TestQuicTransport() {
    helix::QuicListener listener(0);
    int port = listener.GetPort();
    assert(port > 0);

    // Verify socket exists
    std::cout << "✓ TestQuicTransport passed (UDP bound on port " << port << ")!" << std::endl;
}

void TestGateway() {
    helix::MultiTenantRateLimiter limiter(2.0, 10.0);

    // Default configuration allows 2 and rejects 3rd
    assert(limiter.Allow("tenant-1", 1.0) == true);
    assert(limiter.Allow("tenant-1", 1.0) == true);
    assert(limiter.Allow("tenant-1", 1.0) == false);

    // Custom limit configuration overrides
    limiter.SetTenantLimit("tenant-2", 5.0, 50.0);
    assert(limiter.Allow("tenant-2", 5.0) == true);
    assert(limiter.Allow("tenant-2", 1.0) == false);

    std::cout << "✓ TestGateway passed!" << std::endl;
}

void TestMultiplexer() {
    helix::MultiplexedServer server(0);
    int port = server.GetPort();
    assert(port > 0);

    server.Start([](int fd) {}, [](int fd) {});
    server.Stop();

    std::cout << "✓ TestMultiplexer passed (bound on port " << port << ")!" << std::endl;
}

void TestTensor() {
    helix::Tensor t;
    t.dtype = "float32";
    t.shape = {1, 4};
    
    // Allocate space for 4 floats
    t.data.resize(4 * sizeof(float));
    float* raw = reinterpret_cast<float*>(t.data.data());
    raw[0] = 1.0f;
    raw[1] = 2.0f;
    raw[2] = 3.0f;
    raw[3] = 4.0f;

    // Verify zero-copy retrieval
    const float* data = helix::GetTensorData<float>(t);
    assert(data != nullptr);
    assert(data[0] == 1.0f);
    assert(data[1] == 2.0f);
    assert(data[2] == 3.0f);
    assert(data[3] == 4.0f);

    std::cout << "✓ TestTensor passed!" << std::endl;
}

int main() {
#ifdef _WIN32
    WSADATA wsaData;
    WSAStartup(MAKEWORD(2, 2), &wsaData);
#endif
    TestConsistentHashBalancer();
    TestSniffer();
    TestDeadline();
    TestCompression();
    TestHealth();
    TestQuicTransport();
    TestGateway();
    TestMultiplexer();
    TestTensor();
    std::cout << "All C++ Parity tests passed successfully!" << std::endl;
    return 0;
}
