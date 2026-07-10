package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"log"

	"github.com/bradfitz/gomemcache/memcache"
)

// CacheInterceptor provides unary response caching via Memcached
type CacheInterceptor struct {
	client *memcache.Client
	ttl    int32 // Time to live in seconds
}

func NewCacheInterceptor(memcachedServers []string, ttlSeconds int32) *CacheInterceptor {
	return &CacheInterceptor{
		client: memcache.New(memcachedServers...),
		ttl:    ttlSeconds,
	}
}

// GenerateCacheKey computes a SHA256 hash of the method name and the request payload.
func (c *CacheInterceptor) GenerateCacheKey(method string, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// Get attempts to retrieve a cached response.
func (c *CacheInterceptor) Get(key string) ([]byte, bool) {
	item, err := c.client.Get(key)
	if err == nil && item != nil {
		return item.Value, true
	}
	return nil, false
}

// Set asynchronously caches the response.
func (c *CacheInterceptor) Set(key string, payload []byte) {
	go func() {
		err := c.client.Set(&memcache.Item{
			Key:        key,
			Value:      payload,
			Expiration: c.ttl,
		})
		if err != nil {
			log.Printf("Failed to set cache for key %s: %v", key, err)
		}
	}()
}
