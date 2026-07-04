package runtime

import (
	"fmt"
	"sync"
)

// Resolver defines the pluggable interface for discovering target addresses.
type Resolver interface {
	Resolve(serviceName string) ([]string, error)
}

// StaticResolver is a simple map-backed resolver.
type StaticResolver struct {
	mu      sync.RWMutex
	targets map[string][]string
}

func NewStaticResolver() *StaticResolver {
	return &StaticResolver{
		targets: make(map[string][]string),
	}
}

func (r *StaticResolver) Register(serviceName string, addresses []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.targets[serviceName] = addresses
}

func (r *StaticResolver) Resolve(serviceName string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	addrs, ok := r.targets[serviceName]
	if !ok {
		return nil, fmt.Errorf("service %s not found in resolver targets", serviceName)
	}
	return addrs, nil
}
