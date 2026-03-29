// Package orchestration provides Layer 2 MCP tools — multi-step operations with rollback.
package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lobo235/homelab-mcp-server/internal/clients/adguard"
	"github.com/lobo235/homelab-mcp-server/internal/clients/cloudflare"
	"github.com/lobo235/homelab-mcp-server/internal/clients/curseforge"
	"github.com/lobo235/homelab-mcp-server/internal/clients/minecraft"
	"github.com/lobo235/homelab-mcp-server/internal/clients/nomad"
	"github.com/lobo235/homelab-mcp-server/internal/clients/vault"
	"github.com/lobo235/homelab-mcp-server/internal/tools/authz"
	"github.com/lobo235/homelab-mcp-server/internal/validation"
)

// Deps holds the dependencies for orchestration tools.
type Deps struct {
	Nomad      *nomad.Client
	Adguard    *adguard.Client
	Cloudflare *cloudflare.Client
	Minecraft  *minecraft.Client
	Curseforge *curseforge.Client
	Vault      *vault.Client
	Log        *slog.Logger

	CFZoneName        string
	MCPublicDomain    string
	ArtifactAllowlist []string
	VolumeAllowlist   []string
	NomadDatacenter   string
	NomadNodePool     string
}

// Register adds all Layer 2 orchestration tools to the MCP server.
func Register(s *server.MCPServer, d *Deps) {
	s.AddTools(
		provisionMinecraftServer(d),
		destroyMinecraftServer(d),
		getDestroyStatus(),
		renameMinecraftServer(d),
		provisionNomadWorkload(d),
		destroyNomadWorkload(d),
		addModToServer(d),
		setServerModloader(d),
	)
}

// rollbackProvision reverses completed provisioning steps in reverse order.
func (d *Deps) rollbackProvision(ctx context.Context, name string, steps []string) {
	d.Log.Warn("rolling back provision", "server", name, "completed_steps", steps)
	dirName := validation.MCServerDir(name)
	for i := len(steps) - 1; i >= 0; i-- {
		switch steps[i] {
		case "dns":
			hostname := dirName + "." + d.MCPublicDomain
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

// executeProvision runs the 6-step Minecraft server provisioning workflow.
// Order: DNS first (maximize propagation time), then secret, directory, job spec, submit, health.
// Convention: job name is "mc-{name}" but DNS and NFS use bare "{name}".
func (d *Deps) executeProvision(ctx context.Context, name, hcl string) (map[string]any, error) {
	var steps []string
	dirName := validation.MCServerDir(name)
	hostname := dirName + "." + d.MCPublicDomain

	// Step 1: Create Cloudflare DNS CNAME (first to maximize propagation time).
	// Idempotent: if the record already exists, log a warning and continue.
	d.Log.Info("provision: create DNS", "server", name, "hostname", hostname)
	proxied := false
	rec := cloudflare.DNSRecord{
		Type: "CNAME", Name: hostname, Content: d.MCPublicDomain, TTL: 1, Proxied: &proxied,
	}
	if _, err := d.Cloudflare.CreateRecordByZoneName(ctx, d.CFZoneName, rec); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			d.Log.Info("provision: DNS record already exists, continuing", "hostname", hostname)
		} else {
			return nil, fmt.Errorf("step 1/5 create DNS failed: %w", err)
		}
	}
	steps = append(steps, "dns")

	// Step 2: Create Vault secret (RCON password) — uses job name (mc-{name}).
	d.Log.Info("provision: create secret", "server", name)
	if err := d.Vault.CreateSecret(ctx, name); err != nil {
		d.rollbackProvision(ctx, name, steps)
		return nil, fmt.Errorf("step 2/5 create secret failed: %w", err)
	}
	steps = append(steps, "secret")

	// Step 3: Init NFS server directory — uses bare name without mc- prefix.
	// Idempotent: if directory already exists from a previous attempt, continue.
	d.Log.Info("provision: init directory", "server", name, "dir", dirName)
	if err := d.Minecraft.InitServer(ctx, dirName, 1001, 1001); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "chown") {
			d.Log.Info("provision: directory already exists or chown skipped, continuing", "dir", dirName)
		} else {
			d.rollbackProvision(ctx, name, steps)
			return nil, fmt.Errorf("step 3/5 init directory failed: %w", err)
		}
	}
	steps = append(steps, "directory")

	// Step 4: Submit Nomad job.
	d.Log.Info("provision: submit job", "server", name)
	if _, err := d.Nomad.SubmitJob(ctx, hcl); err != nil {
		d.rollbackProvision(ctx, name, steps)
		return nil, fmt.Errorf("step 4/5 submit job failed: %w", err)
	}
	// Don't wait for health — it blocks for minutes and kills the SSE connection.
	// Return immediately and let the user check status later.
	result := map[string]any{
		"server": name, "hostname": hostname, "status": "provisioned",
		"note": "Server provisioned successfully. It will take 1-5 minutes to start. Use get_minecraft_server_status to check when it's ready.",
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
			// No ownership check — this is a creation tool. The chatbot enforces
			// max_servers limits and records ownership after successful provisioning.
			authz.ExtractUserContext(req)
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

			if err := validation.ValidateJobSpec(hcl, d.ArtifactAllowlist, d.VolumeAllowlist, d.NomadDatacenter, d.NomadNodePool); err != nil {
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
			_, role, ownedServers := authz.ExtractUserContext(req)
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := authz.RequireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			deleteDir := req.GetBool("delete_directory", false)

			// Run destruction in background to avoid blocking the SSE connection.
			tracker.startDestroy(name, deleteDir)
			go d.executeDestroy(name, deleteDir)

			result := map[string]any{
				"server": name,
				"status": "destroying",
				"note":   "Server destruction started. Use get_destroy_status to check progress.",
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}

func getDestroyStatus() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_destroy_status",
			mcp.WithDescription("Check the status of an async server destruction. Returns step progress, errors, and final status."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Server name (e.g. mc-myserver)")),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			state := GetDestroyState(name)
			if state == nil {
				return mcp.NewToolResultError(fmt.Sprintf("no destroy operation found for %q", name)), nil
			}
			return mcp.NewToolResultJSON(state)
		},
	}
}

