// Package orchestration provides Layer 2 MCP tools — multi-step operations with rollback.
package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lobo235/homelab-mcp-server/internal/clients/adguard"
	"github.com/lobo235/homelab-mcp-server/internal/clients/cloudflare"
	"github.com/lobo235/homelab-mcp-server/internal/clients/minecraft"
	"github.com/lobo235/homelab-mcp-server/internal/clients/nomad"
	"github.com/lobo235/homelab-mcp-server/internal/clients/vault"
	"github.com/lobo235/homelab-mcp-server/internal/validation"
)

// Deps holds the dependencies for orchestration tools.
type Deps struct {
	Nomad      *nomad.Client
	Adguard    *adguard.Client
	Cloudflare *cloudflare.Client
	Minecraft  *minecraft.Client
	Vault      *vault.Client
	Log        *slog.Logger

	CFZoneName        string
	MCPublicDomain    string
	ArtifactAllowlist []string
}

// Register adds all Layer 2 orchestration tools to the MCP server.
func Register(s *server.MCPServer, d *Deps) {
	s.AddTools(
		provisionMinecraftServer(d),
		destroyMinecraftServer(d),
		provisionNomadWorkload(d),
		destroyNomadWorkload(d),
	)
}

// rollbackProvision reverses completed provisioning steps in reverse order.
func (d *Deps) rollbackProvision(ctx context.Context, name string, steps []string) {
	d.Log.Warn("rolling back provision", "server", name, "completed_steps", steps)
	for i := len(steps) - 1; i >= 0; i-- {
		switch steps[i] {
		case "dns":
			hostname := name + "." + d.MCPublicDomain
			if err := d.Cloudflare.DeleteRecordByZoneName(ctx, d.CFZoneName, hostname); err != nil {
				d.Log.Error("rollback: delete DNS failed", "error", err)
			}
		case "job":
			if err := d.Nomad.StopJob(ctx, name); err != nil {
				d.Log.Error("rollback: stop job failed", "error", err)
			}
		case "secret":
			if err := d.Vault.DeleteSecret(ctx, name); err != nil {
				d.Log.Error("rollback: delete secret failed", "error", err)
			}
		case "directory":
			d.Log.Info("rollback: skipping directory deletion (preserving data)")
		}
	}
}

// waitForHealth polls job health for up to maxAttempts * interval.
func (d *Deps) waitForHealth(ctx context.Context, jobID string, maxAttempts int, interval time.Duration) bool {
	for i := 0; i < maxAttempts; i++ {
		health, err := d.Nomad.GetJobHealth(ctx, jobID)
		if err == nil && health.Healthy {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(interval):
		}
	}
	return false
}

// executeProvision runs the 6-step Minecraft server provisioning workflow.
// Order: DNS first (maximize propagation time), then secret, directory, job spec, submit, health.
func (d *Deps) executeProvision(ctx context.Context, name, hcl string) (map[string]any, error) {
	var steps []string
	hostname := name + "." + d.MCPublicDomain

	// Step 1: Create Cloudflare DNS CNAME (first to maximize propagation time).
	d.Log.Info("provision: create DNS", "server", name, "hostname", hostname)
	proxied := false
	rec := cloudflare.DNSRecord{
		Type: "CNAME", Name: hostname, Content: d.MCPublicDomain, TTL: 1, Proxied: &proxied,
	}
	if _, err := d.Cloudflare.CreateRecordByZoneName(ctx, d.CFZoneName, rec); err != nil {
		return nil, fmt.Errorf("step 1/5 create DNS failed: %w", err)
	}
	steps = append(steps, "dns")

	// Step 2: Create Vault secret (RCON password).
	d.Log.Info("provision: create secret", "server", name)
	if err := d.Vault.CreateSecret(ctx, name); err != nil {
		d.rollbackProvision(ctx, name, steps)
		return nil, fmt.Errorf("step 2/5 create secret failed: %w", err)
	}
	steps = append(steps, "secret")

	// Step 3: Init NFS server directory.
	d.Log.Info("provision: init directory", "server", name)
	if err := d.Minecraft.InitServer(ctx, name, 1000, 1000); err != nil {
		d.rollbackProvision(ctx, name, steps)
		return nil, fmt.Errorf("step 3/5 init directory failed: %w", err)
	}
	steps = append(steps, "directory")

	// Step 4: Submit Nomad job.
	d.Log.Info("provision: submit job", "server", name)
	if _, err := d.Nomad.SubmitJob(ctx, hcl); err != nil {
		d.rollbackProvision(ctx, name, steps)
		return nil, fmt.Errorf("step 4/5 submit job failed: %w", err)
	}
	steps = append(steps, "job")

	// Step 5: Wait for health (up to 5 min).
	d.Log.Info("provision: waiting for health", "server", name, "completed_steps", steps)
	healthy := d.waitForHealth(ctx, name, 30, 10*time.Second)

	result := map[string]any{
		"server": name, "hostname": hostname, "healthy": healthy, "status": "provisioned",
	}
	if !healthy {
		result["warning"] = "server provisioned but not yet healthy — check status later"
	}
	return result, nil
}

