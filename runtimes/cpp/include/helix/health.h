#pragma once

#include <string>
#include <unordered_map>
#include <mutex>

namespace helix {

enum class HealthStatus {
    UNKNOWN = 0,
    SERVING = 1,
    NOT_SERVING = 2
};

class HealthChecker {
public:
    void SetServingStatus(const std::string& service, HealthStatus status) {
        std::lock_guard<std::mutex> lock(mutex_);
        statuses_[service] = status;
    }

    HealthStatus Check(const std::string& service) {
        std::lock_guard<std::mutex> lock(mutex_);
        auto it = statuses_.find(service);
        if (it != statuses_.end()) {
            return it->second;
        }
        return HealthStatus::UNKNOWN;
    }

private:
    std::mutex mutex_;
    std::unordered_map<std::string, HealthStatus> statuses_;
};

} // namespace helix
