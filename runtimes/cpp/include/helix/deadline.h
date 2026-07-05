#pragma once

#include <string_view>
#include <chrono>
#include <stdexcept>
#include <cctype>

namespace helix {

inline std::chrono::microseconds ParseGRPCTimeout(std::string_view timeout_str) {
    if (timeout_str.empty()) {
        throw std::runtime_error("empty timeout header");
    }

    size_t val_end = 0;
    while (val_end < timeout_str.size() && std::isdigit(timeout_str[val_end])) {
        val_end++;
    }

    if (val_end == 0 || val_end == timeout_str.size()) {
        throw std::runtime_error("invalid timeout format");
    }

    long long value = std::stoll(std::string(timeout_str.substr(0, val_end)));
    char unit = timeout_str[val_end];

    switch (unit) {
        case 'n': return std::chrono::duration_cast<std::chrono::microseconds>(std::chrono::nanoseconds(value));
        case 'u': return std::chrono::microseconds(value);
        case 'm': return std::chrono::duration_cast<std::chrono::microseconds>(std::chrono::milliseconds(value));
        case 'S': return std::chrono::duration_cast<std::chrono::microseconds>(std::chrono::seconds(value));
        case 'M': return std::chrono::duration_cast<std::chrono::microseconds>(std::chrono::minutes(value));
        case 'H': return std::chrono::duration_cast<std::chrono::microseconds>(std::chrono::hours(value));
        default:
            throw std::runtime_error("unknown timeout unit");
    }
}

} // namespace helix
