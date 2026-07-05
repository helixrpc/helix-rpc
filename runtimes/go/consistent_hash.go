package runtime

import (
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"
	"sync"
)

// ConsistentHashBalancer implements consistent hashing for prefix-based (KVCache) routing.
type ConsistentHashBalancer struct {
	mu           sync.RWMutex
	replicas     int               // Number of virtual nodes per physical node
	ring         []uint32          // Sorted list of virtual node hashes
	hashMap      map[uint32]string // Virtual node hash → Physical node address
	registered   map[string]bool   // Set of registered physical nodes
}

// NewConsistentHashBalancer creates a new ConsistentHashBalancer with a default replica count of 50.
func NewConsistentHashBalancer(replicas int) *ConsistentHashBalancer {
	if replicas <= 0 {
		replicas = 50
	}
	return &ConsistentHashBalancer{
		replicas:   replicas,
		hashMap:    make(map[uint32]string),
		registered: make(map[string]bool),
	}
}

// Add adds physical nodes to the hash ring.
func (b *ConsistentHashBalancer) Add(nodes ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, node := range nodes {
		if b.registered[node] {
			continue
		}
		b.registered[node] = true
		for i := 0; i < b.replicas; i++ {
			hash := crc32.ChecksumIEEE([]byte(node + "#" + strconv.Itoa(i)))
			b.ring = append(b.ring, hash)
			b.hashMap[hash] = node
		}
	}
	sort.Slice(b.ring, func(i, j int) bool { return b.ring[i] < b.ring[j] })
}

// Remove removes physical nodes from the hash ring.
func (b *ConsistentHashBalancer) Remove(node string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.registered[node] {
		return
	}
	delete(b.registered, node)

	newRing := make([]uint32, 0, len(b.ring)-(b.replicas))
	for _, hash := range b.ring {
		if b.hashMap[hash] == node {
			delete(b.hashMap, hash)
		} else {
			newRing = append(newRing, hash)
		}
	}
	b.ring = newRing
}

// Next selects the node nearest to the hash ring index for a given key.
func (b *ConsistentHashBalancer) Next(targets []string) (string, error) {
	return b.NextWithKey(targets, "default-key")
}

// NextWithKey selects a physical target address consistently based on the key.
func (b *ConsistentHashBalancer) NextWithKey(targets []string, key string) (string, error) {
	if len(targets) == 0 {
		return "", fmt.Errorf("no targets available for load balancing")
	}

	b.mu.Lock()
	// Lazily register any target not present on the ring
	needsSort := false
	for _, target := range targets {
		if !b.registered[target] {
			b.registered[target] = true
			for i := 0; i < b.replicas; i++ {
				hash := crc32.ChecksumIEEE([]byte(target + "#" + strconv.Itoa(i)))
				b.ring = append(b.ring, hash)
				b.hashMap[hash] = target
			}
			needsSort = true
		}
	}
	if needsSort {
		sort.Slice(b.ring, func(i, j int) bool { return b.ring[i] < b.ring[j] })
	}
	b.mu.Unlock()

	b.mu.RLock()
	defer b.mu.RUnlock()

	// Filter ring to only include targets that are present in the targets list
	targetMap := make(map[string]bool, len(targets))
	for _, target := range targets {
		targetMap[target] = true
	}

	if len(b.ring) == 0 {
		return targets[0], nil
	}

	hash := crc32.ChecksumIEEE([]byte(key))
	idx := sort.Search(len(b.ring), func(i int) bool {
		return b.ring[i] >= hash
	})

	// Wrap around if we reach the end of the ring
	if idx == len(b.ring) {
		idx = 0
	}

	// Find the first virtual node that is present in the targets list
	startIdx := idx
	for {
		node := b.hashMap[b.ring[idx]]
		if targetMap[node] {
			return node, nil
		}
		idx = (idx + 1) % len(b.ring)
		if idx == startIdx {
			break
		}
	}

	return targets[0], nil
}
