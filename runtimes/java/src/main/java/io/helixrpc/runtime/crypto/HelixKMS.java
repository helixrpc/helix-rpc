package io.helixrpc.runtime.crypto;

import software.amazon.awssdk.core.SdkBytes;
import software.amazon.awssdk.services.kms.KmsClient;
import software.amazon.awssdk.services.kms.model.DecryptRequest;
import software.amazon.awssdk.services.kms.model.DecryptResponse;
import software.amazon.awssdk.services.kms.model.GenerateDataKeyRequest;
import software.amazon.awssdk.services.kms.model.GenerateDataKeyResponse;

public class HelixKMS {
    private final KmsClient kmsClient;
    private final String keyId;

    public HelixKMS(KmsClient kmsClient, String keyId) {
        this.kmsClient = kmsClient;
        this.keyId = keyId;
    }

    public Envelope generateDataKey() {
        GenerateDataKeyRequest dataKeyRequest = GenerateDataKeyRequest.builder()
                .keyId(keyId)
                .keySpec("AES_256")
                .build();

        GenerateDataKeyResponse response = kmsClient.generateDataKey(dataKeyRequest);
        return new Envelope(
                response.plaintext().asByteArray(),
                response.ciphertextBlob().asByteArray()
        );
    }

    public byte[] decryptDataKey(byte[] encryptedDataKey) {
        DecryptRequest decryptRequest = DecryptRequest.builder()
                .ciphertextBlob(SdkBytes.fromByteArray(encryptedDataKey))
                .build();

        DecryptResponse decryptResponse = kmsClient.decrypt(decryptRequest);
        return decryptResponse.plaintext().asByteArray();
    }

    public static class Envelope {
        private final byte[] plaintext;
        private final byte[] ciphertext;

        public Envelope(byte[] plaintext, byte[] ciphertext) {
            this.plaintext = plaintext;
            this.ciphertext = ciphertext;
        }

        public byte[] getPlaintext() {
            return plaintext;
        }

        public byte[] getCiphertext() {
            return ciphertext;
        }
    }
}
