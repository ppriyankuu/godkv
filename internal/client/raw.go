package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// GetRaw performs a raw GET to path and returns the response body as a string.
// Useful for endpoints like /cluster/nodes that don't fit the typed API.
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
