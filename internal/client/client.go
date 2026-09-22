// Package client is an HTTP client for nodexa-registry's release API
// (/v1/fleets/{fleet_id}/releases). Used by `nodex push` to reserve a
// release, then complete or fail it after building/pushing images.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to nodexa-registry's FastAPI service.
type Client struct {
	baseURL    string // e.g. "http://localhost:8000"
	apiToken   string // nodexa-user-service API token, sent as Bearer
	httpClient *http.Client
}

// New creates a Client targeting the given registry base URL.
func New(baseURL, apiToken string) *Client {
	return &Client{
		baseURL:  baseURL,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ServiceOut is a single service within a release response.
type ServiceOut struct {
	Name     string  `json:"name"`
	ImageRef string  `json:"image_ref"`
	Digest   *string `json:"digest"`
}

// ReleaseOut is the response from reserve/complete/fail/list endpoints.
type ReleaseOut struct {
	FleetID     string       `json:"fleet_id"`
	Revision    int          `json:"revision"`
	Status      string       `json:"status"`
	PushedBy    string       `json:"pushed_by"`
	Services    []ServiceOut `json:"services"`
	CreatedAt   string       `json:"created_at"`
	CompletedAt *string      `json:"completed_at"`
}

// ReserveRelease creates a new pending release for the given fleet, returning
// the assigned revision number and pre-computed image refs for each service.
func (c *Client) ReserveRelease(fleetID string, serviceNames []string) (*ReleaseOut, error) {
	body := map[string]any{
		"service_names": serviceNames,
	}
	return c.doJSON(http.MethodPost, c.releasesURL(fleetID), body)
}

// CompleteRelease marks a pending release as complete, recording digests.
func (c *Client) CompleteRelease(fleetID string, revision int, digests map[string]string) (*ReleaseOut, error) {
	body := map[string]any{
		"digests": digests,
	}
	return c.doJSON(http.MethodPatch, fmt.Sprintf("%s/%d/complete", c.releasesURL(fleetID), revision), body)
}

// FailRelease marks a pending release as failed.
func (c *Client) FailRelease(fleetID string, revision int) (*ReleaseOut, error) {
	return c.doJSON(http.MethodPatch, fmt.Sprintf("%s/%d/fail", c.releasesURL(fleetID), revision), nil)
}

func (c *Client) releasesURL(fleetID string) string {
	return fmt.Sprintf("%s/v1/fleets/%s/releases", c.baseURL, fleetID)
}

func (c *Client) doJSON(method, url string, body any) (*ReleaseOut, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d from %s %s: %s", resp.StatusCode, method, url, string(respBody))
	}

	var out ReleaseOut
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &out, nil
}
