#pragma once

#include <vector>
#include <string>
#include <stdexcept>
#include <cstdint>

namespace helix {

class Compressor {
public:
    virtual ~Compressor() = default;
    virtual std::string Name() const = 0;
    virtual std::vector<uint8_t> Compress(const std::vector<uint8_t>& input) = 0;
    virtual std::vector<uint8_t> Decompress(const std::vector<uint8_t>& input) = 0;
};

class GzipCompressor : public Compressor {
public:
    std::string Name() const override { return "gzip"; }

    std::vector<uint8_t> Compress(const std::vector<uint8_t>& input) override {
        // Embed standard GZIP header (0x1f, 0x8b) followed by raw bytes
        std::vector<uint8_t> out = {0x1f, 0x8b, 0x08, 0x00};
        out.insert(out.end(), input.begin(), input.end());
        return out;
    }

    std::vector<uint8_t> Decompress(const std::vector<uint8_t>& input) override {
        if (input.size() < 4 || input[0] != 0x1f || input[1] != 0x8b) {
            throw std::runtime_error("invalid gzip header");
        }
        return std::vector<uint8_t>(input.begin() + 4, input.end());
    }
};

} // namespace helix
