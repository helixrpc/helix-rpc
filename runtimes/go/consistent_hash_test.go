package runtime

import (
	"testing"
)

func TestConsistentHashBalancer(t *testing.T) {
	lb := NewConsistentHashBalancer(50)
	targets := []string{"127.0.0.1:8081", "127.0.0.1:8082", "127.0.0.1:8083"}

	// 1. Verifies consistent routing for the same key
	key1 := "system-prompt-llm-1"
	choice1, err := lb.NextWithKey(targets, key1)
	if err != nil {
		t.Fatalf("NextWithKey failed: %v", err)
	}

	for i := 0; i < 20; i++ {
		c, err := lb.NextWithKey(targets, key1)
		if err != nil {
			t.Fatalf("NextWithKey failed on iteration %d: %v", i, err)
		}
		if c != choice1 {
			t.Errorf("consistent hash violated: expected %s, got %s", choice1, c)
		}
	}

	// 2. Verifies different key can route to a stable choice
	key2 := "different-system-prompt"
	choice2, err := lb.NextWithKey(targets, key2)
	if err != nil {
		t.Fatalf("NextWithKey failed: %v", err)
	}

	for i := 0; i < 20; i++ {
		c, err := lb.NextWithKey(targets, key2)
		if err != nil {
			t.Fatalf("NextWithKey failed: %v", err)
		}
		if c != choice2 {
			t.Errorf("consistent hash violated for key2: expected %s, got %s", choice2, c)
		}
	}

	// 3. Verify node removal redistribution
	lb.Remove(choice1)
	newTargets := []string{}
	for _, target := range targets {
		if target != choice1 {
			newTargets = append(newTargets, target)
		}
	}

	cAfterRemove, err := lb.NextWithKey(newTargets, key1)
	if err != nil {
		t.Fatalf("NextWithKey after removal failed: %v", err)
	}
	if cAfterRemove == choice1 {
		t.Errorf("removed node %s was selected", choice1)
	}
}
