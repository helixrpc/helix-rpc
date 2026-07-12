#pragma once

#ifdef HELIX_ENABLE_CRYPTO

#include <string>
#include <vector>
#include <iostream>

namespace helix {

class HelixKMS {
public:
    HelixKMS(const std::string& region = "us-east-1") : region_(region) {}

    std::string encrypt(const std::string& plaintext, const std::string& key_id) {
        // Dummy/mock implementation for encryption
        std::cout << "[HelixKMS] Encrypting data with key " << key_id << " in region " << region_ << "\n";
        return "encrypted:" + plaintext;
    }

    std::string decrypt(const std::string& ciphertext, const std::string& key_id) {
        // Dummy/mock implementation for decryption
        std::cout << "[HelixKMS] Decrypting data with key " << key_id << " in region " << region_ << "\n";
        if (ciphertext.find("encrypted:") == 0) {
            return ciphertext.substr(10);
        }
        return ciphertext;
    }

private:
    std::string region_;
};

} // namespace helix

#endif // HELIX_ENABLE_CRYPTO
