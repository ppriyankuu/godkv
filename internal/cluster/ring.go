// Package cluster handles all distributed logic: consistent hashing, node
// membership, replication, and quorum enforcement.
package cluster

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
)

// ConsistentHash implements a hash ring for distributing keys across nodes.
//
// Interview explanation:
//   Naive modular hashing (key % N) is fragile: adding/removing a node
//   remaps almost every key.  Consistent hashing places both nodes and keys
//   on a 2^32 ring.  A key is owned by the first node clockwise from the
//   key's hash.  When a node is added or removed only the keys between that
//   node and its predecessor need to move — on average 1/N of all keys.
//
//   Virtual nodes (vnodes) improve load balance.  Each physical node is
//   mapped to `replicas` positions on the ring, so its slots are spread
//   evenly rather than lumped together.

const defaultVnodes = 150 // number of virtual nodes per physical node

// Ring is a consistent-hash ring.  Safe for concurrent use.
type Ring struct {
	mu     sync.RWMutex
	vnodes int
	ring   map[uint32]string // position → nodeID
	sorted []uint32          // sorted ring positions for binary search
}

// NewRing creates an empty ring.  vnodes controls load balance; 100–200 is typical.
func NewRing(vnodes int) *Ring {
	if vnodes <= 0 {
		vnodes = defaultVnodes
	}
	return &Ring{
		vnodes: vnodes,
		ring:   make(map[uint32]string),
	}
}

// AddNode places `vnodes` virtual copies of nodeID on the ring.
func (r *Ring) AddNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := 0; i < r.vnodes; i++ {
		pos := r.hash(fmt.Sprintf("%s#%d", nodeID, i))
		r.ring[pos] = nodeID
	}
	r.rebuild()
}

// RemoveNode removes all virtual nodes for nodeID.
func (r *Ring) RemoveNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := 0; i < r.vnodes; i++ {
		pos := r.hash(fmt.Sprintf("%s#%d", nodeID, i))
		delete(r.ring, pos)
	}
	r.rebuild()
}

// GetNodes returns the N nodes responsible for key (replication targets).
// N is capped at the number of nodes actually in the ring.
func (r *Ring) GetNodes(key string, n int) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.sorted) == 0 {
		return nil
	}

	pos := r.hash(key)
	idx := r.search(pos) // index of the first ring position >= pos

	seen := make(map[string]bool)
	var nodes []string

	// Walk clockwise around the ring until we have N distinct physical nodes.
	for i := 0; i < len(r.sorted) && len(nodes) < n; i++ {
		vpos := r.sorted[(idx+i)%len(r.sorted)]
		nodeID := r.ring[vpos]
		if !seen[nodeID] {
			seen[nodeID] = true
			nodes = append(nodes, nodeID)
		}
	}
	return nodes
}

// Nodes returns all distinct physical node IDs in the ring.
func (r *Ring) Nodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	var nodes []string
	for _, id := range r.ring {
		if !seen[id] {
			seen[id] = true
			nodes = append(nodes, id)
		}
	}
	sort.Strings(nodes)
	return nodes
}

// NodeCount returns the number of distinct physical nodes.
func (r *Ring) NodeCount() int {
	return len(r.Nodes())
}

// ─── Internal ────────────────────────────────────────────────────────────────

func (r *Ring) hash(s string) uint32 {
	h := sha256.Sum256([]byte(s))
	return binary.BigEndian.Uint32(h[:4])
}

func (r *Ring) rebuild() {
	r.sorted = make([]uint32, 0, len(r.ring))
	for pos := range r.ring {
		r.sorted = append(r.sorted, pos)
	}
	sort.Slice(r.sorted, func(i, j int) bool { return r.sorted[i] < r.sorted[j] })
}

// search returns the index of the first element in sorted >= pos
// (wraps to 0 if all elements are smaller — ring semantics).
func (r *Ring) search(pos uint32) int {
	idx := sort.Search(len(r.sorted), func(i int) bool {
		return r.sorted[i] >= pos
	})
	if idx == len(r.sorted) {
		idx = 0
	}
	return idx
}
