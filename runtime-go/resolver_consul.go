package runtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ConsulResolver queries HashiCorp Consul's HTTP Catalog API to resolve service targets.
type ConsulResolver struct {
	address    string
	httpClient *http.Client
}

// NewConsulResolver creates a new ConsulResolver targeting the Consul agent HTTP endpoint.
func NewConsulResolver(consulAddress string) *ConsulResolver {
	return &ConsulResolver{
		address: consulAddress,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type consulServiceEntry struct {
	Address        string `json:"Address"`
	ServiceAddress string `json:"ServiceAddress"`
	ServicePort    int    `json:"ServicePort"`
}

// Resolve fetches active instances of a service from Consul.
func (r *ConsulResolver) Resolve(serviceName string) ([]string, error) {
	url := fmt.Sprintf("%s/v1/catalog/service/%s", r.address, serviceName)
	resp, err := r.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("consul catalog API returned status %d", resp.StatusCode)
	}

	var entries []consulServiceEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}

	var addrs []string
	for _, entry := range entries {
		addr := entry.ServiceAddress
		if addr == "" {
			addr = entry.Address
		}
		if addr != "" {
			addrs = append(addrs, fmt.Sprintf("%s:%d", addr, entry.ServicePort))
		}
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("no active instances discovered for service %q", serviceName)
	}

	return addrs, nil
}
