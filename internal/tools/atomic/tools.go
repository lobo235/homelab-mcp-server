// Package atomic provides Layer 1 MCP tools — each wraps a single gateway call.
package atomic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lobo235/homelab-mcp-server/internal/clients/adguard"
	"github.com/lobo235/homelab-mcp-server/internal/clients/cloudflare"
	"github.com/lobo235/homelab-mcp-server/internal/clients/curseforge"
	"github.com/lobo235/homelab-mcp-server/internal/clients/minecraft"
	"github.com/lobo235/homelab-mcp-server/internal/clients/nomad"
	"github.com/lobo235/homelab-mcp-server/internal/clients/vault"
	"github.com/lobo235/homelab-mcp-server/internal/itzgcache"
	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
	"github.com/lobo235/homelab-mcp-server/internal/speccache"
	"github.com/lobo235/homelab-mcp-server/internal/validation"
)

// requireServerName extracts and validates a server name from the request.
func requireServerName(req mcp.CallToolRequest, param string) (string, error) {
	name, err := req.RequireString(param)
	if err != nil {
		return "", err
	}
	if err := validation.ValidateServerName(name); err != nil {
		return "", err
	}
	return name, nil
}

// mcDir resolves the NFS directory name from a Nomad job ID (strips mc- prefix).
func mcDir(jobID string) string { return validation.MCServerDir(jobID) }

// requireJobID extracts and validates a job ID from the request.
func requireJobID(req mcp.CallToolRequest, param string) (string, error) {
	id, err := req.RequireString(param)
	if err != nil {
		return "", err
	}
	if !validation.ValidateJobName(id) {
		return "", fmt.Errorf("invalid job ID %q: must match ^[a-z0-9][a-z0-9-]{1,48}[a-z0-9]$", id)
	}
	return id, nil
}

// requireAllocID extracts and validates an allocation ID from the request.
func requireAllocID(req mcp.CallToolRequest, param string) (string, error) {
	id, err := req.RequireString(param)
	if err != nil {
		return "", err
	}
	if err := validation.ValidateAllocID(id); err != nil {
		return "", err
	}
	return id, nil
}

// Deps holds the dependencies for atomic tools.
type Deps struct {
	Nomad      *nomad.Client
	Adguard    *adguard.Client
	Cloudflare *cloudflare.Client
	Minecraft  *minecraft.Client
	Curseforge *curseforge.Client
	Vault      *vault.Client
	ItzgDocs   *itzgcache.Cache
	SpecCache  *speccache.Cache
	ModpackKB  *modpackkb.KB
	Log        *slog.Logger

	CFZoneName        string
	ArtifactAllowlist []string
	NFSBasePath       string
	NomadDatacenter   string
	NomadNodePool     string
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
		downloadToServer(d),
		getDownloadStatus(d),
		listArchiveContents(d),
		listServerFiles(d),
		readServerFile(d),
		writeServerFile(d),
		moveServerFile(d),
		deleteServerFile(d),

		// Utility tools
		fetchArtifact(d),

		// CurseForge tools
		searchModpacks(d),
		searchMods(d),
		validateModpack(d),
		getModpackFiles(d),
		getModpackFile(d),
		validateMod(d),
		getModFile(d),

		// itzg documentation tools
		searchItzgDocs(d),
		getItzgDoc(d),

		// Modpack knowledge base tools
		getModpackKnowledge(d),
		saveModpackKnowledge(d),
		listModpackKnowledge(d),
		deleteModpackKnowledge(d),
	)
}

