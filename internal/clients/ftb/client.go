// Package ftb provides a direct HTTP client for the public FTB (Feed The Beast) API.
package ftb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Pack represents an FTB modpack.
type Pack struct {
	ID       int              `json:"id"`
	Name     string           `json:"name"`
	Synopsis string           `json:"synopsis"`
	Tags     []Tag            `json:"tags"`
	Versions []PackVersionRef `json:"versions"`
}

// Tag represents an FTB tag.
type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// PackVersionRef is a lightweight reference to a pack version.
type PackVersionRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// PackVersion contains full details for a specific pack version.
type PackVersion struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Targets []Target `json:"targets"`
	Files   []File   `json:"files"`
}

// Target describes a game or modloader target for a pack version.
type Target struct {
	Name    string `json:"name"` // "minecraft", "forge", "neoforge", "fabric"
	Version string `json:"version"`
	Type    string `json:"type"` // "game", "modloader"
}

// File represents a file within an FTB pack version.
type File struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	URL        string `json:"url"`
	Size       int64  `json:"size"`
	ServerOnly bool   `json:"serverOnly"`
	ClientOnly bool   `json:"clientOnly"`
}

// SearchResponse is the response from the FTB search endpoint.
type SearchResponse struct {
	Packs []int `json:"packs"`
	Total int   `json:"total"`
}

// APIError represents a non-2xx response from the FTB API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ftb API error %d: %s", e.StatusCode, e.Body)
}

// Client is an HTTP client for the FTB API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new FTB API client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    "https://api.modpacks.ch",
	}
}

// NewClientWithBaseURL creates a client with a custom base URL (for testing).
func NewClientWithBaseURL(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
	}
}

// SearchPacks searches for FTB packs by name.
// Returns up to 10 full pack details.
func (c *Client) SearchPacks(ctx context.Context, query string) ([]Pack, error) {
	path := "/public/modpack/search/8?term=" + url.QueryEscape(query)

	var searchResp SearchResponse
	if err := c.doJSON(ctx, path, &searchResp); err != nil {
		return nil, fmt.Errorf("search packs %q: %w", query, err)
	}

	// Limit to 10 results.
	packIDs := searchResp.Packs
	if len(packIDs) > 10 {
		packIDs = packIDs[:10]
	}

	packs := make([]Pack, 0, len(packIDs))
	for _, id := range packIDs {
		pack, err := c.GetPack(ctx, id)
		if err != nil {
			// Skip packs we can't fetch rather than failing the whole search.
			continue
		}
		packs = append(packs, *pack)
	}

	return packs, nil
}

// GetPack retrieves an FTB pack by ID.
func (c *Client) GetPack(ctx context.Context, packID int) (*Pack, error) {
	var pack Pack
	path := fmt.Sprintf("/public/modpack/%d", packID)
	if err := c.doJSON(ctx, path, &pack); err != nil {
		return nil, fmt.Errorf("get pack %d: %w", packID, err)
	}
	return &pack, nil
}

// GetVersion retrieves a specific version of an FTB pack.
func (c *Client) GetVersion(ctx context.Context, packID, versionID int) (*PackVersion, error) {
	var version PackVersion
	path := fmt.Sprintf("/public/modpack/%d/%d", packID, versionID)
	if err := c.doJSON(ctx, path, &version); err != nil {
		return nil, fmt.Errorf("get version %d/%d: %w", packID, versionID, err)
	}
	return &version, nil
}

func (c *Client) doJSON(ctx context.Context, path string, result any) error {
	reqURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Limit response reads to prevent resource exhaustion (50 MB).
	const maxResponseSize = 50 * 1024 * 1024
	limitedBody := io.LimitReader(resp.Body, maxResponseSize)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(limitedBody)
		return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	if err := json.NewDecoder(limitedBody).Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
