package go_go

import (
	generated "github.com/helix-rpc/helix/tests/go/generated"
	"testing"
)

func TestFlatBuffersCodec(t *testing.T) {
	original := &generated.UserProfile{
		UserID:   987654321,
		Username: "helix_user",
		Email:    "helix@example.com",
	}

	// Marshal to FlatBuffers
	buf := original.MarshalFlatBuffers()
	if len(buf) == 0 {
		t.Fatalf("MarshalFlatBuffers returned empty buffer")
	}

	// Unmarshal back
	decoded := &generated.UserProfile{}
	err := decoded.UnmarshalFlatBuffers(buf)
	if err != nil {
		t.Fatalf("UnmarshalFlatBuffers failed: %v", err)
	}

	// Assertions
	if decoded.UserID != original.UserID {
		t.Errorf("UserID mismatch: got %d, want %d", decoded.UserID, original.UserID)
	}
	if decoded.Username != original.Username {
		t.Errorf("Username mismatch: got %q, want %q", decoded.Username, original.Username)
	}
	if decoded.Email != original.Email {
		t.Errorf("Email mismatch: got %q, want %q", decoded.Email, original.Email)
	}
}
