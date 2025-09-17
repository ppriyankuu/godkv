package cluster

import (
	"distributed-kvstore/internal/store"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// holds the configuration for a single node in the cluster
type Config struct {
	NodeID      string   `json:"node_id"`
	Address     string   `json:"address"`
	Peers       []string `json:"peers"`
	Replication int      `json:"replication"`
	ReadQuorum  int      `json:"read_quorum"`
	WriteQuorum int      `json:"write_quorum"`
}

// represents a single node in the kv-store
type Node struct {
	config     *Config
	store      *store.Store
	hash       *ConsistentHash
	peers      map[string]*http.Client
	mu         sync.RWMutex
	replicator *Replicator
}

// the format for reqs sent to other nodes for quorum ops
type QuorumRequest struct {
	Key     string         `json:"key"`
	Value   string         `json:"value,omitempty"`
	Version *store.Version `json:"version,omitempty"`
}

// the format for resps received from other nodes
type QuorumResponse struct {
	Success bool           `json:"success"`
	Value   string         `json:"value,omitempty"`
	Version *store.Version `json:"version,omitempty"`
	Error   string         `json:"error,omitempty"`
}

func NewNode(config *Config, store *store.Store) *Node {
	hash := NewConsistentHash(100) // initializes consistent hashing with 100 virtual nodes
	hash.AddNode(config.NodeID)    // adds the current node to the consistent hash ring

	peers := make(map[string]*http.Client)
	for _, peer := range config.Peers {
		// creates an HTTP client for each peer with a timeout
		peers[peer] = &http.Client{Timeout: 5 * time.Second}
	}

	node := &Node{
		config: config,
		store:  store,
		hash:   hash,
		peers:  peers,
	}

	node.replicator = NewReplicator(node) // initializes the replicator for this node
	return node
}

// handles a client's req to write a new key-value pair
func (n *Node) Put(key, value string) error {
	// finds the nodes responsible for replicating this key
	nodes := n.hash.GetNodes(key, n.config.Replication)

	// creates a new version with a vector clock entry for the current node
	version := &store.Version{
		Clock:     map[string]int64{n.config.NodeID: time.Now().UnixNano()},
		Timestamp: time.Now().UnixNano(),
	}

	req := &QuorumRequest{
		Key:     key,
		Value:   value,
		Version: version,
	}

	// executes the write operation on the quorum of nodes
	return n.executeWriteQuorum(nodes, req)
}

// handles a client's req to read a key's value
func (n *Node) Get(key string) (*store.Entry, error) {
	// finds the nodes that hold replicas of this key
	nodes := n.hash.GetNodes(key, n.config.Replication)

	req := &QuorumRequest{Key: key}
	// executes a read operation on a quorum of nodes
	responses, err := n.executeReadQuorum(nodes, req)
	if err != nil {
		return nil, err
	}

	// read repair; find latest version and repair outdated nodes
	latest := n.findLatestVersion(responses)
	n.readRepair(key, latest, responses)

	return latest, nil
}

// handles a client's req to delete a key-value pair
func (n *Node) Delete(key string) error {
	// finds the nodes responsible for replicating this key
	nodes := n.hash.GetNodes(key, n.config.Replication)

	req := &QuorumRequest{Key: key}
	return n.executeDeleteQuorum(nodes, req)
}

// performs a write op on multple nodes concurrently
func (n *Node) executeWriteQuorum(nodes []string, req *QuorumRequest) error {
	success := 0
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, node := range nodes {
		wg.Add(1)
		go func(nodeAddr string) {
			defer wg.Done()

			if nodeAddr == n.config.Address {
				// writes the key-value pair to the local store
				if err := n.store.Put(req.Key, req.Value, req.Version); err == nil {
					mu.Lock()
					success++
					mu.Unlock()
				}
				return
			}

			// replicates the write op to a remote node
			if n.replicateWrite(nodeAddr, req) {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}(node)
	}

	wg.Wait()

	if success >= n.config.WriteQuorum {
		return nil
	}

	return fmt.Errorf("write quorum not met: %d/%d", success, n.config.WriteQuorum)
}

// performs a read operation on multiple nodes concurrently
func (n *Node) executeReadQuorum(nodes []string, req *QuorumRequest) ([]*QuorumResponse, error) {
	responses := make([]*QuorumResponse, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	// a channel to receive responses from goroutines
	responseChan := make(chan *QuorumResponse, len(nodes))

	for _, node := range nodes {
		wg.Add(1)
		go func(nodeAddr string) {
			defer wg.Done()

			var resp *QuorumResponse
			if nodeAddr == n.config.Address {
				// reads the value from the local store
				if entry, exists := n.store.Get(req.Key); exists {
					resp = &QuorumResponse{
						Success: true,
						Value:   entry.Value,
						Version: &entry.Version,
					}
				} else {
					resp = &QuorumResponse{Success: false}
				}
			} else {
				// replicates the read op to a remote node
				resp = n.replicateRead(nodeAddr, req)
			}

			if resp != nil {
				// sends the resp to the channel
				responseChan <- resp
			}
		}(node)
	}

	// a goroutine to close the channel after all reqs are done
	go func() {
		wg.Wait()
		close(responseChan)
	}()

	// collects all reponses from the channel
	for resp := range responseChan {
		mu.Lock()
		responses = append(responses, resp)
		mu.Unlock()
	}

	if len(responses) >= n.config.ReadQuorum {
		return responses, nil
	}

	return nil, fmt.Errorf("read quorum not met: %d/%d", len(responses), n.config.ReadQuorum)
}

// performs a delete operation on multiple nodes concurrently
func (n *Node) executeDeleteQuorum(nodes []string, req *QuorumRequest) error {
	success := 0
	var wg sync.WaitGroup // waits for all the goroutines to finish
	var mu sync.Mutex     // protects the shared success counter

	for _, node := range nodes {
		wg.Add(1)
		go func(nodeAddr string) {
			defer wg.Done()

			if nodeAddr == n.config.Address {
				// deletes the key from the local store
				if err := n.store.Delete(req.Key); err == nil {
					mu.Lock()
					success++
					mu.Unlock()
				}
				return
			}

			// replicates the delete operation to a remote node
			if n.replicateDelete(nodeAddr, req) {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}(node)
	}

	wg.Wait() // blocks until all delete operations are attempted

	if success >= n.config.WriteQuorum {
		return nil
	}

	return fmt.Errorf("delete quorum not met: %d/%d", success, n.config.WriteQuorum)
}

// iterates through responses to find the one with the most recent timestamp
func (n *Node) findLatestVersion(responses []*QuorumResponse) *store.Entry {
	var latest *store.Entry

	for _, resp := range responses {
		// skip unsuccessful resps or those with no version info
		if !resp.Success || resp.Version == nil {
			continue
		}

		entry := &store.Entry{
			Value:   resp.Value,
			Version: *resp.Version,
		}

		// compare timestamps to find the latest version
		if latest == nil || entry.Version.Timestamp > latest.Version.Timestamp {
			latest = entry
		}
	}
	return latest
}

// asyncly repairs outdated replicas with the latest version
func (n *Node) readRepair(key string, latest *store.Entry, responses []*QuorumResponse) {
	if latest == nil {
		return
	}

	// starts a new goroutine for repair process to avoid blocking
	go func() {
		for _, resp := range responses {
			// skip unsuccessful or up-to-date responses
			if !resp.Success || resp.Version == nil {
				continue
			}

			// check if a replica is outdated
			if resp.Version.Timestamp < latest.Version.Timestamp {
				// this node needs repair
				req := &QuorumRequest{
					Key:     key,
					Value:   latest.Value,
					Version: &latest.Version,
				}

				// find the specific node that needs repair and send the updated data
				// this section simplifies the logic by sending all the nodes again.
				// a more refined implementation would target only the outdated ones
				nodes := n.hash.GetNodes(key, n.config.Replication)
				for _, node := range nodes {
					if node != n.config.Address {
						n.replicateWrite(node, req)
					}
				}
			}
		}
	}()
}
