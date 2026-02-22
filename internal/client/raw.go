package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// GetRaw performs a raw HTTP GET request to any path
// and returns the full response body as a string.
//
// Big idea:
// Most client methods (Put, Get, Delete) are strongly typed.
// They decode JSON into Go structs.
//
// But sometimes:
//   - The endpoint returns custom JSON
//   - The endpoint returns plain text
//   - The endpoint does not match our typed models
//
// Instead of writing a new typed wrapper,
// we provide a generic "raw" method.
//
// This is flexible and useful for:
//   - /cluster/nodes
//   - debugging endpoints
//   - admin endpoints
//   - future experimental APIs
//
// It keeps the client reusable without needing
// to constantly add new structs.
func (c *Client) GetRaw(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s%s", c.baseURL, path), nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	return string(body), err
}
