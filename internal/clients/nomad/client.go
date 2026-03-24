// Package nomad provides an HTTP client wrapper for the nomad-gateway API.
package nomad

import (
	"context"
	"fmt"
	"net/url"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

// Job represents a Nomad job summary.
type Job struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// JobDetail represents detailed job information.
type JobDetail struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Status      string   `json:"status"`
	Datacenters []string `json:"datacenters"`
	TaskGroups  []any    `json:"task_groups"`
}

// Allocation represents a Nomad allocation.
// Field names match nomad-gateway's PascalCase JSON output (Nomad API native format).
type Allocation struct {
	ID            string `json:"ID"`
	Name          string `json:"Name"`
	TaskGroup     string `json:"TaskGroup"`
	ClientStatus  string `json:"ClientStatus"`
	DesiredStatus string `json:"DesiredStatus"`
	NodeID        string `json:"NodeID"`
}

// HealthStatus represents job health information.
type HealthStatus struct {
	JobID   string `json:"job_id"`
	Healthy bool   `json:"healthy"`
	Status  string `json:"status"`
}

// SubmitResponse represents a job submission response.
type SubmitResponse struct {
	JobID string `json:"job_id"`
}

// Client wraps the nomad-gateway HTTP API.
type Client struct {
	base *clients.Base
}

// NewClient creates a new nomad-gateway client.
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

// ListJobs returns all jobs from nomad-gateway.
func (c *Client) ListJobs(ctx context.Context) ([]Job, error) {
	var jobs []Job
	if err := c.base.DoJSON(ctx, "GET", "/jobs", nil, &jobs); err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return jobs, nil
}

// GetJob returns detailed information about a specific job.
func (c *Client) GetJob(ctx context.Context, jobID string) (*JobDetail, error) {
	var job JobDetail
	if err := c.base.DoJSON(ctx, "GET", "/jobs/"+jobID, nil, &job); err != nil {
		return nil, fmt.Errorf("get job %q: %w", jobID, err)
	}
	return &job, nil
}

// GetJobSpec returns the original HCL spec for a job.
func (c *Client) GetJobSpec(ctx context.Context, jobID string) (string, error) {
	var result struct {
		Source string `json:"Source"`
	}
	if err := c.base.DoJSON(ctx, "GET", "/jobs/"+jobID+"/spec", nil, &result); err != nil {
		return "", fmt.Errorf("get job spec %q: %w", jobID, err)
	}
	return result.Source, nil
}

// SubmitJob submits a raw HCL job spec.
// The nomad-gateway expects the raw HCL text as the request body (not JSON-wrapped).
func (c *Client) SubmitJob(ctx context.Context, hcl string) (*SubmitResponse, error) {
	var resp SubmitResponse
	if err := c.base.DoJSON(ctx, "POST", "/jobs", hcl, &resp); err != nil {
		return nil, fmt.Errorf("submit job: %w", err)
	}
	return &resp, nil
}

// StopJob stops a Nomad job.
func (c *Client) StopJob(ctx context.Context, jobID string) error {
	if err := c.base.DoNoContent(ctx, "DELETE", "/jobs/"+jobID, nil); err != nil {
		return fmt.Errorf("stop job %q: %w", jobID, err)
	}
	return nil
}

// GetJobHealth returns the health status of a job.
func (c *Client) GetJobHealth(ctx context.Context, jobID string) (*HealthStatus, error) {
	var health HealthStatus
	if err := c.base.DoJSON(ctx, "GET", "/jobs/"+jobID+"/health", nil, &health); err != nil {
		return nil, fmt.Errorf("get job health %q: %w", jobID, err)
	}
	return &health, nil
}

// ListAllocations returns allocations for a job.
func (c *Client) ListAllocations(ctx context.Context, jobID string) ([]Allocation, error) {
	var allocs []Allocation
	if err := c.base.DoJSON(ctx, "GET", "/jobs/"+jobID+"/allocations", nil, &allocs); err != nil {
		return nil, fmt.Errorf("list allocations for %q: %w", jobID, err)
	}
	return allocs, nil
}

// GetAllocation returns a specific allocation.
func (c *Client) GetAllocation(ctx context.Context, jobID, allocID string) (*Allocation, error) {
	var alloc Allocation
	path := fmt.Sprintf("/jobs/%s/allocations/%s", jobID, allocID)
	if err := c.base.DoJSON(ctx, "GET", path, nil, &alloc); err != nil {
		return nil, fmt.Errorf("get allocation %q: %w", allocID, err)
	}
	return &alloc, nil
}

// RestartAllocation restarts a specific allocation.
func (c *Client) RestartAllocation(ctx context.Context, jobID, allocID string) error {
	path := fmt.Sprintf("/jobs/%s/allocations/%s/restart", jobID, allocID)
	if err := c.base.DoNoContent(ctx, "POST", path, nil); err != nil {
		return fmt.Errorf("restart allocation %q: %w", allocID, err)
	}
	return nil
}

// GetAllocationLogs returns logs for a specific allocation as plain text.
// task is required by the nomad-gateway. logType defaults to "stdout" if empty.
// grep is optional — if set, only lines containing the pattern are returned.
func (c *Client) GetAllocationLogs(ctx context.Context, jobID, allocID, task, logType, grep string) (string, error) {
	if logType == "" {
		logType = "stdout"
	}
	path := fmt.Sprintf("/jobs/%s/allocations/%s/logs?task=%s&type=%s", jobID, allocID, url.QueryEscape(task), url.QueryEscape(logType))
	if grep != "" {
		path += "&grep=" + url.QueryEscape(grep)
	}
	text, err := c.base.DoText(ctx, "GET", path, nil)
	if err != nil {
		return "", fmt.Errorf("get allocation logs %q: %w", allocID, err)
	}
	return text, nil
}
