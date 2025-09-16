package cluster

import (
	"distributed-kvstore/internal/store"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Config struct {
	NodeID      string   `json:"node_id"`
	Address     string   `json:"address"`
	Peers       []string `json:"peers"`
	Replication int      `json:"replication"`
	ReadQuorum  int      `json:"read_quorum"`
	WriteQuorum int      `json:"write_quorum"`
}

type Node struct {
	config     *Config
	store      *store.Store
	hash       *ConsistentHash
	peers      map[string]*http.Client
	mu         sync.RWMutex
	replicator *Replicator
}

type QuorumRequest struct {
	Key     string         `json:"key"`
	Value   string         `json:"value,omitempty"`
	Version *store.Version `json:"version,omitempty"`
}

type QuorumResponse struct {
	Success bool           `json:"success"`
	Value   string         `json:"value,omitempty"`
	Version *store.Version `json:"version,omitempty"`
	Error   string         `json:"error,omitempty"`
}

func NewNode(config *Config, store *store.Store) *Node {
	hash := NewConsistentHash(100)
	hash.AddNode(config.NodeID)

	peers := make(map[string]*http.Client)
	for _, peer := range config.Peers {
		peers[peer] = &http.Client{Timeout: 5 * time.Second}
	}

	node := &Node{
		config: config,
		store:  store,
		hash:   hash,
		peers:  peers,
	}

	node.replicator = NewReplicator(node)
	return node
}

func (n *Node) Put(key, value string) error {
	nodes := n.hash.GetNodes(key, n.config.Replication)

	version := &store.Version{
		Clock:     map[string]int64{n.config.NodeID: time.Now().UnixNano()},
		Timestamp: time.Now().UnixNano(),
	}

	req := &QuorumRequest{
		Key:     key,
		Value:   value,
		Version: version,
	}

	return n.executeWriteQuorum(nodes, req)
}

func (n *Node) Get(key string) (*store.Entry, error) {
	nodes := n.hash.GetNodes(key, n.config.Replication)

	req := &QuorumRequest{Key: key}
	responses, err := n.executeReadQuorum(nodes, req)
	if err != nil {
		return nil, err
	}

	// read repair; find latest version and repair outdated nodes

	// latest:= n.findLatestVersion(responses)
	// n.readrepai
}

func (n *Node) executeWriteQuorum(nodes []string, req *QuorumRequest) error {
	success := 0
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, node := range nodes {
		wg.Add(1)
		go func(nodeAddr string) {
			defer wg.Done()

			if nodeAddr == n.config.Address {
				// local write
				if err := n.store.Put(req.Key, req.Value, req.Version); err == nil {
					mu.Lock()
					success++
					mu.Unlock()
				}
				return
			}

			// remote write
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

func (n *Node) executeReadQuorum(nodes []string, req *QuorumRequest) ([]*QuorumResponse, error) {
	responses := make([]*QuorumResponse, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	responseChan := make(chan *QuorumResponse, len(nodes))

	for _, node := range nodes {
		wg.Add(1)
		go func(nodeAddr string) {
			defer wg.Done()

			var resp *QuorumResponse
			if nodeAddr == n.config.Address {
				// local read
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
				// remote read
				resp = n.replicateRead(nodeAddr, req)
			}

			if resp != nil {
				responseChan <- resp
			}
		}(node)
	}

	go func() {
		wg.Wait()
		close(responseChan)
	}()

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