// executeDestroy runs the server destruction steps in the background.
// Uses context.Background since the original request context may be cancelled.
func (d *Deps) executeDestroy(name string, deleteDir bool) {
	ctx := context.Background()
	dirName := validation.MCServerDir(name)

	d.Log.Info("destroy: stop job", "server", name)
	if err := d.Nomad.StopJob(ctx, name); err != nil {
		d.Log.Error("destroy: stop job failed", "error", err)
		tracker.addError(name, fmt.Sprintf("stop job: %s", err.Error()))
	} else {
		tracker.addStep(name, "stop_job")
	}

	hostname := dirName + "." + d.MCPublicDomain
	d.Log.Info("destroy: delete DNS", "server", name, "hostname", hostname)
	if err := d.Cloudflare.DeleteRecordByZoneName(ctx, d.CFZoneName, hostname); err != nil {
		d.Log.Error("destroy: delete DNS failed", "error", err)
		tracker.addError(name, fmt.Sprintf("delete DNS: %s", err.Error()))
	} else {
		tracker.addStep(name, "delete_dns")
	}

	d.Log.Info("destroy: delete secret", "server", name)
	if err := d.Vault.DeleteSecret(ctx, name); err != nil {
		d.Log.Error("destroy: delete secret failed", "error", err)
		tracker.addError(name, fmt.Sprintf("delete secret: %s", err.Error()))
	} else {
		tracker.addStep(name, "delete_secret")
	}

	if deleteDir {
		d.Log.Info("destroy: delete directory", "server", name, "dir", dirName)
		if err := d.Minecraft.DeleteServer(ctx, dirName); err != nil {
			d.Log.Error("destroy: delete directory failed", "error", err)
			tracker.addError(name, fmt.Sprintf("delete directory: %s", err.Error()))
		} else {
			tracker.addStep(name, "delete_directory")
		}
	}

	tracker.complete(name)
	d.Log.Info("destroy: complete", "server", name, "state", tracker.get(name).Status)
}

