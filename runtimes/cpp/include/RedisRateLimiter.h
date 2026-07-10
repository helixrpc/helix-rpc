#pragma once

#include <sw/redis++/redis++.h>
#include <string>
#include <chrono>
#include <cmath>
#include <memory>
#include <optional>

namespace helix {
namespace runtime {

const std::string LUA_TOKEN_BUCKET = R"(
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local tokens_key = key .. ':tokens'
local timestamp_key = key .. ':ts'

local last_tokens = tonumber(redis.call('get', tokens_key))
if last_tokens == nil then
    last_tokens = burst
end

local last_refreshed = tonumber(redis.call('get', timestamp_key))
if last_refreshed == nil then
    last_refreshed = 0
end

local delta = math.max(0, now - last_refreshed)
local filled_tokens = math.min(burst, last_tokens + (delta * rate))
local allowed = filled_tokens >= 1
local new_tokens = filled_tokens

if allowed then
    new_tokens = filled_tokens - 1
end

local ttl = math.ceil(burst / rate)
redis.call('setex', tokens_key, ttl, new_tokens)
redis.call('setex', timestamp_key, ttl, now)

if allowed then return 1 else return 0 end
)";

class RedisRateLimiter {
private:
    std::shared_ptr<sw::redis::Redis> redis_;
    double rps_;
    int burst_;
    std::string script_sha_;

public:
    RedisRateLimiter(const std::string& redis_uri, double rps, int burst)
        : redis_(std::make_shared<sw::redis::Redis>(redis_uri)), rps_(rps), burst_(burst) {
        script_sha_ = redis_->script_load(LUA_TOKEN_BUCKET);
    }

    bool allow(const std::string& key) {
        auto now = std::chrono::duration_cast<std::chrono::seconds>(
            std::chrono::system_clock::now().time_since_epoch()).count();
        
        std::vector<std::string> keys = {"ratelimit:" + key};
        std::vector<std::string> args = {
            std::to_string(rps_),
            std::to_string(burst_),
            std::to_string(now)
        };

        long long result = redis_->evalsha<long long>(script_sha_, keys.begin(), keys.end(), args.begin(), args.end());
        return result == 1;
    }
};

} // namespace runtime
} // namespace helix
