package runtime

import (
	"testing"
)

func TestCacheInterceptor_GenerateCacheKey(t *testing.T) {
	interceptor := NewCacheInterceptor([]string{"localhost:11211"}, 60)

	key1 := interceptor.GenerateCacheKey("GetUser", []byte(`{"id": 1}`))
	key2 := interceptor.GenerateCacheKey("GetUser", []byte(`{"id": 1}`))
	key3 := interceptor.GenerateCacheKey("GetUser", []byte(`{"id": 2}`))

	if key1 != key2 {
		t.Errorf("Expected identical keys for identical method and payload, got %s and %s", key1, key2)
	}

	if key1 == key3 {
		t.Errorf("Expected different keys for different payloads, got %s", key1)
	}
}
