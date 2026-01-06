package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// responsible for communicating with other nodes to replicate data
type Replicator struct {
	node   *Node
	client *http.Client
}

// to createa a new Replicator instance
func NewReplicator(node *Node) *Replicator {
	return &Replicator{
		node: node,
		client: &http.Client{
			Timeout: 10 * time.Second, // timeout for HTTP reqs to prevent them from hanging
		},
	}
}

// sends a PUT req to another node to replicate a write operation
func (n *Node) replicateWrite(nodeAddr string, req *QuorumRequest) bool {
	// calls the generic retry function with specific parameters for write operation
	return n.replicateWithRetry("PUT", nodeAddr, "/internal/replicate", req, 3)
}

// sends a GET req to another node to replicate a read operation
func (n *Node) replicateRead(nodeAddr string, req *QuorumRequest) *QuorumResponse {
	var resp QuorumResponse
	// calls the generic retry func that also handles the response
	if n.replicateWithRetryAndResponse("GET", nodeAddr, "/internal/replicate/"+req.Key, nil, &resp, 3) {
		return &resp
	}
	return nil
}

// sends a DELETE req to another node to replicate a delete operation
func (n *Node) replicateDelete(nodeAddr string, req *QuorumRequest) bool {
	// calls the generic retry func
	return n.replicateWithRetry("DELETE", nodeAddr, "/internal/replicate"+req.Key, nil, 3)
}

// a helper function that retries a req a specified no. of times
func (n *Node) replicateWithRetry(method, nodeAddr, path string, payload any, maxRetries int) bool {
	var resp QuorumResponse
	// serves as a wrapper for more comprehensive retry func
	return n.replicateWithRetryAndResponse(method, nodeAddr, path, payload, &resp, maxRetries)
}

// the core func for making a req with retries and an exponential backoff
func (n *Node) replicateWithRetryAndResponse(method, nodeAddr, path string, payload, response any, maxRetries int) bool {
	backoff := time.Millisecond * 100 // initial backoff duration

	for attempt := range maxRetries { // loops for max no. of retries
		if attempt > 0 {
			time.Sleep(backoff) // waits before retrying
			backoff *= 2        // increases the backoff duration for the next attempt (exponential backoff)
		}

		if n.makeRequest(method, nodeAddr, path, payload, response) {
			return true // true for successful request
		}
	}
	return false // false if all retries fail
}

// performs a single HTTP req to another node
func (n *Node) makeRequest(method, nodeAddr, path string, payload, response any) bool {
	url := fmt.Sprintf("http://%s%s", nodeAddr, path) // constructs a full URL
	var reqBody []byte
	if payload != nil {
		var err error
		reqBody, err = json.Marshal(payload)
		if err != nil {
			return false
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(reqBody))
	if err != nil {
		return false
	}

	req.Header.Set("Content-Type", "application/json")

	client := n.peers[nodeAddr]
	if client == nil {
		client = n.replicator.client
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	if response != nil {
		return json.NewDecoder(resp.Body).Decode(response) == nil
	}

	return true
}