// rollbackRename reverses completed rename steps in reverse order.
// oldName is the original job name (mc-{name}), newName is the target job name.
func (d *Deps) rollbackRename(ctx context.Context, oldName, newName string, steps []string) {
	d.Log.Warn("rolling back rename", "old", oldName, "new", newName, "completed_steps", steps)
	oldDir := validation.MCServerDir(oldName)
	newDir := validation.MCServerDir(newName)
	for i := len(steps) - 1; i >= 0; i-- {
		switch steps[i] {
		case "new_dns":
			hostname := newDir + "." + d.MCPublicDomain
			if err := d.Cloudflare.DeleteRecordByZoneName(ctx, d.CFZoneName, hostname); err != nil {
				d.Log.Error("rollback: delete new DNS failed", "error", err)
			}
		case "old_dns":
			// Re-create old DNS record.
			hostname := oldDir + "." + d.MCPublicDomain
			proxied := false
			rec := cloudflare.DNSRecord{
				Type: "CNAME", Name: hostname, Content: d.MCPublicDomain, TTL: 1, Proxied: &proxied,
			}
			if _, err := d.Cloudflare.CreateRecordByZoneName(ctx, d.CFZoneName, rec); err != nil {
				d.Log.Error("rollback: re-create old DNS failed", "error", err)
			}
		case "new_job":
			if err := d.Nomad.StopJob(ctx, newName); err != nil {
				d.Log.Error("rollback: stop new job failed", "error", err)
			}
		case "new_secret":
			if err := d.Vault.DeleteSecret(ctx, newName); err != nil {
				d.Log.Error("rollback: delete new secret failed", "error", err)
			}
		case "old_secret":
			// Re-create old secret (will get a new password, but better than nothing).
			if err := d.Vault.CreateSecret(ctx, oldName); err != nil {
				d.Log.Error("rollback: re-create old secret failed", "error", err)
			}
		case "directory":
			// Rename directory back from new to old.
			if err := d.Minecraft.RenameServer(ctx, newDir, oldDir); err != nil {
				d.Log.Error("rollback: rename directory back failed", "error", err)
			}
		case "stop_old":
			// Re-submit old job using original HCL (if we still have it, but we don't store it here).
			// Best effort: log that manual intervention may be needed.
			d.Log.Warn("rollback: old job was stopped but cannot be automatically restarted — manual intervention may be needed", "job", oldName)
		}
	}
}

