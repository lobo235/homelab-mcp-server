// Package adguard provides an HTTP client wrapper for the adguard-home-gateway API.
package adguard

import (
	"context"
	"fmt"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

// Rewrite represents an AdGuard DNS rewrite rule.
type Rewrite struct {
	Domain string `json:"domain"`
	Answer string `json:"answer"`
}

// Client wraps the adguard-home-gateway HTTP API.
type Client struct {
	base *clients.Base
}

// NewClient creates a new adguard-home-gateway client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{base: clients.NewBase(baseURL, apiKey)}
}

// Ping checks gateway reachability.
func (c *Client) Ping(ctx context.Context) error {
	return c.base.Ping(ctx)
}

// ListRewrites returns all DNS rewrites.
func (c *Client) ListRewrites(ctx context.Context) ([]Rewrite, error) {
	var rewrites []Rewrite
	if err := c.base.DoJSON(ctx, "GET", "/rewrites", nil, &rewrites); err != nil {
		return nil, fmt.Errorf("list rewrites: %w", err)
	}
	return rewrites, nil
}

// CreateRewrite creates a new DNS rewrite rule.
func (c *Client) CreateRewrite(ctx context.Context, domain, answer string) error {
	req := Rewrite{Domain: domain, Answer: answer}
	if err := c.base.DoNoContent(ctx, "POST", "/rewrites", req); err != nil {
		return fmt.Errorf("create rewrite %q: %w", domain, err)
	}
	return nil
}

// DeleteRewrite deletes a DNS rewrite by domain.
func (c *Client) DeleteRewrite(ctx context.Context, domain string) error {
	if err := c.base.DoNoContent(ctx, "DELETE", "/rewrites/"+domain, nil); err != nil {
		return fmt.Errorf("delete rewrite %q: %w", domain, err)
	}
	return nil
}
