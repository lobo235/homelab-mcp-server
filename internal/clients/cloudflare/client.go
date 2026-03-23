// Package cloudflare provides an HTTP client wrapper for the cloudflare-gateway API.
package cloudflare

import (
	"context"
	"fmt"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

// DNSRecord represents a Cloudflare DNS record.
type DNSRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied *bool  `json:"proxied,omitempty"`
}

// Client wraps the cloudflare-gateway HTTP API.
type Client struct {
	base *clients.Base
}

// NewClient creates a new cloudflare-gateway client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{base: clients.NewBase(baseURL, apiKey)}
}

// Ping checks gateway reachability.
func (c *Client) Ping(ctx context.Context) error {
	return c.base.Ping(ctx)
}

// CreateRecordByZoneName creates a DNS record using the zone name convenience route.
func (c *Client) CreateRecordByZoneName(ctx context.Context, zoneName string, rec DNSRecord) (*DNSRecord, error) {
	var created DNSRecord
	path := fmt.Sprintf("/zones-by-name/%s/records", zoneName)
	if err := c.base.DoJSON(ctx, "POST", path, rec, &created); err != nil {
		return nil, fmt.Errorf("create DNS record in zone %q: %w", zoneName, err)
	}
	return &created, nil
}

// DeleteRecordByZoneName deletes a DNS record by record name using the zone name convenience route.
func (c *Client) DeleteRecordByZoneName(ctx context.Context, zoneName, recordName string) error {
	path := fmt.Sprintf("/zones-by-name/%s/records/%s", zoneName, recordName)
	if err := c.base.DoNoContent(ctx, "DELETE", path, nil); err != nil {
		return fmt.Errorf("delete DNS record %q in zone %q: %w", recordName, zoneName, err)
	}
	return nil
}

// ListRecordsByZoneName lists DNS records in a zone by zone name.
func (c *Client) ListRecordsByZoneName(ctx context.Context, zoneName string) ([]DNSRecord, error) {
	var records []DNSRecord
	path := fmt.Sprintf("/zones-by-name/%s/records", zoneName)
	if err := c.base.DoJSON(ctx, "GET", path, nil, &records); err != nil {
		return nil, fmt.Errorf("list DNS records in zone %q: %w", zoneName, err)
	}
	return records, nil
}

// CreateRecord creates a DNS record in a zone by zone ID.
func (c *Client) CreateRecord(ctx context.Context, zoneID string, rec DNSRecord) (*DNSRecord, error) {
	var created DNSRecord
	path := fmt.Sprintf("/zones/%s/records", zoneID)
	if err := c.base.DoJSON(ctx, "POST", path, rec, &created); err != nil {
		return nil, fmt.Errorf("create DNS record in zone %q: %w", zoneID, err)
	}
	return &created, nil
}

// DeleteRecord deletes a DNS record by zone ID and record ID.
func (c *Client) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	path := fmt.Sprintf("/zones/%s/records/%s", zoneID, recordID)
	if err := c.base.DoNoContent(ctx, "DELETE", path, nil); err != nil {
		return fmt.Errorf("delete DNS record %q: %w", recordID, err)
	}
	return nil
}
