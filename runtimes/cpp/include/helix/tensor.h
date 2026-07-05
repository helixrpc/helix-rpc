#pragma once

#include <string>
#include <vector>
#include <stdexcept>

namespace helix {

struct Tensor {
    std::string dtype;
    std::vector<int64_t> shape;
    std::vector<uint8_t> data;
};

template<typename T>
const T* GetTensorData(const Tensor& t) {
    if (t.data.empty()) return nullptr;
    return reinterpret_cast<const T*>(t.data.data());
}

template<typename T>
T* GetTensorData(Tensor& t) {
    if (t.data.empty()) return nullptr;
    return reinterpret_cast<T*>(t.data.data());
}

} // namespace helix
