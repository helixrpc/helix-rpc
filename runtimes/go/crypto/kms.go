package crypto

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// HelixKMS implements AWS KMS payload encryption and decryption.
type HelixKMS struct {
	client *kms.Client
	keyID  string
}

// NewHelixKMS creates a new HelixKMS instance.
func NewHelixKMS(client *kms.Client, keyID string) *HelixKMS {
	return &HelixKMS{
		client: client,
		keyID:  keyID,
	}
}

// EncryptPayload encrypts a payload using AWS KMS.
func (h *HelixKMS) EncryptPayload(ctx context.Context, payload []byte) ([]byte, error) {
	input := &kms.EncryptInput{
		KeyId:     &h.keyID,
		Plaintext: payload,
	}
	output, err := h.client.Encrypt(ctx, input)
	if err != nil {
		return nil, err
	}
	return output.CiphertextBlob, nil
}

// DecryptPayload decrypts a payload using AWS KMS.
func (h *HelixKMS) DecryptPayload(ctx context.Context, ciphertext []byte) ([]byte, error) {
	input := &kms.DecryptInput{
		KeyId:          &h.keyID,
		CiphertextBlob: ciphertext,
	}
	output, err := h.client.Decrypt(ctx, input)
	if err != nil {
		return nil, err
	}
	return output.Plaintext, nil
}
