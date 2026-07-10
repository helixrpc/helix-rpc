package runtime

import (
	"fmt"
	"net"
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

// CoreDNSResolver resolves standard Kubernetes service DNS names.
// In simple scenarios, this acts just like DNS, but it can be extended 
// with client-go endpoints watch for more advanced client-side load balancing.
type CoreDNSResolver struct {
	// Namespace for the resolution (e.g. "default")
	Namespace string
}

func (k *CoreDNSResolver) Resolve(serviceName string) ([]string, error) {
	fqdn := serviceName
	if k.Namespace != "" {
		fqdn = serviceName + "." + k.Namespace + ".svc.cluster.local"
	}
	
	// Real DNS lookup for CoreDNS
	addrs, err := net.LookupHost(fqdn)
	if err != nil {
		return nil, err
	}

	return addrs, nil
}
