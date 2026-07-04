package runtime

import (
	"context"
	"fmt"
	"net"
)

// DNSResolver resolves service targets using standard DNS lookup or SRV record queries.
type DNSResolver struct {
	resolver *net.Resolver
}

// NewDNSResolver creates a new DNSResolver using the system resolver.
func NewDNSResolver() *DNSResolver {
	return &DNSResolver{
		resolver: net.DefaultResolver,
	}
}

// Resolve queries DNS SRV records, falling back to A/AAAA IP address lookups if SRV is not available.
func (r *DNSResolver) Resolve(target string) ([]string, error) {
	_, srvs, err := r.resolver.LookupSRV(context.Background(), "", "", target)
	if err == nil && len(srvs) > 0 {
		var addrs []string
		for _, srv := range srvs {
			addrs = append(addrs, fmt.Sprintf("%s:%d", srv.Target, srv.Port))
		}
		return addrs, nil
	}

	// Fallback to A/AAAA host lookup
	ips, err := r.resolver.LookupHost(context.Background(), target)
	if err != nil {
		return nil, err
	}

	return ips, nil
}
