#pragma once

#include <vector>
#include <string>
#include <map>
#include <queue>
#include <mutex>
#include <thread>
#include <cstdint>
#include <stdexcept>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <unistd.h>

namespace helix {

class QuicVirtualStream {
public:
    QuicVirtualStream(uint32_t stream_id, int fd, struct sockaddr_in addr)
        : stream_id_(stream_id), fd_(fd), addr_(addr) {}

    void Write(const std::vector<uint8_t>& data) {
        std::vector<uint8_t> packet;
        packet.push_back(static_cast<uint8_t>((stream_id_ >> 24) & 0xFF));
        packet.push_back(static_cast<uint8_t>((stream_id_ >> 16) & 0xFF));
        packet.push_back(static_cast<uint8_t>((stream_id_ >> 8) & 0xFF));
        packet.push_back(static_cast<uint8_t>(stream_id_ & 0xFF));
        packet.insert(packet.end(), data.begin(), data.end());

        sendto(fd_, packet.data(), packet.size(), 0, (struct sockaddr*)&addr_, sizeof(addr_));
    }

    void PushData(const std::vector<uint8_t>& data) {
        std::lock_guard<std::mutex> lock(mutex_);
        queue_.push(data);
    }

    std::vector<uint8_t> Read() {
        std::lock_guard<std::mutex> lock(mutex_);
        if (queue_.empty()) return {};
        auto res = std::move(queue_.front());
        queue_.pop();
        return res;
    }

private:
    uint32_t stream_id_;
    int fd_;
    struct sockaddr_in addr_;
    std::mutex mutex_;
    std::queue<std::vector<uint8_t>> queue_;
};

class QuicListener {
public:
    explicit QuicListener(int port = 0) {
        fd_ = socket(AF_INET, SOCK_DGRAM, 0);
        if (fd_ < 0) throw std::runtime_error("failed to create UDP socket");

        struct sockaddr_in addr{};
        addr.sin_family = AF_INET;
        addr.sin_addr.s_addr = INADDR_ANY;
        addr.sin_port = htons(port);

        if (bind(fd_, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
            close(fd_);
            throw std::runtime_error("failed to bind UDP socket");
        }

        running_ = true;
        thread_ = std::thread(&QuicListener::RunLoop, this);
    }

    ~QuicListener() {
        running_ = false;
        if (fd_ >= 0) close(fd_);
        if (thread_.joinable()) thread_.join();

        for (auto& entry : streams_) {
            delete entry.second;
        }
    }

    int GetPort() const {
        struct sockaddr_in addr{};
        socklen_t len = sizeof(addr);
        if (getsockname(fd_, (struct sockaddr*)&addr, &len) == 0) {
            return ntohs(addr.sin_port);
        }
        return 0;
    }

    QuicVirtualStream* Accept() {
        std::lock_guard<std::mutex> lock(mutex_);
        if (accept_queue_.empty()) return nullptr;
        auto* res = accept_queue_.front();
        accept_queue_.pop();
        return res;
    }

private:
    void RunLoop() {
        std::vector<uint8_t> buffer(65535);
        struct sockaddr_in client_addr{};
        socklen_t client_len = sizeof(client_addr);

        while (running_) {
            ssize_t n = recvfrom(fd_, buffer.data(), buffer.size(), 0, (struct sockaddr*)&client_addr, &client_len);
            if (n < 4) continue;

            uint32_t stream_id = (static_cast<uint32_t>(buffer[0]) << 24) |
                                 (static_cast<uint32_t>(buffer[1]) << 16) |
                                 (static_cast<uint32_t>(buffer[2]) << 8)  |
                                 static_cast<uint32_t>(buffer[3]);

            std::string client_ip = inet_ntoa(client_addr.sin_addr);
            std::string key = client_ip + ":" + std::to_string(ntohs(client_addr.sin_port)) + ":" + std::to_string(stream_id);

            std::lock_guard<std::mutex> lock(mutex_);
            auto it = streams_.find(key);
            QuicVirtualStream* stream = nullptr;
            if (it == streams_.end()) {
                stream = new QuicVirtualStream(stream_id, fd_, client_addr);
                streams_[key] = stream;
                accept_queue_.push(stream);
            } else {
                stream = it->second;
            }

            std::vector<uint8_t> payload(buffer.begin() + 4, buffer.begin() + n);
            stream->PushData(payload);
        }
    }

    int fd_;
    bool running_;
    std::thread thread_;
    std::mutex mutex_;
    std::map<std::string, QuicVirtualStream*> streams_;
    std::queue<QuicVirtualStream*> accept_queue_;
};

} // namespace helix