// executeRename runs the multi-step Minecraft server rename workflow.
// Order: stop old job, rename directory, create new secret, delete old secret,
// fetch & update HCL, submit new job, delete old DNS, create new DNS.
func (d *Deps) executeRename(ctx context.Context, oldName, newName, oldHCL string) (map[string]any, error) {
	var steps []string
	oldDir := validation.MCServerDir(oldName)
	newDir := validation.MCServerDir(newName)
	newHostname := newDir + "." + d.MCPublicDomain

	// Step 1: Stop the old Nomad job.
	d.Log.Info("rename: stop old job", "old", oldName)
	if err := d.Nomad.StopJob(ctx, oldName); err != nil {
		return nil, fmt.Errorf("step 1/7 stop old job failed: %w", err)
	}
	steps = append(steps, "stop_old")

	// Step 2: Rename NFS directory (old bare name -> new bare name).
	d.Log.Info("rename: rename directory", "old_dir", oldDir, "new_dir", newDir)
	if err := d.Minecraft.RenameServer(ctx, oldDir, newDir); err != nil {
		d.rollbackRename(ctx, oldName, newName, steps)
		return nil, fmt.Errorf("step 2/7 rename directory failed: %w", err)
	}
	steps = append(steps, "directory")

	// Step 3: Create new Vault secret (auto-generates new RCON password).
	d.Log.Info("rename: create new secret", "new", newName)
	if err := d.Vault.CreateSecret(ctx, newName); err != nil {
		d.rollbackRename(ctx, oldName, newName, steps)
		return nil, fmt.Errorf("step 3/7 create new secret failed: %w", err)
	}
	steps = append(steps, "new_secret")

	// Step 4: Delete old Vault secret.
	d.Log.Info("rename: delete old secret", "old", oldName)
	if err := d.Vault.DeleteSecret(ctx, oldName); err != nil {
		d.rollbackRename(ctx, oldName, newName, steps)
		return nil, fmt.Errorf("step 4/7 delete old secret failed: %w", err)
	}
	steps = append(steps, "old_secret")

	// Step 5: String-replace old name with new name in HCL and submit.
	d.Log.Info("rename: update and submit HCL", "old", oldName, "new", newName)
	newHCL := strings.ReplaceAll(oldHCL, oldName, newName)
	newHCL = strings.ReplaceAll(newHCL, oldDir, newDir)
	if _, err := d.Nomad.SubmitJob(ctx, newHCL); err != nil {
		d.rollbackRename(ctx, oldName, newName, steps)
		return nil, fmt.Errorf("step 5/7 submit renamed job failed: %w", err)
	}
	steps = append(steps, "new_job")

	// Step 6: Delete old DNS CNAME.
	oldHostname := oldDir + "." + d.MCPublicDomain
	d.Log.Info("rename: delete old DNS", "hostname", oldHostname)
	if err := d.Cloudflare.DeleteRecordByZoneName(ctx, d.CFZoneName, oldHostname); err != nil {
		d.Log.Warn("rename: delete old DNS failed (non-fatal, continuing)", "error", err)
		// Non-fatal: old DNS pointing to same domain is harmless.
	}
	steps = append(steps, "old_dns")

	// Step 7: Create new DNS CNAME.
	d.Log.Info("rename: create new DNS", "hostname", newHostname)
	proxied := false
	rec := cloudflare.DNSRecord{
		Type: "CNAME", Name: newHostname, Content: d.MCPublicDomain, TTL: 1, Proxied: &proxied,
	}
	if _, err := d.Cloudflare.CreateRecordByZoneName(ctx, d.CFZoneName, rec); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			d.Log.Info("rename: new DNS record already exists, continuing", "hostname", newHostname)
		} else {
			d.rollbackRename(ctx, oldName, newName, steps)
			return nil, fmt.Errorf("step 7/7 create new DNS failed: %w", err)
		}
	}

	result := map[string]any{
		"old_name":     oldName,
		"new_name":     newName,
		"hostname":     newHostname,
		"status":       "renamed",
		"note":         "Server renamed successfully. It will take 1-5 minutes to start under the new name. Use get_minecraft_server_status to check when it's ready.",
		"rcon_updated": true,
	}
	return result, nil
}

func renameMinecraftServer(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("rename_minecraft_server",
			mcp.WithDescription("Rename a Minecraft server — stops the job, renames NFS directory, creates new Vault secret, submits renamed HCL, updates DNS. The old server is stopped and replaced."),
			mcp.WithString("old_name", mcp.Required(), mcp.Description("Current server name (e.g., mc-survival)")),
			mcp.WithString("new_name", mcp.Required(), mcp.Description("New server name (e.g., mc-austycraft)")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := authz.ExtractUserContext(req)
			oldName, err := req.RequireString("old_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			newName, err := req.RequireString("new_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Validate both names.
			if err := validation.ValidateServerName(oldName); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid old_name: %s", err.Error())), nil
			}
			if err := validation.ValidateServerName(newName); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid new_name: %s", err.Error())), nil
			}
			if oldName == newName {
				return mcp.NewToolResultError("old_name and new_name must be different"), nil
			}

			// Authorization: user must own the old server.
			if err := authz.RequireServerAccess(role, oldName, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Fetch the old job's HCL spec before stopping it.
			d.Log.Info("rename: fetch old job spec", "server", oldName)
			oldHCL, err := d.Nomad.GetJobSpec(ctx, oldName)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to fetch old job spec: %s", err.Error())), nil
			}

			result, err := d.executeRename(ctx, oldName, newName, oldHCL)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
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
			_, role, _ := authz.ExtractUserContext(req)
			if err := authz.RequireAdmin(role); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
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

			if err := validation.ValidateJobSpec(hcl, d.ArtifactAllowlist, d.VolumeAllowlist, d.NomadDatacenter, d.NomadNodePool); err != nil {
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
			_, role, _ := authz.ExtractUserContext(req)
			if err := authz.RequireAdmin(role); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
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
