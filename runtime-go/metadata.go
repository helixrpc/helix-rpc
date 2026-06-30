package runtime

import (
	"context"
	"strings"
)

type mdKey struct{}

// MD represents the metadata map of key-value pairs propagated across boundaries.
// Keys are normalized to lowercase.
type MD map[string][]string

// New creates metadata from a simple map of string key-value pairs.
func New(m map[string]string) MD {
	md := make(MD, len(m))
	for k, v := range m {
		key := strings.ToLower(k)
		md[key] = []string{v}
	}
	return md
}

// Get returns the values associated with the given metadata key.
func (md MD) Get(key string) []string {
	k := strings.ToLower(key)
	return md[k]
}

// Set stores the values for a metadata key.
func (md MD) Set(key string, values ...string) {
	k := strings.ToLower(key)
	md[k] = values
}

// NewContext returns a new context containing the given metadata.
func NewContext(ctx context.Context, md MD) context.Context {
	return context.WithValue(ctx, mdKey{}, md)
}

// FromContext returns the metadata from the context, if present.
func FromContext(ctx context.Context) (MD, bool) {
	md, ok := ctx.Value(mdKey{}).(MD)
	return md, ok
}
