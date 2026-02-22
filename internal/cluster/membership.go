package cluster

import (
	"fmt"
	"sync"
)

////////////////////////////////////////////////////////////////////////////////
// NODE
////////////////////////////////////////////////////////////////////////////////

// Node represents one member in the cluster.
//
// Fields:
//
//	ID        → unique identifier (used in hashing ring)
//	Address   → host:port for HTTP communication
//	IsAlive   → simple liveness flag
//
// In a real production system, liveness would be managed
// automatically using heartbeats or a gossip protocol.
type Node struct {
	ID      string `json:"id"`
	Address string `json:"address"` // host:port
	IsAlive bool   `json:"is_alive"`
}

////////////////////////////////////////////////////////////////////////////////
// MEMBERSHIP
////////////////////////////////////////////////////////////////////////////////

// Membership keeps track of:
//
//   - Which nodes exist in the cluster
//   - Which nodes are alive
//   - The consistent-hash ring
//
// Important:
//
// This implementation uses static membership.
// In production systems, this would typically be replaced by:
//
//   - Gossip protocol (SWIM, Serf)
//   - Service discovery (Consul, etcd)
//   - Kubernetes service registry
//
// Thread safety:
//   - RWMutex protects node map
//   - Ring has its own internal lock
type Membership struct {
	mu    sync.RWMutex
	nodes map[string]*Node // nodeID → Node
	ring  *Ring
}

////////////////////////////////////////////////////////////////////////////////
// CONSTRUCTOR
////////////////////////////////////////////////////////////////////////////////

// NewMembership creates a membership instance
// seeded with an initial list of nodes.
//
// Steps:
//
//  1. Create empty node map
//  2. Create consistent hash ring
//  3. Mark each node as alive
//  4. Add each node to the ring
//
// After this completes:
//   - The cluster is ready to route keys.
func NewMembership(nodes []Node, vnodes int) *Membership {
	m := &Membership{
		nodes: make(map[string]*Node),
		ring:  NewRing(vnodes),
	}

	for i := range nodes {
		n := nodes[i]
		n.IsAlive = true
		m.nodes[n.ID] = &n
		m.ring.AddNode(n.ID)
	}

	return m
}

////////////////////////////////////////////////////////////////////////////////
// JOIN / LEAVE
////////////////////////////////////////////////////////////////////////////////

// Join adds a new node to the cluster.
//
// Steps:
//
//  1. Acquire write lock
//  2. Ensure node is not already present
//  3. Mark node alive
//  4. Add to node map
//  5. Add to consistent hash ring
//
// Adding to the ring will cause
// some keys to move to the new node.
//
// Thanks to consistent hashing,
// only ~1/N of keys move.
func (m *Membership) Join(node Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.nodes[node.ID]; ok {
		return fmt.Errorf("node %s already in cluster", node.ID)
	}

	node.IsAlive = true
	m.nodes[node.ID] = &node
	m.ring.AddNode(node.ID)

	return nil
}

// Leave removes a node from the cluster.
//
// This represents a graceful shutdown.
//
// Steps:
//
//  1. Acquire write lock
//  2. Ensure node exists
//  3. Remove from node map
//  4. Remove from consistent hash ring
//
// Removing a node causes its key range
// to be reassigned to the next nodes
// clockwise on the ring.
func (m *Membership) Leave(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.nodes[nodeID]; !ok {
		return fmt.Errorf("node %s not in cluster", nodeID)
	}

	delete(m.nodes, nodeID)
	m.ring.RemoveNode(nodeID)

	return nil
}

////////////////////////////////////////////////////////////////////////////////
// LOOKUPS
////////////////////////////////////////////////////////////////////////////////

// GetNode returns a specific node by ID.
//
// Returns:
//   - pointer to Node
//   - boolean indicating existence
//
// Read lock allows concurrent readers.
func (m *Membership) GetNode(id string) (*Node, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	n, ok := m.nodes[id]
	return n, ok
}

// All returns a copy of all current nodes.
//
// Why return a copy?
//
// So callers cannot accidentally modify
// internal cluster state.
//
// This is a common defensive programming pattern.
func (m *Membership) All() []Node {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		out = append(out, *n)
	}
	return out
}

////////////////////////////////////////////////////////////////////////////////
// RING ACCESS
////////////////////////////////////////////////////////////////////////////////

// Ring exposes the underlying consistent-hash ring.
//
// Used by higher layers (like Replicator)
// to determine which nodes own a key.
func (m *Membership) Ring() *Ring {
	return m.ring
}

////////////////////////////////////////////////////////////////////////////////
// KEY ROUTING
////////////////////////////////////////////////////////////////////////////////

// ReplicaNodes returns the nodes responsible for a given key.
//
// Steps:
//
//  1. Ask consistent hash ring for N node IDs
//  2. Convert IDs into Node pointers
//  3. Return the list
//
// The ring determines ownership.
// Membership provides the actual Node objects.
//
// This separation keeps responsibilities clean:
//   - Ring = routing logic
//   - Membership = cluster state
func (m *Membership) ReplicaNodes(key string, n int) []*Node {

	ids := m.ring.GetNodes(key, n)

	m.mu.RLock()
	defer m.mu.RUnlock()

	var nodes []*Node

	for _, id := range ids {
		if node, ok := m.nodes[id]; ok {
			nodes = append(nodes, node)
		}
	}

	return nodes
}
