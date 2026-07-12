#pragma once

#ifdef _WIN32
#include <winsock2.h>
#include <ws2tcpip.h>
#pragma comment(lib, "ws2_32.lib")
typedef int socklen_t;
typedef int ssize_t;
#define close closesocket
#else
#include <sys/socket.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <unistd.h>
#endif
#include <thread>
#include <functional>
#include <string>
#include <vector>
#include <stdexcept>
#include <iostream>

namespace helix {

class MultiplexedServer {
public:
    explicit MultiplexedServer(int port = 0) {
        fd_ = socket(AF_INET, SOCK_STREAM, 0);
        if (fd_ < 0) throw std::runtime_error("failed to create server socket");

        int opt = 1;
        setsockopt(fd_, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));

        struct sockaddr_in addr{};
        addr.sin_family = AF_INET;
        addr.sin_addr.s_addr = INADDR_ANY;
        addr.sin_port = htons(port);

        if (bind(fd_, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
            close(fd_);
            throw std::runtime_error("failed to bind server socket");
        }

        if (listen(fd_, 10) < 0) {
            close(fd_);
            throw std::runtime_error("failed to listen on server socket");
        }

        running_ = true;
    }

    ~MultiplexedServer() {
        Stop();
    }

    int GetPort() const {
        struct sockaddr_in addr{};
        socklen_t len = sizeof(addr);
        if (getsockname(fd_, (struct sockaddr*)&addr, &len) == 0) {
            return ntohs(addr.sin_port);
        }
        return 0;
    }

    void Start(std::function<void(int)> grpc_handler, std::function<void(int)> http_handler) {
        thread_ = std::thread([this, grpc_handler, http_handler]() {
            while (running_) {
                struct sockaddr_in client_addr{};
                socklen_t client_len = sizeof(client_addr);
                int client_fd = accept(fd_, (struct sockaddr*)&client_addr, &client_len);
                if (client_fd < 0) {
                    if (!running_) break;
                    continue;
                }

                int flag = 1;
                setsockopt(client_fd, IPPROTO_TCP, TCP_NODELAY, (char*)&flag, sizeof(int));
                setsockopt(client_fd, SOL_SOCKET, SO_KEEPALIVE, (char*)&flag, sizeof(int));

                std::thread([client_fd, grpc_handler, http_handler]() {
                    std::vector<char> peek_buf(8);
                    ssize_t n = recv(client_fd, peek_buf.data(), peek_buf.size(), MSG_PEEK);
                    if (n >= 4 && std::string(peek_buf.data(), 4) == "PRI ") {
                        grpc_handler(client_fd);
                    } else {
                        http_handler(client_fd);
                    }
                    close(client_fd);
                }).detach();
            }
        });
    }

    void Stop() {
        running_ = false;
        if (fd_ >= 0) {
            close(fd_);
            fd_ = -1;
        }
        if (thread_.joinable()) {
            thread_.join();
        }
    }

private:
    int fd_;
    bool running_;
    std::thread thread_;
};

inline void WriteSseChunk(int client_fd, const std::string& data) {
    std::string chunk = "data: " + data + "\n\n";
    send(client_fd, chunk.data(), chunk.size(), 0);
}

} // namespace helix
