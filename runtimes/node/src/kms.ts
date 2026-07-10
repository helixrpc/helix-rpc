import { KMSClient, EncryptCommand, DecryptCommand } from "@aws-sdk/client-kms";

/**
 * HelixKMS provides envelope encryption and decryption utility using AWS KMS.
 * This is used to transparently encrypt highly sensitive fields (e.g., PII)
 * within FlatBuffers or Protobuf payloads before broadcasting them.
 */
export class HelixKMS {
    private client: KMSClient;
    private keyId: string;

    constructor(region: string, keyId: string) {
        this.client = new KMSClient({ region });
        this.keyId = keyId;
    }

    /**
     * Encrypts a raw Uint8Array payload using the provided KMS Key ID.
     */
    async encryptPayload(plaintext: Uint8Array): Promise<Uint8Array> {
        const command = new EncryptCommand({
            KeyId: this.keyId,
            Plaintext: plaintext,
        });
        
        try {
            const response = await this.client.send(command);
            if (!response.CiphertextBlob) {
                throw new Error("KMS Encryption failed: Missing CiphertextBlob");
            }
            return response.CiphertextBlob;
        } catch (error) {
            console.error("Helix KMS Encryption Error:", error);
            throw error;
        }
    }

    /**
     * Decrypts an encrypted Uint8Array payload using AWS KMS.
     */
    async decryptPayload(ciphertext: Uint8Array): Promise<Uint8Array> {
        const command = new DecryptCommand({
            CiphertextBlob: ciphertext,
        });

        try {
            const response = await this.client.send(command);
            if (!response.Plaintext) {
                throw new Error("KMS Decryption failed: Missing Plaintext");
            }
            return response.Plaintext;
        } catch (error) {
            console.error("Helix KMS Decryption Error:", error);
            throw error;
        }
    }
}
