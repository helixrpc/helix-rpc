#pragma once

#include <libmemcached/memcached.h>
#include <openssl/sha.h>
#include <string>
#include <vector>
#include <optional>
#include <sstream>
#include <iomanip>

namespace helix {
namespace runtime {

class MemcachedCache {
private:
    memcached_st* memc_;
    int ttl_seconds_;

    std::string sha256_hex(const std::string& input) {
        unsigned char hash[SHA256_DIGEST_LENGTH];
        SHA256_CTX sha256;
        SHA256_Init(&sha256);
        SHA256_Update(&sha256, input.c_str(), input.size());
        SHA256_Final(hash, &sha256);

        std::stringstream ss;
        for(int i = 0; i < SHA256_DIGEST_LENGTH; i++) {
            ss << std::hex << std::setw(2) << std::setfill('0') << (int)hash[i];
        }
        return ss.str();
    }

public:
    MemcachedCache(const std::string& host, int port, int ttl_seconds) : ttl_seconds_(ttl_seconds) {
        memc_ = memcached_create(NULL);
        memcached_server_add(memc_, host.c_str(), port);
    }

    ~MemcachedCache() {
        if (memc_) {
            memcached_free(memc_);
        }
    }

    std::string generate_cache_key(const std::string& method, const std::vector<uint8_t>& payload) {
        std::string combined = method;
        combined.append(reinterpret_cast<const char*>(payload.data()), payload.size());
        return sha256_hex(combined);
    }

    std::optional<std::vector<uint8_t>> get(const std::string& key) {
        size_t value_length;
        uint32_t flags;
        memcached_return_t rc;
        
        char* value = memcached_get(memc_, key.c_str(), key.length(), &value_length, &flags, &rc);
        
        if (rc == MEMCACHED_SUCCESS && value != nullptr) {
            std::vector<uint8_t> result(value, value + value_length);
            free(value);
            return result;
        }
        
        return std::nullopt;
    }

    void set(const std::string& key, const std::vector<uint8_t>& payload) {
        memcached_set(memc_, key.c_str(), key.length(),
                      reinterpret_cast<const char*>(payload.data()), payload.size(),
                      ttl_seconds_, 0);
    }
};

} // namespace runtime
} // namespace helix
