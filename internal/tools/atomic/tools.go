// Package atomic provides Layer 1 MCP tools — each wraps a single gateway call.
package atomic

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lobo235/homelab-mcp-server/internal/clients/adguard"
	"github.com/lobo235/homelab-mcp-server/internal/clients/cloudflare"
	"github.com/lobo235/homelab-mcp-server/internal/clients/curseforge"
	"github.com/lobo235/homelab-mcp-server/internal/clients/minecraft"
	"github.com/lobo235/homelab-mcp-server/internal/clients/nomad"
	"github.com/lobo235/homelab-mcp-server/internal/clients/vault"
	"github.com/lobo235/homelab-mcp-server/internal/validation"
)

// Deps holds the dependencies for atomic tools.
type Deps struct {
	Nomad      *nomad.Client
	Adguard    *adguard.Client
	Cloudflare *cloudflare.Client
	Minecraft  *minecraft.Client
	Curseforge *curseforge.Client
	Vault      *vault.Client
	Log        *slog.Logger

	CFZoneName        string
	ArtifactAllowlist []string
}

// Register adds all Layer 1 atomic tools to the MCP server.
func Register(s *server.MCPServer, d *Deps) {
	s.AddTools(
		// Nomad tools
		listRunningJobs(d),
		getJobSpec(d),
		getJobStatus(d),
		getJobLogs(d),
		submitNomadJob(d),
		stopNomadJob(d),
		restartNomadAllocation(d),
		watchJobHealth(d),

		// Cloudflare tools
		createCloudflareRecord(d),
		deleteCloudflareRecord(d),

		// AdGuard tools
		createLocalDNSRewrite(d),
		deleteLocalDNSRewrite(d),

		// Vault tools
		createServerSecret(d),
		deleteServerSecret(d),

		// Minecraft tools
		initServerDirectory(d),
		deleteServerDirectory(d),
		executeRCONCommand(d),
		listBackups(d),
		createBackup(d),

		// CurseForge tools
		validateModpack(d),
		getModpackFiles(d),
		validateMod(d),
	)
}

func listRunningJobs(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_running_jobs",
			mcp.WithDescription("List all running Nomad jobs"),
		),
		Handler: func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			jobs, err := d.Nomad.ListJobs(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(jobs)
		},
	}
}

func getJobSpec(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_job_spec",
			mcp.WithDescription("Get the original HCL spec for a Nomad job"),
			mcp.WithString("job_id", mcp.Required(), mcp.Description("Nomad job ID")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			jobID, err := req.RequireString("job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			spec, err := d.Nomad.GetJobSpec(ctx, jobID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(spec), nil
		},
	}
}

