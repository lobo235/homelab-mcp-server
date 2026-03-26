// Package modrinth provides a direct HTTP client for the public Modrinth API.
package modrinth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Project represents a Modrinth project (modpack, mod, etc.).
type Project struct {
	ID          string   `json:"id"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	Downloads   int      `json:"downloads"`
	ProjectType string   `json:"project_type"`
}

// Version represents a specific version of a Modrinth project.
type Version struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	VersionNumber string        `json:"version_number"`
	GameVersions  []string      `json:"game_versions"`
	Loaders       []string      `json:"loaders"`
	Files         []VersionFile `json:"files"`
}

// VersionFile represents a downloadable file in a Modrinth version.
type VersionFile struct {
	URL      string            `json:"url"`
	Filename string            `json:"filename"`
	Primary  bool              `json:"primary"`
	Size     int64             `json:"size"`
	Hashes   map[string]string `json:"hashes"`
}

// APIError represents a non-2xx response from the Modrinth API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("modrinth API error %d: %s", e.StatusCode, e.Body)
}

// Client is an HTTP client for the Modrinth API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
}

// searchResponse is the envelope returned by the Modrinth search endpoint.
type searchResponse struct {
	Hits []Project `json:"hits"`
}

// NewClient creates a new Modrinth API client.
func NewClient(userAgent string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    "https://api.modrinth.com/v2",
		userAgent:  userAgent,
	}
}

// NewClientWithBaseURL creates a client with a custom base URL (for testing).
func NewClientWithBaseURL(baseURL, userAgent string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
		userAgent:  userAgent,
	}
}

// SearchProjects searches for Modrinth projects by query and type.
func (c *Client) SearchProjects(ctx context.Context, query string, projectType string) ([]Project, error) {
	facets := fmt.Sprintf(`[["project_type:%s"]]`, projectType)
	path := fmt.Sprintf("/search?query=%s&facets=%s", url.QueryEscape(query), url.QueryEscape(facets))

	var resp searchResponse
	if err := c.doJSON(ctx, path, &resp); err != nil {
		return nil, fmt.Errorf("search projects %q: %w", query, err)
	}
	return resp.Hits, nil
}

// GetProject retrieves a single Modrinth project by ID or slug.
func (c *Client) GetProject(ctx context.Context, idOrSlug string) (*Project, error) {
	var project Project
	if err := c.doJSON(ctx, "/project/"+url.PathEscape(idOrSlug), &project); err != nil {
		return nil, fmt.Errorf("get project %q: %w", idOrSlug, err)
	}
	return &project, nil
}

// GetVersions returns all versions for a Modrinth project.
func (c *Client) GetVersions(ctx context.Context, projectID string) ([]Version, error) {
	var versions []Version
	if err := c.doJSON(ctx, "/project/"+url.PathEscape(projectID)+"/version", &versions); err != nil {
		return nil, fmt.Errorf("get versions for %q: %w", projectID, err)
	}
	return versions, nil
}

// GetVersion retrieves a single Modrinth version by ID.
func (c *Client) GetVersion(ctx context.Context, versionID string) (*Version, error) {
	var version Version
	if err := c.doJSON(ctx, "/version/"+url.PathEscape(versionID), &version); err != nil {
		return nil, fmt.Errorf("get version %q: %w", versionID, err)
	}
	return &version, nil
}

func (c *Client) doJSON(ctx context.Context, path string, result any) error {
	reqURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

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