func provisionMinecraftServer(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("provision_minecraft_server",
			mcp.WithDescription("Provision a Minecraft server: init directory, create Vault secret, submit Nomad job, create Cloudflare DNS, wait for health. Rolls back on failure."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Server name")),
			mcp.WithString("hcl", mcp.Required(), mcp.Description("HCL job spec for the server")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			hcl, err := req.RequireString("hcl")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if err := validation.ValidateJobSpec(hcl, d.ArtifactAllowlist); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid_job_spec: %s", err.Error())), nil
			}

			result, err := d.executeProvision(ctx, name, hcl)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}

func destroyMinecraftServer(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("destroy_minecraft_server",
			mcp.WithDescription("Destroy a Minecraft server: stop job, delete DNS, delete secret. Best-effort cleanup."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Server name")),
			mcp.WithBoolean("delete_directory", mcp.Description("Also delete the server directory (default: false)")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			deleteDir := req.GetBool("delete_directory", false)

			var errors []string

			d.Log.Info("destroy: stop job", "server", name)
			if err := d.Nomad.StopJob(ctx, name); err != nil {
				d.Log.Error("destroy: stop job failed", "error", err)
				errors = append(errors, fmt.Sprintf("stop job: %s", err.Error()))
			}

			hostname := name + "." + d.MCPublicDomain
			d.Log.Info("destroy: delete DNS", "server", name, "hostname", hostname)
			if err := d.Cloudflare.DeleteRecordByZoneName(ctx, d.CFZoneName, hostname); err != nil {
				d.Log.Error("destroy: delete DNS failed", "error", err)
				errors = append(errors, fmt.Sprintf("delete DNS: %s", err.Error()))
			}

			d.Log.Info("destroy: delete secret", "server", name)
			if err := d.Vault.DeleteSecret(ctx, name); err != nil {
				d.Log.Error("destroy: delete secret failed", "error", err)
				errors = append(errors, fmt.Sprintf("delete secret: %s", err.Error()))
			}

			if deleteDir {
				d.Log.Info("destroy: delete directory", "server", name)
				if err := d.Minecraft.DeleteServer(ctx, name); err != nil {
					d.Log.Error("destroy: delete directory failed", "error", err)
					errors = append(errors, fmt.Sprintf("delete directory: %s", err.Error()))
				}
			}

			result := map[string]any{"server": name, "status": "destroyed"}
			if len(errors) > 0 {
				result["errors"] = errors
				result["status"] = "partially_destroyed"
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}

func provisionNomadWorkload(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("provision_nomad_workload",
			mcp.WithDescription("Provision a generic Nomad workload: submit job, create AdGuard DNS rewrite. Rolls back DNS on failure."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Workload name")),
			mcp.WithString("hcl", mcp.Required(), mcp.Description("HCL job spec")),
			mcp.WithString("dns_domain", mcp.Description("Optional DNS domain for AdGuard rewrite")),
			mcp.WithString("dns_answer", mcp.Description("Optional DNS answer IP for AdGuard rewrite")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			hcl, err := req.RequireString("hcl")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if err := validation.ValidateJobSpec(hcl, d.ArtifactAllowlist); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid_job_spec: %s", err.Error())), nil
			}

			d.Log.Info("provision workload: submit job", "name", name)
			resp, err := d.Nomad.SubmitJob(ctx, hcl)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("submit job failed: %s", err.Error())), nil
			}

			dnsDomain := req.GetString("dns_domain", "")
			dnsAnswer := req.GetString("dns_answer", "")
			if dnsDomain != "" && dnsAnswer != "" {
				d.Log.Info("provision workload: create DNS rewrite", "domain", dnsDomain)
				if err := d.Adguard.CreateRewrite(ctx, dnsDomain, dnsAnswer); err != nil {
					d.Log.Error("provision workload: DNS failed, rolling back job", "error", err)
					if stopErr := d.Nomad.StopJob(ctx, name); stopErr != nil {
						d.Log.Error("rollback: stop job failed", "error", stopErr)
					}
					return mcp.NewToolResultError(fmt.Sprintf("create DNS rewrite failed: %s", err.Error())), nil
				}
			}

			result := map[string]any{"name": name, "job_id": resp.JobID, "status": "provisioned"}
			if dnsDomain != "" {
				result["dns_domain"] = dnsDomain
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}

func destroyNomadWorkload(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("destroy_nomad_workload",
			mcp.WithDescription("Destroy a generic Nomad workload: stop job, delete AdGuard DNS rewrite."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Workload name (Nomad job ID)")),
			mcp.WithString("dns_domain", mcp.Description("DNS domain to remove from AdGuard")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			var errors []string

			d.Log.Info("destroy workload: stop job", "name", name)
			if err := d.Nomad.StopJob(ctx, name); err != nil {
				d.Log.Error("destroy workload: stop job failed", "error", err)
				errors = append(errors, fmt.Sprintf("stop job: %s", err.Error()))
			}

			dnsDomain := req.GetString("dns_domain", "")
			if dnsDomain != "" {
				d.Log.Info("destroy workload: delete DNS rewrite", "domain", dnsDomain)
				if err := d.Adguard.DeleteRewrite(ctx, dnsDomain); err != nil {
					d.Log.Error("destroy workload: delete DNS failed", "error", err)
					errors = append(errors, fmt.Sprintf("delete DNS: %s", err.Error()))
				}
			}

			result := map[string]any{"name": name, "status": "destroyed"}
			if len(errors) > 0 {
				result["errors"] = errors
				result["status"] = "partially_destroyed"
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}
