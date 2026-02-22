package store

// VectorClock tracks causality across distributed nodes.
//
// Interview explanation:
//   A vector clock is a map from nodeID → logical counter.  Every time node N
//   writes a key, it increments its own counter.  When we compare two versions
//   of the same key from different nodes we can determine:
//     - A happened-before B  → A is stale, discard it
//     - B happened-before A  → B is stale, discard it
//     - A and B are concurrent → real conflict, need resolution policy
//
//   Example timeline:
//     Node1 writes:  {node1:1}
//     Node2 gets it: {node1:1}         (received via replication)
//     Node2 writes:  {node1:1, node2:1} (increments own counter)
//     Node1 gets it: sees node2:1 > 0 for that key → accepts the update
//
//   This is cheaper than Lamport timestamps (scalar) because it captures
//   partial ordering rather than total ordering.

// ClockRelation represents the causal relationship between two vector clocks.
type ClockRelation int

const (
	Before           ClockRelation = iota // self happened-before other
	After                                 // self happened-after other
	Equal                                 // identical
	ConcurrentClocks                      // concurrent — true conflict
)

// VectorClock maps node IDs to their logical counters.
type VectorClock map[string]uint64

// Increment bumps the counter for nodeID.
func (vc VectorClock) Increment(nodeID string) {
	vc[nodeID]++
}

// Compare returns the causal relationship of vc relative to other.
func (vc VectorClock) Compare(other VectorClock) ClockRelation {
	vcDominates := false    // vc has at least one counter > other
	otherDominates := false // other has at least one counter > vc

	// Check all keys in vc.
	for node, cnt := range vc {
		if cnt > other[node] {
			vcDominates = true
		} else if cnt < other[node] {
			otherDominates = true
		}
	}
	// Check keys that exist only in other.
	for node, cnt := range other {
		if _, ok := vc[node]; !ok && cnt > 0 {
			otherDominates = true
		}
	}

	switch {
	case !vcDominates && !otherDominates:
		return Equal
	case vcDominates && !otherDominates:
		return After
	case !vcDominates && otherDominates:
		return Before
	default:
		return ConcurrentClocks
	}
}

// Merge returns a new VectorClock that takes the max of each counter.
// Used when merging concurrent versions.
func (vc VectorClock) Merge(other VectorClock) VectorClock {
	merged := vc.Copy()
	for node, cnt := range other {
		if cnt > merged[node] {
			merged[node] = cnt
		}
	}
	return merged
}

// Copy returns a deep copy.
func (vc VectorClock) Copy() VectorClock {
	c := make(VectorClock, len(vc))
	for k, v := range vc {
		c[k] = v
	}
	return c
}
