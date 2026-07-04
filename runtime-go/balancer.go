package runtime

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

// endpoint holds the atomic active-connection count for one backend target.
// It is padded to a full cache line (64 bytes) to eliminate false sharing
// between cores on high-core-count machines.
type endpoint struct {
	active int64
	_      [56]byte // padding to 64-byte cache line
}

// LeastConnBalancer tracks active requests per target using lock-free atomics.
// Targets are stored in a pre-indexed slice; a concurrent-safe index map is
// built lazily and promoted with a single pointer swap so that the hot path
// (Next/Done) never acquires a mutex.
type LeastConnBalancer struct {
	// mu guards only the slow path: registering a new target.
	mu sync.Mutex
	// index is an immutable snapshot (*indexMap) swapped atomically.
	index unsafe.Pointer // *indexMap
	// eps is the backing array; its elements are aligned to cache lines.
	eps []*endpoint
}

type indexMap struct {
	m    map[string]int // target address → slice index in eps
	keys []string       // ordered list for iteration
}

func NewLeastConnBalancer() *LeastConnBalancer {
	b := &LeastConnBalancer{}
	empty := &indexMap{m: make(map[string]int)}
	atomic.StorePointer(&b.index, unsafe.Pointer(empty))
	return b
}

// Register pre-warms the balancer with a fixed set of targets.
// Must be called before the first call to Next; safe to call concurrently.
func (b *LeastConnBalancer) Register(targets []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	old := (*indexMap)(atomic.LoadPointer(&b.index))
	next := &indexMap{
		m:    make(map[string]int, len(old.m)+len(targets)),
		keys: make([]string, 0, len(old.m)+len(targets)),
	}
	// Copy existing entries
	for t, idx := range old.m {
		next.m[t] = idx
		next.keys = append(next.keys, t)
	}
	// Add new entries
	for _, t := range targets {
		if _, exists := next.m[t]; !exists {
			idx := len(b.eps)
			b.eps = append(b.eps, &endpoint{})
			next.m[t] = idx
			next.keys = append(next.keys, t)
		}
	}
	atomic.StorePointer(&b.index, unsafe.Pointer(next))
}

// Next selects the target with the fewest in-flight requests (lock-free hot path).
// If a target appears in `targets` that is not yet registered it is registered
// on-the-fly using the slow path.
func (b *LeastConnBalancer) Next(targets []string) (string, error) {
	if len(targets) == 0 {
		return "", fmt.Errorf("no targets available for load balancing")
	}

	idx := (*indexMap)(atomic.LoadPointer(&b.index))

	// Ensure all targets are registered (slow path only on first encounter).
	for _, t := range targets {
		if _, ok := idx.m[t]; !ok {
			b.Register([]string{t})
			idx = (*indexMap)(atomic.LoadPointer(&b.index))
		}
	}

	// Snapshot all active counts *before* choosing so the increment of the
	// winner does not bias the comparison of subsequent targets.
	selected := ""
	var minActive int64 = -1
	for _, t := range targets {
		i := idx.m[t]
		cur := atomic.LoadInt64(&b.eps[i].active)
		if minActive < 0 || cur < minActive {
			minActive = cur
			selected = t
		}
	}

	// Optimistically increment; caller must call Done when finished.
	atomic.AddInt64(&b.eps[idx.m[selected]].active, 1)
	return selected, nil
}

// Done decrements the active-connection counter for the given target.
// It is safe to call from any goroutine; no lock is taken.
func (b *LeastConnBalancer) Done(target string) {
	idx := (*indexMap)(atomic.LoadPointer(&b.index))
	if i, ok := idx.m[target]; ok {
		atomic.AddInt64(&b.eps[i].active, -1)
	}
}

// ActiveConns returns the current in-flight count for a target (for observability).
func (b *LeastConnBalancer) ActiveConns(target string) int64 {
	idx := (*indexMap)(atomic.LoadPointer(&b.index))
	if i, ok := idx.m[target]; ok {
		return atomic.LoadInt64(&b.eps[i].active)
	}
	return 0
}
