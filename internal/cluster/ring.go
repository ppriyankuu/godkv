// Package cluster handles all distributed logic:
//
//   - Consistent hashing (who owns which key?)
//   - Node membership (adding/removing nodes)
//   - Replication target selection
//
// Big idea:
//
// In a distributed key-value store, we must decide:
//
//	"Which node is responsible for this key?"
//
// This file implements consistent hashing to solve that problem
// efficiently and safely.
package cluster

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
	"sort"
	"sync"
)

////////////////////////////////////////////////////////////////////////////////
// CONSISTENT HASHING
////////////////////////////////////////////////////////////////////////////////

// Why not just use:  hash(key) % N ?
//
// Because if a node is added or removed:
//     → Almost ALL keys get remapped.
//     → Massive data movement.
//     → System instability.
//
// Instead, we use consistent hashing.
//
// Core idea:
//
// 1) Imagine a circle (a ring) of numbers from 0 → 2^32.
// 2) Each node is placed on this ring using a hash.
// 3) Each key is also placed on this ring using a hash.
// 4) A key belongs to the first node clockwise from its position.
//
// If a node is added or removed:
//     → Only nearby keys move.
//     → On average, only 1/N of keys are affected.
//     → Much more stable.
//
// This is what real systems like Cassandra and Dynamo use.

// Virtual nodes:
//
// If we put only 1 position per physical node,
// load can become uneven.
//
// So we create many "virtual nodes" per physical node.
// Each physical node appears multiple times on the ring.
// This spreads its ownership more evenly.
//
// Typical range: 100–200 virtual nodes per physical node.
const defaultVnodes = 150

////////////////////////////////////////////////////////////////////////////////
// RING STRUCTURE
////////////////////////////////////////////////////////////////////////////////

// Ring represents the consistent hash ring.
//
// It is safe for concurrent use.
//
// Fields:
//
//	mu     → protects all ring state
//	vnodes → number of virtual nodes per physical node
//	ring   → maps ring position → nodeID
//	sorted → sorted list of positions (for binary search)
//
// Why do we store `sorted`?
//
// Because we need fast lookup of:
//
//	"first position >= keyHash"
//
// We use binary search on this sorted slice.
type Ring struct {
	mu     sync.RWMutex
	vnodes int
	ring   map[uint32]string
	sorted []uint32
}

////////////////////////////////////////////////////////////////////////////////
// CONSTRUCTOR
////////////////////////////////////////////////////////////////////////////////

// NewRing creates an empty hash ring.
//
// If vnodes <= 0, we use a sensible default.
// More vnodes → better load balance (but slightly more memory).
func NewRing(vnodes int) *Ring {
	if vnodes <= 0 {
		vnodes = defaultVnodes
	}
	return &Ring{
		vnodes: vnodes,
		ring:   make(map[uint32]string),
	}
}

////////////////////////////////////////////////////////////////////////////////
// NODE MANAGEMENT
////////////////////////////////////////////////////////////////////////////////

// AddNode adds a physical node to the ring.
//
// Steps:
//  1. Lock (write lock)
//  2. For i = 0 → vnodes
//  3. Hash "nodeID#i" to generate virtual position
//  4. Insert into ring map
//  5. Rebuild sorted positions
//
// Why "nodeID#i"?
//
// So each virtual node hashes to a different position.
func (r *Ring) AddNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := 0; i < r.vnodes; i++ {
		pos := r.hash(fmt.Sprintf("%s#%d", nodeID, i))
		r.ring[pos] = nodeID
	}
	r.rebuild()
}

// RemoveNode removes a physical node.
//
// We must remove ALL its virtual nodes.
// Then rebuild the sorted slice.
func (r *Ring) RemoveNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := 0; i < r.vnodes; i++ {
		pos := r.hash(fmt.Sprintf("%s#%d", nodeID, i))
		delete(r.ring, pos)
	}
	r.rebuild()
}

////////////////////////////////////////////////////////////////////////////////
// KEY LOOKUP (REPLICATION LOGIC)
////////////////////////////////////////////////////////////////////////////////

// GetNodes returns the N distinct physical nodes
// responsible for a given key.
//
// This supports replication.
//
// Example:
//
//	If replication factor = 3,
//	we return 3 distinct nodes clockwise on the ring.
//
// Steps:
//
//  1. Read lock
//  2. Hash the key
//  3. Find first ring position >= keyHash
//  4. Walk clockwise collecting distinct nodeIDs
//  5. Stop once we have N unique nodes
//
// Important:
// Multiple virtual nodes may belong to the same physical node.
// We must ensure we only return distinct physical nodes.
func (r *Ring) GetNodes(key string, n int) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.sorted) == 0 {
		return nil
	}

	pos := r.hash(key)
	idx := r.search(pos)

	seen := make(map[string]bool)
	var nodes []string

	// Walk clockwise around the ring.
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

////////////////////////////////////////////////////////////////////////////////
// INTROSPECTION HELPERS
////////////////////////////////////////////////////////////////////////////////

// Nodes returns all distinct physical nodes.
//
// Useful for debugging or monitoring.
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

// NodeCount returns how many physical nodes exist.
//
// Note: This is NOT number of virtual nodes.
func (r *Ring) NodeCount() int {
	return len(r.Nodes())
}

////////////////////////////////////////////////////////////////////////////////
// INTERNAL HELPERS
////////////////////////////////////////////////////////////////////////////////

// hash converts a string into a 32-bit ring position.
//
// Why sha256?
//
// We want:
//   - Even distribution
//   - Low collision probability
//
// We only use the first 4 bytes (32 bits)
// because our ring is 2^32 in size.
func (r *Ring) hash(s string) uint32 {
	h := sha256.Sum256([]byte(s))
	return binary.BigEndian.Uint32(h[:4])
}

// rebuild reconstructs the sorted slice of ring positions.
//
// We must call this after:
//   - Adding a node
//   - Removing a node
//
// Why?
// Because binary search requires sorted data.
func (r *Ring) rebuild() {
	r.sorted = make([]uint32, 0, len(r.ring))
	for pos := range r.ring {
		r.sorted = append(r.sorted, pos)
	}
	slices.Sort(r.sorted)
}

// search finds the index of the first ring position >= pos.
//
// If all positions are smaller,
// we wrap around to index 0.
//
// This gives us circular (ring) behavior.
func (r *Ring) search(pos uint32) int {
	idx := sort.Search(len(r.sorted), func(i int) bool {
		return r.sorted[i] >= pos
	})

	// Wrap-around case.
	if idx == len(r.sorted) {
		idx = 0
	}
	return idx
}
