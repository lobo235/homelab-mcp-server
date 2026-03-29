// Package minecraft provides an HTTP client wrapper for the minecraft-gateway RCON API.
package minecraft

import (
	"context"
	"fmt"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

// RCONRequest represents an RCON command request.
type RCONRequest struct {
	Command string `json:"command"`
}

// RCONResponse represents the response from an RCON command.
type RCONResponse struct {
	Response string `json:"response"`
}

// Client wraps the minecraft-gateway HTTP API.
type Client struct {
	base *clients.Base
}

// NewClient creates a new minecraft-gateway client.
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

// ExecuteRCON sends an RCON command to a server.
func (c *Client) ExecuteRCON(ctx context.Context, name, command string) (string, error) {
	req := RCONRequest{Command: command}
	var resp RCONResponse
	if err := c.base.DoJSON(ctx, "POST", "/servers/"+name+"/rcon", req, &resp); err != nil {
		return "", fmt.Errorf("rcon %q: %w", name, err)
	}
	return resp.Response, nil
}
