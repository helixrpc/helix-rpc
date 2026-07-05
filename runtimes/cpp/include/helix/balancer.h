#pragma once

#include <string>
#include <vector>
#include <map>
#include <set>
#include <algorithm>
#include <functional>
#include <stdexcept>

namespace helix {

class ConsistentHashBalancer {
public:
    explicit ConsistentHashBalancer(size_t replicas = 50) : replicas_(replicas) {
        if (replicas_ == 0) replicas_ = 50;
    }

    void Add(const std::string& node) {
        if (registered_.count(node)) return;
        registered_.insert(node);

        for (size_t i = 0; i < replicas_; ++i) {
            std::string virtual_key = node + "#" + std::to_string(i);
            size_t hash = std::hash<std::string>{}(virtual_key);
            ring_.push_back(hash);
            hash_map_[hash] = node;
        }
        std::sort(ring_.begin(), ring_.end());
    }

    void Remove(const std::string& node) {
        if (!registered_.count(node)) return;
        registered_.erase(node);

        std::vector<size_t> new_ring;
        for (auto hash : ring_) {
            if (hash_map_[hash] == node) {
                hash_map_.erase(hash);
            } else {
                new_ring.push_back(hash);
            }
        }
        ring_ = std::move(new_ring);
    }

    std::string NextWithKey(const std::vector<std::string>& targets, const std::string& key) {
        if (targets.empty()) {
            throw std::runtime_error("no targets available for load balancing");
        }

        // Lazily register any unregistered target
        for (const auto& target : targets) {
            if (!registered_.count(target)) {
                Add(target);
            }
        }

        if (ring_.empty()) {
            return targets[0];
        }

        size_t hash = std::hash<std::string>{}(key);
        auto it = std::lower_bound(ring_.begin(), ring_.end(), hash);
        
        size_t idx = 0;
        if (it != ring_.end()) {
            idx = std::distance(ring_.begin(), it);
        }

        std::set<std::string> target_set(targets.begin(), targets.end());
        size_t start_idx = idx;

        do {
            std::string node = hash_map_[ring_[idx]];
            if (target_set.count(node)) {
                return node;
            }
            idx = (idx + 1) % ring_.size();
        } while (idx != start_idx);

        return targets[0];
    }

private:
    size_t replicas_;
    std::vector<size_t> ring_;
    std::map<size_t, std::string> hash_map_;
    std::set<std::string> registered_;
};

} // namespace helix
