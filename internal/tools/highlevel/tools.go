// Package highlevel provides Layer 3 MCP tools — user intent fulfillment.
package highlevel

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lobo235/homelab-mcp-server/internal/clients/minecraft"
	"github.com/lobo235/homelab-mcp-server/internal/clients/nomad"
	"github.com/lobo235/homelab-mcp-server/internal/validation"
)

// playerNamePattern validates Minecraft usernames.
var playerNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{1,16}$`)

// Deps holds the dependencies for high-level tools.
type Deps struct {
	Nomad     *nomad.Client
	Minecraft *minecraft.Client
	Log       *slog.Logger
}

// Register adds all Layer 3 high-level tools to the MCP server.
func Register(s *server.MCPServer, d *Deps) {
	s.AddTools(
		createMinecraftServer(d),
		destroyMinecraftServerByName(d),
		upgradeMinecraftServer(d),
		getMinecraftServerStatus(d),
		sendRCONCommand(d),
		opPlayer(d),
		deopPlayer(d),
		backupServer(d),
		deployGenericWorkload(d),
	)
}

// createMinecraftServer provides the full context Claude needs to generate a job spec,
// then the chatbot calls provision_minecraft_server with the generated HCL.
// This tool gathers reference specs and returns them so Claude can adapt them.
func createMinecraftServer(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_minecraft_server",
			mcp.WithDescription("Gather reference job specs for creating a new Minecraft server. Returns existing job specs that Claude should adapt. After generating HCL, call provision_minecraft_server to deploy."),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Name for the new server (e.g., mc-atm10-kids)")),
			mcp.WithString("modpack_type", mcp.Description("Modpack type: 'vanilla', 'modpack', or 'custom'")),
			mcp.WithString("reference_job", mcp.Description("Specific job ID to use as reference (optional; auto-selects if omitted)")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			serverName, err := req.RequireString("server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(serverName); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			modpackType := req.GetString("modpack_type", "modpack")
			referenceJob := req.GetString("reference_job", "")

			// If no reference job specified, auto-select based on modpack type.
			if referenceJob == "" {
				switch modpackType {
				case "vanilla":
					referenceJob = "mc-vanilla1"
				default:
					referenceJob = "mc-atm10"
				}
			}

			// Fetch the reference spec.
			spec, err := d.Nomad.GetJobSpec(ctx, referenceJob)
			if err != nil {
				// If reference not found, list all jobs so Claude can pick one.
				jobs, listErr := d.Nomad.ListJobs(ctx)
				if listErr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("could not fetch reference spec %q and failed to list jobs: %s", referenceJob, listErr.Error())), nil
				}
				result := map[string]any{
					"server_name":         serverName,
					"reference_not_found": referenceJob,
					"available_jobs":      jobs,
					"instructions":        "Reference job not found. Choose a job from the list and call create_minecraft_server again with reference_job set, or call get_job_spec to inspect a job's HCL before adapting it.",
				}
				return mcp.NewToolResultJSON(result)
			}

			result := map[string]any{
				"server_name":    serverName,
				"reference_job":  referenceJob,
				"reference_spec": spec,
				"instructions":   "Adapt this reference HCL spec for the new server. Change the job name, adjust resources as needed, update container env vars. Then call provision_minecraft_server with the generated HCL.",
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}

func destroyMinecraftServerByName(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("destroy_minecraft_server_by_name",
			mcp.WithDescription("Destroy a Minecraft server by name. Delegates to destroy_minecraft_server orchestration tool."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Minecraft server name")),
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

			// This is a convenience wrapper — the chatbot should call destroy_minecraft_server directly.
			// Return instructions for the orchestration call.
			deleteDir := req.GetBool("delete_directory", false)

			result := map[string]any{
				"action":           "destroy",
				"server_name":      name,
				"delete_directory": deleteDir,
				"instructions":     "Call destroy_minecraft_server with these parameters to proceed with teardown.",
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}

func upgradeMinecraftServer(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("upgrade_minecraft_server",
			mcp.WithDescription("Upgrade a Minecraft server: backup current world, fetch current spec, return both so Claude can generate an updated spec. After generating HCL, call submit_nomad_job to apply."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("new_version", mcp.Description("New modpack version or game version")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			newVersion := req.GetString("new_version", "")

			// Step 1: Create a backup before upgrade.
			d.Log.Info("upgrade: creating backup", "server", name)
			backup, backupErr := d.Minecraft.CreateBackup(ctx, name)

			// Step 2: Fetch current job spec.
			spec, specErr := d.Nomad.GetJobSpec(ctx, name)
			if specErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to fetch current spec for %q: %s", name, specErr.Error())), nil
			}

			result := map[string]any{
				"server_name":  name,
				"current_spec": spec,
				"instructions": "Modify this HCL spec with the new version. Then call submit_nomad_job with the updated HCL to apply the upgrade.",
			}
			if newVersion != "" {
				result["new_version"] = newVersion
			}
			if backupErr != nil {
				result["backup_warning"] = fmt.Sprintf("backup failed: %s — proceed with caution", backupErr.Error())
			} else {
				result["backup"] = backup
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}

func getMinecraftServerStatus(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_minecraft_server_status",
			mcp.WithDescription("Get comprehensive status of a Minecraft server including job state and allocation health"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Minecraft server name")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			job, err := d.Nomad.GetJob(ctx, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get job failed: %s", err.Error())), nil
			}

			allocs, err := d.Nomad.ListAllocations(ctx, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get allocations failed: %s", err.Error())), nil
			}

			health, _ := d.Nomad.GetJobHealth(ctx, name)

			result := map[string]any{
				"name":        name,
				"job_status":  job.Status,
				"allocations": allocs,
			}
			if health != nil {
				result["healthy"] = health.Healthy
				result["health_status"] = health.Status
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}

func sendRCONCommand(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("send_rcon_command",
			mcp.WithDescription("Send an RCON command to a Minecraft server"),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("command", mcp.Required(), mcp.Description("RCON command to execute")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(name); err != nil {
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

func opPlayer(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("op_player",
			mcp.WithDescription("Give operator privileges to a player on a Minecraft server"),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("player", mcp.Required(), mcp.Description("Player username")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			player, err := req.RequireString("player")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !playerNamePattern.MatchString(player) {
				return mcp.NewToolResultError(fmt.Sprintf("invalid player name %q: must match [a-zA-Z0-9_]{1,16}", player)), nil
			}

			response, err := d.Minecraft.ExecuteRCON(ctx, name, "op "+player)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(response), nil
		},
	}
}

func deopPlayer(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("deop_player",
			mcp.WithDescription("Remove operator privileges from a player on a Minecraft server"),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name")),
			mcp.WithString("player", mcp.Required(), mcp.Description("Player username")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("server_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			player, err := req.RequireString("player")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !playerNamePattern.MatchString(player) {
				return mcp.NewToolResultError(fmt.Sprintf("invalid player name %q: must match [a-zA-Z0-9_]{1,16}", player)), nil
			}

			response, err := d.Minecraft.ExecuteRCON(ctx, name, "deop "+player)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(response), nil
		},
	}
}

func backupServer(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("backup_server",
			mcp.WithDescription("Create a backup of a Minecraft server"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Minecraft server name")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := validation.ValidateServerName(name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			backup, err := d.Minecraft.CreateBackup(ctx, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("backup failed: %s", err.Error())), nil
			}
			return mcp.NewToolResultJSON(backup)
		},
	}
}

// deployGenericWorkload gathers reference specs for non-Minecraft workloads.
// Claude generates HCL from the description + references, then calls provision_nomad_workload.
func deployGenericWorkload(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("deploy_generic_workload",
			mcp.WithDescription("Gather reference job specs for deploying a generic (non-Minecraft) workload. Returns existing specs for Claude to adapt. After generating HCL, call provision_nomad_workload."),
			mcp.WithString("description", mcp.Required(), mcp.Description("Description of the workload to deploy")),
			mcp.WithString("reference_job", mcp.Description("Specific job ID to use as reference (optional)")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			description, err := req.RequireString("description")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			referenceJob := req.GetString("reference_job", "")

			result := map[string]any{
				"description":  description,
				"instructions": "Generate a Nomad HCL job spec for this workload. Then call provision_nomad_workload with the HCL and optionally dns_domain/dns_answer for AdGuard DNS rewrite.",
			}

			if referenceJob != "" {
				spec, specErr := d.Nomad.GetJobSpec(ctx, referenceJob)
				if specErr != nil {
					result["reference_warning"] = fmt.Sprintf("could not fetch reference spec %q: %s", referenceJob, specErr.Error())
				} else {
					result["reference_job"] = referenceJob
					result["reference_spec"] = spec
				}
			} else {
				// List available jobs so Claude can pick a reference.
				jobs, listErr := d.Nomad.ListJobs(ctx)
				if listErr != nil {
					result["list_warning"] = fmt.Sprintf("could not list jobs: %s", listErr.Error())
				} else {
					result["available_jobs"] = jobs
				}
			}

			return mcp.NewToolResultJSON(result)
		},
	}
}