func listRunningJobs(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_running_jobs",
			mcp.WithDescription("List all running Nomad jobs"),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			jobs, err := d.Nomad.ListJobs(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			// Non-admin users only see jobs matching their owned servers.
			if role != "admin" && role != "" {
				owned := make(map[string]bool, len(ownedServers))
				for _, s := range ownedServers {
					owned[s] = true
				}
				filtered := make([]nomad.Job, 0, len(jobs))
				for _, j := range jobs {
					if owned[j.ID] {
						filtered = append(filtered, j)
					}
				}
				jobs = filtered
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
			// No ownership check — read-only, needed as reference template for new server creation.
			extractUserContext(req)
			jobID, err := requireJobID(req, "job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			spec, err := d.Nomad.GetJobSpec(ctx, jobID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			// Update spec cache with latest HCL (only if non-empty to avoid clobbering).
			if d.SpecCache != nil && spec != "" {
				_ = d.SpecCache.Write(jobID, spec)
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
			_, role, ownedServers := extractUserContext(req)
			jobID, err := requireJobID(req, "job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireJobAccess(role, jobID, ownedServers); err != nil {
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
			mcp.WithDescription("Get logs from a Nomad job allocation. Returns log_bytes in the response — if logs are large (>10KB), consider using the grep parameter to filter for specific patterns like 'ERROR', 'WARN', or mod names instead of fetching full logs. You must call get_job_status first to obtain a live allocation ID."),
			mcp.WithString("job_id", mcp.Required(), mcp.Description("Nomad job ID")),
			mcp.WithString("alloc_id", mcp.Required(), mcp.Description("Allocation ID (get this from get_job_status)")),
			mcp.WithString("task", mcp.Required(), mcp.Description("Task name within the allocation (get this from get_job_status)")),
			mcp.WithString("log_type", mcp.Description("Log type: stdout or stderr (default: stdout)")),
			mcp.WithString("grep", mcp.Description("Filter pattern — only return log lines containing this text (e.g., 'ERROR', 'WARN', 'client-only'). Use this for large logs instead of fetching everything.")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			jobID, err := requireJobID(req, "job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireJobAccess(role, jobID, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			allocID, err := requireAllocID(req, "alloc_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			task, err := req.RequireString("task")
			if err != nil {
				return mcp.NewToolResultError("task is required"), nil
			}
			logType := req.GetString("log_type", "")
			grep := req.GetString("grep", "")
			logs, err := d.Nomad.GetAllocationLogs(ctx, jobID, allocID, task, logType, grep)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			result := map[string]any{
				"logs":      logs,
				"log_bytes": len(logs),
			}
			return mcp.NewToolResultJSON(result)
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
			// Extract user context (job ownership checked at orchestration layer).
			extractUserContext(req)
			hcl, err := req.RequireString("hcl")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Pre-flight validation.
			if err := validation.ValidateJobSpec(hcl, d.ArtifactAllowlist, d.NFSBasePath, d.NomadDatacenter, d.NomadNodePool); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid_job_spec: %s", err.Error())), nil
			}

			resp, err := d.Nomad.SubmitJob(ctx, hcl)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			// Update spec cache with the submitted HCL.
			if d.SpecCache != nil && resp.JobID != "" {
				_ = d.SpecCache.Write(resp.JobID, hcl)
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
			_, role, ownedServers := extractUserContext(req)
			jobID, err := requireJobID(req, "job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireJobAccess(role, jobID, ownedServers); err != nil {
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
			mcp.WithDescription("Restart a specific Nomad allocation. Call get_job_status first to obtain a live allocation ID."),
			mcp.WithString("job_id", mcp.Required(), mcp.Description("Nomad job ID")),
			mcp.WithString("alloc_id", mcp.Required(), mcp.Description("Allocation ID (get this from get_job_status)")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			jobID, err := requireJobID(req, "job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireJobAccess(role, jobID, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			allocID, err := requireAllocID(req, "alloc_id")
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
			_, role, ownedServers := extractUserContext(req)
			jobID, err := requireJobID(req, "job_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireJobAccess(role, jobID, ownedServers); err != nil {
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
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
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
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
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
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dirName := mcDir(name)
			if err := d.Minecraft.InitServer(ctx, dirName, 1000, 1000); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Server directory initialized for %q (dir: %s)", name, dirName)), nil
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
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dirName := mcDir(name)
			if err := d.Minecraft.DeleteServer(ctx, dirName); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Server directory deleted for %q (dir: %s)", name, dirName)), nil
		},
	}
}

// blockedRCONCommands are RCON commands that must not be executed via the chatbot.
// These are either destructive with no undo, or duplicate existing tools.
var blockedRCONCommands = []string{
	"stop",     // Use stop_nomad_job instead — proper orchestration with DNS cleanup
	"save-off", // Too easy to forget save-on, causes silent data loss
	"ban-ip",   // IP bans are hard to undo and can affect shared IPs
}

// isBlockedRCONCommand checks if the command starts with a blocked keyword.
func isBlockedRCONCommand(command string) (string, bool) {
	cmd := strings.ToLower(strings.TrimSpace(command))
	for _, blocked := range blockedRCONCommands {
		if cmd == blocked || strings.HasPrefix(cmd, blocked+" ") {
			return blocked, true
		}
	}
	return "", false
}

func executeRCONCommand(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("execute_rcon_command",
			mcp.WithDescription("Send an RCON command to a Minecraft server. Some commands are blocked for safety: 'stop' (use stop_nomad_job instead), 'save-off' (risk of data loss), 'ban-ip' (hard to undo). For destructive commands like ban, op, deop, kick, gamerule, fill, setblock, kill — always confirm with the user before executing."),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("command", mcp.Required(), mcp.Description("RCON command to execute")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			command, err := req.RequireString("command")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Block dangerous commands.
			if blocked, ok := isBlockedRCONCommand(command); ok {
				return mcp.NewToolResultError(fmt.Sprintf("RCON command %q is blocked for safety. Use the appropriate MCP tool instead (e.g., stop_nomad_job for stopping servers).", blocked)), nil
			}

			// RCON uses the full job ID (mc-atm10) — the minecraft-gateway needs it
			// for Nomad allocation lookup. Don't strip the mc- prefix here.
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
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			backups, err := d.Minecraft.ListBackups(ctx, mcDir(name))
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
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			backup, err := d.Minecraft.CreateBackup(ctx, mcDir(name))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(backup)
		},
	}
}

func downloadToServer(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("download_to_server",
			mcp.WithDescription("Start an async download of a file from CurseForge, Modrinth, or FTB to a Minecraft server directory. Returns immediately with a download ID — use get_download_status to poll for completion. Large downloads (modpack server packs) can take several minutes."),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("url", mcp.Required(), mcp.Description("Download URL (CurseForge, Modrinth, FTB, etc.)")),
			mcp.WithString("dest_path", mcp.Description("Destination DIRECTORY relative to server root (default: \".\"). The downloaded file keeps its original name from the URL. E.g., \"mods\" puts mod.jar into mods/mod.jar.")),
			mcp.WithBoolean("extract", mcp.Description("Extract archive after download (default: false)")),
			mcp.WithString("mode", mcp.Description("Write mode: overwrite, skip_existing, or clean_first (default: overwrite)")),
			mcp.WithNumber("uid", mcp.Description("File owner UID (default: 1001)")),
			mcp.WithNumber("gid", mcp.Description("File owner GID (default: 1001)")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			rawURL, err := req.RequireString("url")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			destPath := req.GetString("dest_path", ".")
			extract := req.GetBool("extract", false)
			mode := req.GetString("mode", "overwrite")
			uid := req.GetInt("uid", 1001)
			gid := req.GetInt("gid", 1001)

			dirName := mcDir(name)
			dlReq := minecraft.DownloadRequest{
				URL:      rawURL,
				DestPath: destPath,
				Extract:  extract,
				Mode:     mode,
				UID:      uid,
				GID:      gid,
			}
			result, err := d.Minecraft.DownloadToServer(ctx, dirName, dlReq)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}

func getDownloadStatus(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_download_status",
			mcp.WithDescription("Check the status of an async file download. Returns status (running/done/failed), and on completion includes files_count and total_bytes. Tell the user the download is in progress and suggest checking back in a minute for large files."),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("download_id", mcp.Required(), mcp.Description("Download ID returned by download_to_server")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			downloadID, err := req.RequireString("download_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dirName := mcDir(name)
			status, err := d.Minecraft.GetDownloadStatus(ctx, dirName, downloadID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(status)
		},
	}
}

func listArchiveContents(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_archive_contents",
			mcp.WithDescription("List files inside a zip or tar archive on a Minecraft server's filesystem. Useful for inspecting downloaded server pack archives before extraction."),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("path", mcp.Required(), mcp.Description("Relative path to the archive within the server directory")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			archivePath, err := req.RequireString("path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dirName := mcDir(name)
			entries, err := d.Minecraft.ListArchiveContents(ctx, dirName, archivePath)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(entries)
		},
	}
}

func listServerFiles(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_server_files",
			mcp.WithDescription("List files and directories in a Minecraft server's filesystem. Use this to explore the server directory structure, find config files, check mod installations, etc."),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name (bare name without mc- prefix, e.g., 'atm10')")),
			mcp.WithString("path", mcp.Description("Subdirectory path relative to server root (default: root directory). E.g., 'mods', 'config', 'logs'")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			subPath := req.GetString("path", "")
			files, err := d.Minecraft.ListFiles(ctx, name, subPath)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(files)
		},
	}
}

func readServerFile(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("read_server_file",
			mcp.WithDescription("Read a file from a Minecraft server's filesystem. Use this to inspect config files (server.properties, ops.json, config/*.toml, etc.)."),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("path", mcp.Required(), mcp.Description("Relative path to the file within the server directory")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			filePath, err := req.RequireString("path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dirName := mcDir(name)
			content, err := d.Minecraft.ReadFile(ctx, dirName, filePath)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(content), nil
		},
	}
}

func writeServerFile(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("write_server_file",
			mcp.WithDescription("Write content to a file on a Minecraft server's filesystem. Use this for writing config files, scripts, or other text files."),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("path", mcp.Required(), mcp.Description("Relative path to the file within the server directory")),
			mcp.WithString("content", mcp.Required(), mcp.Description("File content to write")),
			mcp.WithNumber("uid", mcp.Description("File owner UID (default: 1001)")),
			mcp.WithNumber("gid", mcp.Description("File owner GID (default: 1001)")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			filePath, err := req.RequireString("path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			content, err := req.RequireString("content")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			uid := req.GetInt("uid", 1001)
			gid := req.GetInt("gid", 1001)

			dirName := mcDir(name)
			if err := d.Minecraft.WriteFile(ctx, dirName, filePath, content, uid, gid); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("File %q written to server %q", filePath, name)), nil
		},
	}
}

func moveServerFile(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("move_server_file",
			mcp.WithDescription("Move or rename a file/directory within a Minecraft server's filesystem. Use this to reorganize files, rename configs, or move downloaded files to the right location."),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("src_path", mcp.Required(), mcp.Description("Source path relative to server root")),
			mcp.WithString("dst_path", mcp.Required(), mcp.Description("Destination path relative to server root")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			srcPath, err := req.RequireString("src_path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dstPath, err := req.RequireString("dst_path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dirName := mcDir(name)
			if err := d.Minecraft.MoveFile(ctx, dirName, srcPath, dstPath); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Moved %q to %q on server %q", srcPath, dstPath, name)), nil
		},
	}
}

func deleteServerFile(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("delete_server_file",
			mcp.WithDescription("Delete a file or directory from a Minecraft server's filesystem. Use with caution — this is destructive and cannot be undone. Always confirm with the user before deleting."),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("path", mcp.Required(), mcp.Description("Path to delete, relative to server root")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, role, ownedServers := extractUserContext(req)
			name, err := requireServerName(req, "server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := requireServerAccess(role, name, ownedServers); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			filePath, err := req.RequireString("path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			dirName := mcDir(name)
			if err := d.Minecraft.DeleteFile(ctx, dirName, filePath); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Deleted %q from server %q", filePath, name)), nil
		},
	}
}

func fetchArtifact(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("fetch_artifact",
			mcp.WithDescription("Fetch the contents of a trusted artifact URL (scripts, configs). Only URLs from the artifact allowlist are permitted. Use this to read helper scripts referenced in Nomad job specs so you can understand and adapt them."),
			mcp.WithString("url", mcp.Required(), mcp.Description("Full URL to fetch (must be on the artifact allowlist, e.g., raw.githubusercontent.com/lobo235/...)")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			rawURL, err := req.RequireString("url")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Validate against allowlist.
			stripped := strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
			allowlist := append([]string{}, validation.DefaultArtifactAllowlist...)
			allowlist = append(allowlist, d.ArtifactAllowlist...)
			allowed := false
			for _, prefix := range allowlist {
				if strings.HasPrefix(stripped, prefix) {
					allowed = true
					break
				}
			}
			if !allowed {
				return mcp.NewToolResultError(fmt.Sprintf("URL %q is not on the artifact allowlist", rawURL)), nil
			}

			// Ensure HTTPS.
			fetchURL := rawURL
			if !strings.HasPrefix(fetchURL, "https://") {
				fetchURL = "https://" + stripped
			}

			// Add cache-busting param to bypass GitHub CDN caching of stale content.
			if strings.Contains(fetchURL, "raw.githubusercontent.com") {
				sep := "?"
				if strings.Contains(fetchURL, "?") {
					sep = "&"
				}
				fetchURL += sep + "t=" + fmt.Sprintf("%d", time.Now().Unix())
			}

			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Get(fetchURL)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("fetch failed: %s", err.Error())), nil
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("fetch returned HTTP %d", resp.StatusCode)), nil
			}

			// Cap at 64KB to avoid token overload.
			body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("read failed: %s", err.Error())), nil
			}

			return mcp.NewToolResultText(string(body)), nil
		},
	}
}

func searchModpacks(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("search_modpacks",
			mcp.WithDescription("Search CurseForge for modpacks by name. Use this to find project IDs when you don't know them. Returns up to 10 results sorted by popularity. Results include deployment_knowledge if the modpack is in the knowledge base."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Search query (e.g., 'All the Mods 10', 'ATM9', 'RLCraft')")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := req.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			results, err := d.Curseforge.SearchModpacks(ctx, query)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Enrich with KB knowledge if available.
			if d.ModpackKB != nil {
				enriched := toMaps(results)
				enriched = enrichResultsWithKB(d.ModpackKB, enriched)
				return mcp.NewToolResultJSON(enriched)
			}

			return mcp.NewToolResultJSON(results)
		},
	}
}

func searchMods(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("search_mods",
			mcp.WithDescription("Search CurseForge for mods by name. Use this to find mod project IDs when you don't know them. Returns up to 10 results sorted by popularity."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Search query (e.g., 'JEI', 'Create', 'Mekanism')")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := req.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			results, err := d.Curseforge.SearchMods(ctx, query)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(results)
		},
	}
}

func validateModpack(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("validate_modpack",
			mcp.WithDescription("Validate a CurseForge modpack exists and return its details. Includes deployment_knowledge if the modpack is in the knowledge base."),
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

			// Enrich with KB knowledge if available.
			if d.ModpackKB != nil {
				enriched := toMap(modpack)
				enriched = enrichWithKB(d.ModpackKB, enriched)
				return mcp.NewToolResultJSON(enriched)
			}

			return mcp.NewToolResultJSON(modpack)
		},
	}
}

func getModpackFiles(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_modpack_files",
			mcp.WithDescription("List available server-pack files and versions for a modpack. Includes deployment_knowledge if the modpack is in the knowledge base."),
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

			// Enrich with KB knowledge if available.
			if d.ModpackKB != nil {
				cfID, _ := strconv.Atoi(projectID)
				if cfID > 0 {
					if mk, kbErr := d.ModpackKB.GetByCurseForgeID(cfID); kbErr == nil {
						result := map[string]any{
							"files":                files,
							"deployment_knowledge": mk,
						}
						return mcp.NewToolResultJSON(result)
					}
				}
			}

			return mcp.NewToolResultJSON(files)
		},
	}
}

func getModpackFile(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_modpack_file",
			mcp.WithDescription("Get details of a specific modpack file by file ID (useful for fetching server pack info via serverPackFileId)"),
			mcp.WithString("project_id", mcp.Required(), mcp.Description("CurseForge project ID")),
			mcp.WithString("file_id", mcp.Required(), mcp.Description("CurseForge file ID")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			projectID, err := req.RequireString("project_id")
			if err != nil {
				return mcp.NewToolResultError("project_id is required"), nil
			}
			fileID, err := req.RequireString("file_id")
			if err != nil {
				return mcp.NewToolResultError("file_id is required"), nil
			}
			file, err := d.Curseforge.GetModpackFile(ctx, projectID, fileID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.Marshal(file)
			return mcp.NewToolResultText(string(data)), nil
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

func getModFile(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_mod_file",
			mcp.WithDescription("Get details of a specific mod file by file ID"),
			mcp.WithString("project_id", mcp.Required(), mcp.Description("CurseForge project ID")),
			mcp.WithString("file_id", mcp.Required(), mcp.Description("CurseForge file ID")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			projectID, err := req.RequireString("project_id")
			if err != nil {
				return mcp.NewToolResultError("project_id is required"), nil
			}
			fileID, err := req.RequireString("file_id")
			if err != nil {
				return mcp.NewToolResultError("file_id is required"), nil
			}
			file, err := d.Curseforge.GetModFile(ctx, projectID, fileID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.Marshal(file)
			return mcp.NewToolResultText(string(data)), nil
		},
	}
}
