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
		getMinecraftServerStatus(d),
		sendRCONCommand(d),
		opPlayer(d),
		deopPlayer(d),
		backupServer(d),
	)
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

			backup, err := d.Minecraft.CreateBackup(ctx, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("backup failed: %s", err.Error())), nil
			}
			return mcp.NewToolResultJSON(backup)
		},
	}
}
