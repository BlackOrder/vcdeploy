// Package testutil provides shared testing utilities for E2E, CLI, and integration tests.
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient provides HTTP client utilities for API testing.
type HTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewHTTPClient creates a new HTTP client for API testing.
func NewHTTPClient(baseURL, token string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetToken sets the authentication token.
func (c *HTTPClient) SetToken(token string) {
	c.token = token
}

// Request makes an HTTP request with optional body and returns the response.
func (c *HTTPClient) Request(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// Get makes a GET request.
func (c *HTTPClient) Get(path string) (*http.Response, error) {
	return c.Request("GET", path, nil)
}

// Post makes a POST request with JSON body.
func (c *HTTPClient) Post(path string, body interface{}) (*http.Response, error) {
	return c.Request("POST", path, body)
}

// Put makes a PUT request with JSON body.
func (c *HTTPClient) Put(path string, body interface{}) (*http.Response, error) {
	return c.Request("PUT", path, body)
}

// Patch makes a PATCH request with JSON body.
func (c *HTTPClient) Patch(path string, body interface{}) (*http.Response, error) {
	return c.Request("PATCH", path, body)
}

// Delete makes a DELETE request.
func (c *HTTPClient) Delete(path string) (*http.Response, error) {
	return c.Request("DELETE", path, nil)
}

// DecodeJSON decodes a JSON response body into the given interface.
func DecodeJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// ReadBody reads and returns the response body as a string.
func ReadBody(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// WaitForEndpoint waits until an HTTP endpoint becomes available.
func WaitForEndpoint(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for endpoint %s", url)
		default:
			req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	return fmt.Errorf("timeout waiting for endpoint %s", url)
}

// WaitForEndpointWithRetry waits for an endpoint with custom retry settings.
func WaitForEndpointWithRetry(url string, timeout, retryInterval time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for endpoint %s", url)
		default:
			req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
			time.Sleep(retryInterval)
		}
	}

	return fmt.Errorf("timeout waiting for endpoint %s", url)
}