func getJobStatus(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_job_status",
			mcp.WithDescription("Get status and allocations for a Nomad job"),
			mcp.WithString("job_id", mcp.Required(), mcp.Description("Nomad job ID")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			jobID, err := req.RequireString("job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			job, err := d.Nomad.GetJob(ctx, jobID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			allocs, err := d.Nomad.ListAllocations(ctx, jobID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			result := map[string]any{
				"job":         job,
				"allocations": allocs,
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}

func getJobLogs(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_job_logs",
			mcp.WithDescription("Get logs from a Nomad job allocation"),
			mcp.WithString("job_id", mcp.Required(), mcp.Description("Nomad job ID")),
			mcp.WithString("alloc_id", mcp.Required(), mcp.Description("Allocation ID")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			jobID, err := req.RequireString("job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			allocID, err := req.RequireString("alloc_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			logs, err := d.Nomad.GetAllocationLogs(ctx, jobID, allocID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(logs)
		},
	}
}

func submitNomadJob(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("submit_nomad_job",
			mcp.WithDescription("Submit a Nomad job spec (raw HCL). Runs pre-flight validation before submitting."),
			mcp.WithString("hcl", mcp.Required(), mcp.Description("Raw HCL job spec")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			hcl, err := req.RequireString("hcl")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Pre-flight validation.
			if err := validation.ValidateJobSpec(hcl, d.ArtifactAllowlist); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid_job_spec: %s", err.Error())), nil
			}

			resp, err := d.Nomad.SubmitJob(ctx, hcl)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(resp)
		},
	}
}

func stopNomadJob(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("stop_nomad_job",
			mcp.WithDescription("Stop a Nomad job"),
			mcp.WithString("job_id", mcp.Required(), mcp.Description("Nomad job ID")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			jobID, err := req.RequireString("job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := d.Nomad.StopJob(ctx, jobID); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Job %q stopped successfully", jobID)), nil
		},
	}
}

func restartNomadAllocation(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("restart_nomad_allocation",
			mcp.WithDescription("Restart a specific Nomad allocation"),
			mcp.WithString("job_id", mcp.Required(), mcp.Description("Nomad job ID")),
			mcp.WithString("alloc_id", mcp.Required(), mcp.Description("Allocation ID")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			jobID, err := req.RequireString("job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			allocID, err := req.RequireString("alloc_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := d.Nomad.RestartAllocation(ctx, jobID, allocID); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Allocation %q restarted", allocID)), nil
		},
	}
}

func watchJobHealth(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("watch_job_health",
			mcp.WithDescription("Check health status of a Nomad job"),
			mcp.WithString("job_id", mcp.Required(), mcp.Description("Nomad job ID")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			jobID, err := req.RequireString("job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			health, err := d.Nomad.GetJobHealth(ctx, jobID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(health)
		},
	}
}

func createCloudflareRecord(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_cloudflare_record",
			mcp.WithDescription("Create a Cloudflare DNS record"),
			mcp.WithString("type", mcp.Required(), mcp.Description("Record type (CNAME, A, etc.)")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Record name (hostname)")),
			mcp.WithString("content", mcp.Required(), mcp.Description("Record content (target)")),
			mcp.WithNumber("ttl", mcp.Description("TTL in seconds (default: 1 = auto)")),
			mcp.WithBoolean("proxied", mcp.Description("Whether to proxy through Cloudflare")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			recType, err := req.RequireString("type")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			content, err := req.RequireString("content")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			ttl := req.GetInt("ttl", 1)
			proxied := req.GetBool("proxied", false)

			rec := cloudflare.DNSRecord{
				Type:    recType,
				Name:    name,
				Content: content,
				TTL:     ttl,
				Proxied: &proxied,
			}
			created, err := d.Cloudflare.CreateRecordByZoneName(ctx, d.CFZoneName, rec)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(created)
		},
	}
}

func deleteCloudflareRecord(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("delete_cloudflare_record",
			mcp.WithDescription("Delete a Cloudflare DNS record by name"),
			mcp.WithString("record_name", mcp.Required(), mcp.Description("DNS record name to delete")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			recordName, err := req.RequireString("record_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := d.Cloudflare.DeleteRecordByZoneName(ctx, d.CFZoneName, recordName); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("DNS record %q deleted", recordName)), nil
		},
	}
}

func createLocalDNSRewrite(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_local_dns_rewrite",
			mcp.WithDescription("Create a local DNS rewrite in AdGuard Home (non-Minecraft services only)"),
			mcp.WithString("domain", mcp.Required(), mcp.Description("Domain name for the rewrite")),
			mcp.WithString("answer", mcp.Required(), mcp.Description("IP address or target")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			domain, err := req.RequireString("domain")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			answer, err := req.RequireString("answer")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := d.Adguard.CreateRewrite(ctx, domain, answer); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("DNS rewrite created: %s -> %s", domain, answer)), nil
		},
	}
}

func deleteLocalDNSRewrite(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("delete_local_dns_rewrite",
			mcp.WithDescription("Delete a local DNS rewrite from AdGuard Home"),
			mcp.WithString("domain", mcp.Required(), mcp.Description("Domain name of the rewrite to delete")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			domain, err := req.RequireString("domain")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := d.Adguard.DeleteRewrite(ctx, domain); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("DNS rewrite %q deleted", domain)), nil
		},
	}
}

func createServerSecret(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_server_secret",
			mcp.WithDescription("Create secrets for a Minecraft server (auto-generates RCON password)"),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := d.Vault.CreateSecret(ctx, name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Secret created for server %q", name)), nil
		},
	}
}

func deleteServerSecret(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("delete_server_secret",
			mcp.WithDescription("Delete all secret versions for a Minecraft server"),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := d.Vault.DeleteSecret(ctx, name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Secret deleted for server %q", name)), nil
		},
	}
}

func initServerDirectory(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("init_server_directory",
			mcp.WithDescription("Initialize NFS directory for a new Minecraft server"),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := d.Minecraft.InitServer(ctx, name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Server directory initialized for %q", name)), nil
		},
	}
}

func deleteServerDirectory(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("delete_server_directory",
			mcp.WithDescription("Delete the NFS directory for a Minecraft server"),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := d.Minecraft.DeleteServer(ctx, name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Server directory deleted for %q", name)), nil
		},
	}
}

func executeRCONCommand(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("execute_rcon_command",
			mcp.WithDescription("Send an RCON command to a Minecraft server"),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("command", mcp.Required(), mcp.Description("RCON command to execute")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			command, err := req.RequireString("command")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			response, err := d.Minecraft.ExecuteRCON(ctx, name, command)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(response), nil
		},
	}
}

func listBackups(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_backups",
			mcp.WithDescription("List backups for a Minecraft server"),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			backups, err := d.Minecraft.ListBackups(ctx, name)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(backups)
		},
	}
}

func createBackup(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_backup",
			mcp.WithDescription("Create a backup for a Minecraft server"),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			backup, err := d.Minecraft.CreateBackup(ctx, name)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(backup)
		},
	}
}

func validateModpack(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("validate_modpack",
			mcp.WithDescription("Validate a CurseForge modpack exists and return its details"),
			mcp.WithString("project_id", mcp.Required(), mcp.Description("CurseForge project ID")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			projectID, err := req.RequireString("project_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			modpack, err := d.Curseforge.GetModpack(ctx, projectID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(modpack)
		},
	}
}

func getModpackFiles(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_modpack_files",
			mcp.WithDescription("List available server-pack files and versions for a modpack"),
			mcp.WithString("project_id", mcp.Required(), mcp.Description("CurseForge project ID")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			projectID, err := req.RequireString("project_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			files, err := d.Curseforge.GetModpackFiles(ctx, projectID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(files)
		},
	}
}

func validateMod(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("validate_mod",
			mcp.WithDescription("Validate a CurseForge mod exists and return its details"),
			mcp.WithString("project_id", mcp.Required(), mcp.Description("CurseForge project ID")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			projectID, err := req.RequireString("project_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			mod, err := d.Curseforge.GetMod(ctx, projectID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(mod)
		},
	}
}
