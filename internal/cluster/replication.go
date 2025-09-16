package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Replicator struct {
	node   *Node
	client *http.Client
}

func NewReplicator(node *Node) *Replicator {
	return &Replicator{
		node: node,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (n *Node) replicateWrite(nodeAddr string, req *QuorumRequest) bool {
	return n.replicateWithRetry("PUT", nodeAddr, "/internal/replicate", req, 3)
}

func (n *Node) replicateRead(nodeAddr string, req *QuorumRequest) *QuorumResponse {
	var resp QuorumResponse
	if n.replicateWithRetryAndResponse("GET", nodeAddr, "/internal/replicate/"+req.Key, nil, &resp, 3) {
		return &resp
	}
	return nil
}

func (n *Node) replicateDelete(nodeAddr string, req *QuorumRequest) bool {
	return n.replicateWithRetry("DELETE", nodeAddr, "/internal/replicate"+req.Key, nil, 3)
}

func (n *Node) replicateWithRetry(method, nodeAddr, path string, payload any, maxRetries int) bool {
	var resp QuorumResponse
	return n.replicateWithRetryAndResponse(method, nodeAddr, path, payload, &resp, maxRetries)
}

func (n *Node) replicateWithRetryAndResponse(method, nodeAddr, path string, payload, response any, maxRetries int) bool {
	backoff := time.Millisecond * 100

	for attempt := range maxRetries {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2 // exponential backoff
		}

		if n.makeRequest(method, nodeAddr, path, payload, response) {
			return true
		}
	}
	return false
}

func (n *Node) makeRequest(method, nodeAddr, path string, payload, response any) bool {
	url := fmt.Sprintf("http://%s%s", nodeAddr, path)
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
