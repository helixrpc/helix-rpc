#pragma once
#define NOMINMAX

#include <string>
#include <unordered_map>
#include <mutex>
#include <chrono>
#include <algorithm>

namespace helix {

class TenantLimiter {
public:
    TenantLimiter(double capacity, double rate)
        : capacity_(capacity), rate_(rate), tokens_(capacity), last_refill_(std::chrono::steady_clock::now()) {}

    bool Consume(double count) {
        auto now = std::chrono::steady_clock::now();
        double elapsed = std::chrono::duration<double>(now - last_refill_).count();
        tokens_ = std::min(capacity_, tokens_ + elapsed * rate_);
        last_refill_ = now;

        if (tokens_ >= count) {
            tokens_ -= count;
            return true;
        }
        return false;
    }

private:
    double capacity_;
    double rate_;
    double tokens_;
    std::chrono::steady_clock::time_point last_refill_;
};

class MultiTenantRateLimiter {
public:
    MultiTenantRateLimiter(double default_capacity, double default_rate)
        : default_capacity_(default_capacity), default_rate_(default_rate) {}

    void SetTenantLimit(const std::string& tenant_id, double capacity, double rate) {
        std::lock_guard<std::mutex> lock(mutex_);
        limiters_[tenant_id] = std::make_unique<TenantLimiter>(capacity, rate);
    }

    bool Allow(const std::string& tenant_id, double count) {
        std::lock_guard<std::mutex> lock(mutex_);
        auto it = limiters_.find(tenant_id);
        if (it == limiters_.end()) {
            auto limiter = std::make_unique<TenantLimiter>(default_capacity_, default_rate_);
            auto* ptr = limiter.get();
            limiters_[tenant_id] = std::move(limiter);
            return ptr->Consume(count);
        }
        return it->second->Consume(count);
    }

private:
    double default_capacity_;
    double default_rate_;
    std::mutex mutex_;
    std::unordered_map<std::string, std::unique_ptr<TenantLimiter>> limiters_;
};

} // namespace helix
