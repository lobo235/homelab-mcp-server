// Package vault provides an HTTP client wrapper for the vault-gateway API.
package vault

import (
	"context"
	"fmt"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

// Secret represents a Minecraft server secret.
type Secret struct {
	RCONPassword string `json:"rcon_password,omitempty"`
}

// Client wraps the vault-gateway HTTP API.
type Client struct {
	base *clients.Base
}

// NewClient creates a new vault-gateway client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{base: clients.NewBase(baseURL, apiKey)}
}

// NewClientWithBase creates a client using a pre-configured Base (with retry/circuit breaker).
func NewClientWithBase(base *clients.Base) *Client {
	return &Client{base: base}
}

// Ping checks gateway reachability.
func (c *Client) Ping(ctx context.Context) error {
	return c.base.Ping(ctx)
}

// CreateSecret creates secrets for a Minecraft server (auto-generates RCON password).
func (c *Client) CreateSecret(ctx context.Context, serverName string) error {
	if err := c.base.DoNoContent(ctx, "POST", "/secrets/minecraft/"+serverName, nil); err != nil {
		return fmt.Errorf("create secret for %q: %w", serverName, err)
	}
	return nil
}

// GetSecret reads secrets for a Minecraft server.
func (c *Client) GetSecret(ctx context.Context, serverName string) (*Secret, error) {
	var secret Secret
	if err := c.base.DoJSON(ctx, "GET", "/secrets/minecraft/"+serverName, nil, &secret); err != nil {
		return nil, fmt.Errorf("get secret for %q: %w", serverName, err)
	}
	return &secret, nil
}

// RotateSecret rotates the RCON password for a Minecraft server.
func (c *Client) RotateSecret(ctx context.Context, serverName string) error {
	if err := c.base.DoNoContent(ctx, "PUT", "/secrets/minecraft/"+serverName, nil); err != nil {
		return fmt.Errorf("rotate secret for %q: %w", serverName, err)
	}
	return nil
}

// DeleteSecret deletes all secret versions for a Minecraft server.
func (c *Client) DeleteSecret(ctx context.Context, serverName string) error {
	if err := c.base.DoNoContent(ctx, "DELETE", "/secrets/minecraft/"+serverName, nil); err != nil {
		return fmt.Errorf("delete secret for %q: %w", serverName, err)
	}
	return nil
}
