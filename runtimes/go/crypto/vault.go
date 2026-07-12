package crypto

import (
	"log"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"
)

// HelixVault manages dynamic keys retrieved from HashiCorp Vault.
type HelixVault struct {
	client     *api.Client
	secretPath string
	keys       map[string]string
	mu         sync.RWMutex
	stopChan   chan struct{}
}

// NewHelixVault creates a new HelixVault instance.
func NewHelixVault(client *api.Client, secretPath string) *HelixVault {
	return &HelixVault{
		client:     client,
		secretPath: secretPath,
		keys:       make(map[string]string),
		stopChan:   make(chan struct{}),
	}
}

// StartPolling begins polling Vault for secret updates at the given interval.
func (h *HelixVault) StartPolling(interval time.Duration) {
	// Do an initial load
	h.reloadKeys()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.reloadKeys()
			case <-h.stopChan:
				return
			}
		}
	}()
}

// StopPolling stops the background polling goroutine.
func (h *HelixVault) StopPolling() {
	close(h.stopChan)
}

func (h *HelixVault) reloadKeys() {
	secret, err := h.client.Logical().Read(h.secretPath)
	if err != nil {
		log.Printf("Error reading from Vault: %v", err)
		return
	}
	if secret == nil || secret.Data == nil {
		log.Println("Vault secret data is empty")
		return
	}

	newKeys := make(map[string]string)
	for k, v := range secret.Data {
		if strVal, ok := v.(string); ok {
			newKeys[k] = strVal
		}
	}

	h.mu.Lock()
	h.keys = newKeys
	h.mu.Unlock()
}

// GetKey retrieves a dynamically loaded key by its name.
func (h *HelixVault) GetKey(keyName string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	val, ok := h.keys[keyName]
	return val, ok
}
