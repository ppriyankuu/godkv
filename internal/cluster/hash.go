package cluster

import (
	"crypto/sha1"
	"slices"
	"sort"
	"strconv"
)

// implements a consistent hashing ring
type ConsistentHash struct {
	// maps a hash value (a point on the ring)
	// to a physical node's address
	ring map[uint32]string
	// a sorted slice of all hash values in the ring
	// for efficient lookups using binary search
	sortedKeys []uint32
	// set of unique physical node addresses
	nodes map[string]bool
	// number of virtual nodes to create per physical node
	// for a more uniform key distribution
	replicas int
}

// creates and initializes a new ConsistentHash struct
func NewConsistentHash(replicas int) *ConsistentHash {
	return &ConsistentHash{
		ring:     make(map[uint32]string),
		nodes:    make(map[string]bool),
		replicas: replicas,
	}
}

// adds a physical node to the hash ring. It creates 'replicas' number
// of virtual nodes and places them on the ring
func (c *ConsistentHash) AddNode(node string) {
	c.nodes[node] = true
	// create multiple virtual nodes for this physical node
	for i := 0; i < c.replicas; i++ {
		// a hash for each virtual node
		key := c.hash(node + strconv.Itoa(i))
		c.ring[key] = node
		c.sortedKeys = append(c.sortedKeys, key)
	}
	// re-sort the keys to maintain order for BS
	slices.Sort(c.sortedKeys)
}

// removes a physical node and all its associated virtual nodes from the hash ring
func (c *ConsistentHash) RemoveNode(node string) {
	delete(c.nodes, node)
	// remove all virtual node entries from the main ring map
	for i := 0; i < c.replicas; i++ {
		key := c.hash(node + strconv.Itoa(i))
		delete(c.ring, key)
	}

	// rebuild the sortedkeys slice by filtering out the removed keys
	newKeys := make([]uint32, 0)
	for _, key := range c.sortedKeys {
		// only keep keys that still exist in the ring map
		if _, exists := c.ring[key]; exists {
			newKeys = append(newKeys, key)
		}
	}
	c.sortedKeys = newKeys
}

// finds the 'count' unique physical nodes responsible for a given key
func (c *ConsistentHash) GetNodes(key string, count int) []string {
	if len(c.ring) == 0 {
		return nil
	}

	// hash the data key to find its position on the ring
	hash := c.hash(key)
	// find the index of the first node at or after the key's hash
	idx := c.search(hash)

	nodes := make([]string, 0, count)
	seen := make(map[string]bool)

	// walk the ring clockwise to find 'count' unique nodes
	for i := 0; i < len(c.sortedKeys) && len(nodes) < count; i++ {
		// modulo operator handles wrapping around the end of the slice.
		actualIdx := (idx + 1) % len(c.sortedKeys)
		node := c.ring[c.sortedKeys[actualIdx]]
		// add the node only if it hasn't been added before
		if !seen[node] {
			nodes = append(nodes, node)
			seen[node] = true
		}
	}
	return nodes
}

// computes a SHA1 hash and truncates it to a uint32
func (c *ConsistentHash) hash(key string) uint32 {
	h := sha1.New()
	h.Write([]byte(key))
	hashBytes := h.Sum(nil)
	// use the first 4 bytes of the 20-byte SHA1 hash for the uint32 value
	return uint32(hashBytes[0])<<24 | uint32(hashBytes[1])<<16 |
		uint32(hashBytes[2])<<8 | uint32(hashBytes[3])
}

// performs a binary search on the 'sortedKeys' to find the
// index of the first key that is >= to the provided key
func (c *ConsistentHash) search(key uint32) int {
	// finds the smallest index 'i' where the condition
	// 'c.sortedKeys[i] >= key' is true
	idx := sort.Search(len(c.sortedKeys), func(i int) bool {
		return c.sortedKeys[i] >= key
	})

	// if the key's hash is greater than all node hashes,
	// wrap around to the first nodes
	if idx == len(c.sortedKeys) {
		idx = 0
	}
	return idx
}
