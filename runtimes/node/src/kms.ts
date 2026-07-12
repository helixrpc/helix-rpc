/**
 * HelixKMS provides envelope encryption and decryption utility using AWS KMS.
 * This is used to transparently encrypt highly sensitive fields (e.g., PII)
 * within FlatBuffers or Protobuf payloads before broadcasting them.
 */
export class HelixKMS {
    private client: any; // KMSClient
    private keyId: string;
    private region: string;
    private awsKmsModule: any;

    constructor(region: string, keyId: string) {
        this.region = region;
        this.keyId = keyId;
    }

    private async init() {
        if (!this.client) {
            try {
                this.awsKmsModule = await import("@aws-sdk/client-kms");
                this.client = new this.awsKmsModule.KMSClient({ region: this.region });
            } catch (err) {
                console.warn("HelixKMS: @aws-sdk/client-kms not installed. Encryption/Decryption will fail.");
                throw new Error("Missing peer dependency @aws-sdk/client-kms");
            }
        }
    }

    /**
     * Encrypts a raw Uint8Array payload using the provided KMS Key ID.
     */
    async encryptPayload(plaintext: Uint8Array): Promise<Uint8Array> {
        await this.init();
        const command = new this.awsKmsModule.EncryptCommand({
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
        await this.init();
        const command = new this.awsKmsModule.DecryptCommand({
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
