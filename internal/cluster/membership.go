package cluster

import (
	"fmt"
	"sync"
)

// Node represents a single cluster member.
type Node struct {
	ID      string `json:"id"`
	Address string `json:"address"` // host:port
	IsAlive bool   `json:"is_alive"`
}

// Membership tracks which nodes are in the cluster.
// In production you would replace this with a gossip protocol (e.g. SWIM/Serf),
// but static membership is the right starting point.
type Membership struct {
	mu    sync.RWMutex
	nodes map[string]*Node // nodeID → Node
	ring  *Ring
}

// NewMembership creates membership seeded with the provided node list.
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

// Join adds a new node to the cluster.
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

// Leave removes a node from the cluster (graceful departure).
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

// GetNode returns the Node for a given ID.
func (m *Membership) GetNode(id string) (*Node, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, ok := m.nodes[id]
	return n, ok
}

// All returns a copy of all current nodes.
func (m *Membership) All() []Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		out = append(out, *n)
	}
	return out
}

// Ring exposes the consistent-hash ring for key routing.
func (m *Membership) Ring() *Ring {
	return m.ring
}

// ReplicaNodes returns the node IDs responsible for key with replication factor n.
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
