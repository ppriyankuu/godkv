// Package client provides a Go SDK for talking to the distributed KV store.
//
// Big idea:
//
// Instead of writing raw HTTP requests everywhere,
// we wrap them inside a clean Go API.
//
// So instead of:
//
//	http.NewRequest(...)
//	json.Marshal(...)
//
// Users can simply call:
//
//	client.Put(ctx, "key", "value")
//	client.Get(ctx, "key")
//
// This is called a "client library" or "SDK".
//
// It hides:
//   - HTTP details
//   - JSON encoding/decoding
//   - Error handling
//
// And exposes a clean Go interface.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents a connection to ONE KV node.
//
// Important:
//
// This client talks to a single node.
// That node is responsible for:
//   - Coordinating replication
//   - Talking to other nodes
//
// So the client does NOT implement distributed logic.
// It just talks to one node.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a new Client.
//
// baseURL example:
//
//	"http://localhost:8080"
//
// timeout protects us from hanging forever.
// In distributed systems:
//
//	NEVER call network without timeout.
func New(baseURL string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// PutResponse is returned after a successful write.
//
// Why return a clock?
//
// Because this is a distributed system.
// Each write updates a vector clock.
// The client may need that for debugging or conflict handling.
type PutResponse struct {
	Key   string            `json:"key"`
	Value string            `json:"value"`
	Clock map[string]uint64 `json:"clock"`
}

// GetResponse includes:
//
//   - The value
//   - Its vector clock
//   - The last update time
//
// This gives full version information.
type GetResponse struct {
	Key       string            `json:"key"`
	Value     string            `json:"value"`
	Clock     map[string]uint64 `json:"clock"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Put stores key=value in the cluster.
//
// Flow:
//
//  1. Create JSON body
//  2. Build HTTP PUT request
//  3. Send request
//  4. Check status
//  5. Decode response
//
// The distributed logic happens inside the server.
// This client only performs the HTTP call.
func (c *Client) Put(ctx context.Context, key, value string) (*PutResponse, error) {
	body, _ := json.Marshal(map[string]string{"value": value})

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/kv/%s", c.baseURL, key), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PUT request failed: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result PutResponse
	return &result, json.NewDecoder(resp.Body).Decode(&result)
}

// Get retrieves value for key.
//
// Special case:
//
//	If server returns 404
//	We convert it into ErrNotFound
func (c *Client) Get(ctx context.Context, key string) (*GetResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/kv/%s", c.baseURL, key), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result GetResponse
	return &result, json.NewDecoder(resp.Body).Decode(&result)
}

// Delete removes key from cluster.
//
// Internally server may:
//   - Create tombstone
//   - Replicate deletion
//
// Client doesn't care.
// It just sends DELETE request.
func (c *Client) Delete(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/kv/%s", c.baseURL, key), nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE request failed: %w", err)
	}
	defer resp.Body.Close()

	return checkStatus(resp)
}

// JoinCluster registers a node into the cluster.
//
// This triggers:
//   - Membership update
//   - Hash ring update
//   - Key redistribution
func (c *Client) JoinCluster(ctx context.Context, nodeID, address string) error {
	body, _ := json.Marshal(map[string]string{"id": nodeID, "address": address})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/cluster/join", c.baseURL), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp)
}

// LeaveCluster removes a node from the cluster.
func (c *Client) LeaveCluster(ctx context.Context, nodeID string) error {
	body, _ := json.Marshal(map[string]string{"id": nodeID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/cluster/leave", c.baseURL), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp)
}

// ─── Errors ───────────────────────────────────────────────────────────────────

// ErrNotFound is returned when a key does not exist in the store.
var ErrNotFound = fmt.Errorf("key not found")

// APIError carries the HTTP status and the error message from the server.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
}

// checkStatus converts HTTP error responses
// into Go errors.
//
// If status is 2xx → success.
// Otherwise:
//
//  1. Read response body
//  2. Try parsing {"error": "..."} JSON
//  3. Return APIError
func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	var apiErr struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &apiErr)
	msg := apiErr.Error
	if msg == "" {
		msg = string(body)
	}
	return &APIError{Status: resp.StatusCode, Message: msg}
}
