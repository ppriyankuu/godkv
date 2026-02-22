// Package client provides a Go library for interacting with the KV store.
// It is used by both the CLI and can be imported by other Go services.
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

// Client talks to a single node of the KV store.  For production use you
// would add a retry layer that automatically re-routes to a healthy node.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a Client pointed at baseURL (e.g. "http://localhost:8080").
func New(baseURL string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// PutResponse is returned from Put operations.
type PutResponse struct {
	Key   string            `json:"key"`
	Value string            `json:"value"`
	Clock map[string]uint64 `json:"clock"`
}

// GetResponse is returned from Get operations.
type GetResponse struct {
	Key       string            `json:"key"`
	Value     string            `json:"value"`
	Clock     map[string]uint64 `json:"clock"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Put stores key=value.  Returns the stored value with its vector clock.
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

// Get retrieves the value for key.  Returns ErrNotFound if the key does not exist.
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

// Delete removes key from the store.
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

// JoinCluster registers a new node with the cluster.
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
