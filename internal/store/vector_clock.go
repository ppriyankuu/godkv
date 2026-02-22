package store

// VectorClock helps us understand "who wrote what and when"
// across multiple distributed nodes.
//
// Problem:
// In distributed systems, two nodes can update the same key at the same time.
// We need a way to detect:
//
//   1) One version is clearly newer → accept it
//   2) One version is clearly older → discard it
//   3) Both were written independently → real conflict
//
// A vector clock solves this.
//
// How it works:
//
// Each key stores a map:
//     nodeID → counter
//
// Every time a node writes:
//     it increments its own counter.
//
// Example:
//
//   Node1 writes:
//     {node1:1}
//
//   Node2 receives it and stores it:
//     {node1:1}
//
//   Node2 writes new value:
//     {node1:1, node2:1}
//
//   Later Node1 receives it.
//   It compares clocks and sees:
//     node2 counter increased → this is newer → accept it
//
// Important idea:
// Vector clocks track "partial ordering".
// We don't force everything into a single global order.
// We only care about causality (what happened before what).

import "maps"

// ClockRelation tells us how two vector clocks relate to each other.
type ClockRelation int

const (
	Before           ClockRelation = iota // This clock is older
	After                                 // This clock is newer
	Equal                                 // Both clocks are exactly the same
	ConcurrentClocks                      // Neither is older — true conflict
)

// VectorClock is a map:
//
//	nodeID → logical counter
//
// Example:
//
//	{
//	    "node1": 3,
//	    "node2": 1
//	}
//
// This means:
//   - node1 updated this key 3 times
//   - node2 updated this key 1 time
type VectorClock map[string]uint64

// Increment increases the counter for a specific node.
//
// This should be called every time the node writes a key.
//
// Example:
//
//	vc.Increment("node1")
//
// If node1 was 2, it becomes 3.
func (vc VectorClock) Increment(nodeID string) {
	vc[nodeID]++
}

// Compare determines how this clock relates to another clock.
//
// It checks:
//
//   - Does vc have any counter strictly greater than other?
//   - Does other have any counter strictly greater than vc?
//
// Possible outcomes:
//
//  1. vc is strictly newer  → After
//  2. vc is strictly older  → Before
//  3. both identical        → Equal
//  4. each has some greater → ConcurrentClocks (real conflict)
//
// Example of conflict:
//
//	vc      = {node1:2}
//	other   = {node2:3}
//
// Neither dominates the other.
// They were updated independently.
// That is a true concurrent conflict.
func (vc VectorClock) Compare(other VectorClock) ClockRelation {
	vcDominates := false    // vc has at least one counter > other
	otherDominates := false // other has at least one counter > vc

	// Check all counters in vc.
	for node, cnt := range vc {
		if cnt > other[node] {
			vcDominates = true
		} else if cnt < other[node] {
			otherDominates = true
		}
	}

	// Check counters that exist only in other.
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

// Merge combines two vector clocks.
//
// For each node, we keep the maximum counter.
//
// This is used when we want to combine two concurrent versions.
//
// Example:
//
//	vc      = {node1:2}
//	other   = {node2:3}
//
// Result:
//
//	{node1:2, node2:3}
//
// Merge does NOT resolve conflicts automatically.
// It only combines version history.
func (vc VectorClock) Merge(other VectorClock) VectorClock {
	merged := vc.Copy()
	for node, cnt := range other {
		if cnt > merged[node] {
			merged[node] = cnt
		}
	}
	return merged
}

// Copy creates a deep copy of the vector clock.
//
// This is important because maps in Go are reference types.
// Without copying, two variables could point to the same map
// and accidentally modify each other.
func (vc VectorClock) Copy() VectorClock {
	c := make(VectorClock, len(vc))
	maps.Copy(c, vc)
	return c
}
