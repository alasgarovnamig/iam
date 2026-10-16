// Package opa implements the PolicyDecider port against an OPA sidecar's
// REST API. Errors are surfaced to the caller which MUST fail closed.
package opa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 3 * time.Second}}
}

func (c *Client) Decide(ctx context.Context, in ports.PolicyInput) (*ports.PolicyDecision, error) {
	body, err := json.Marshal(map[string]any{"input": in})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/data/wallet/authz/decision", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opa decide: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("opa decide: status %d", resp.StatusCode)
	}
	var out struct {
		Result *ports.PolicyDecision `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Result == nil {
		// Policy missing => deny by default.
		return nil, fmt.Errorf("opa decide: empty result (policy not loaded)")
	}
	return out.Result, nil
}

func (c *Client) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("opa unhealthy: status %d", resp.StatusCode)
	}
	return nil
}
