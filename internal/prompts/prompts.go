// Package prompts provides MCP prompt resources that give Claude context about the homelab.
package prompts

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Config holds the configuration values used in prompts.
type Config struct {
	NomadDefaultDatacenter string
	NomadDefaultNodePool   string
	NFSBasePath            string
	VolumeAllowlist        []string
	MCPublicDomain         string
	MCPublicIP             string
	CFZoneName             string
}

// Register adds all MCP prompts to the server.
func Register(s *server.MCPServer, cfg *Config) {
	s.AddPrompt(homelabContext(cfg))
	s.AddPrompt(minecraftServerSizing())
	s.AddPrompt(serverNamingConvention())
}

func homelabContext(cfg *Config) (mcp.Prompt, server.PromptHandlerFunc) {
	return mcp.NewPrompt("homelab_context",
			mcp.WithPromptDescription("Describes the Nomad/Consul/Traefik/mc-router/NFS homelab setup, DNS zones, node pools, and naming conventions"),
		), func(_ context.Context, _ mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			content := fmt.Sprintf(`# Homelab Infrastructure Context

## Container Orchestration
- **Scheduler:** HashiCorp Nomad
- **Default Datacenter:** %s
- **Default Node Pool:** %s
- **Service Discovery:** HashiCorp Consul (services register via tags)

## Networking
- **Reverse Proxy:** Traefik (handles TLS for web services, discovers backends via Consul)
- **Minecraft Proxy:** mc-router (routes Java players to correct server by hostname via Consul service tags)
- **DNS:**
  - Cloudflare DNS for public-facing Minecraft servers (CNAME records pointing to %s)
  - AdGuard Home for internal homelab services (DNS rewrites)

## Storage
- **Persistent Storage:** NFS volumes mounted at %s/<server-name>/
- **Volume layout for modpack servers:**
  - data/ — world, server.properties, ops.json, usercache.json
  - data/backups/ — world backups
  - modpacks/ — modpack files
  - mods/ — individual mods
  - downloads/ — server pack ZIPs
  - config/ — extra config files
  - plugins/ — server plugins
- **Volume layout for vanilla servers:**
  - data/ — all data
  - data/backups/ — world backups

## Minecraft Servers
- Container image: itzg/minecraft-server
- Public domain: %s
- RCON passwords stored in Vault via vault-gateway
- Players connect via: <server-name>.%s

## Secrets
- HashiCorp Vault stores RCON passwords, API tokens, and gateway keys
- vault-gateway handles all Vault interactions via AppRole auth
- RCON passwords are auto-generated (32 chars, [a-zA-Z0-9_.-])
- Generic workload secrets can be created via create_workload_secret (admin-only)

## DNS Zones
- Cloudflare zone: %s (for Minecraft server CNAMEs)
- AdGuard Home: local DNS rewrites for internal services

## Generic Workloads (Admin Only)
- Admins can deploy non-Minecraft workloads using deploy_generic_workload and provision_nomad_workload
- Volume mounts are allowed under these prefixes: %v
- Generic secrets are stored in Vault under category-based paths (e.g., gitea/gitea, postgres/mydb)
- DNS for internal services is managed via AdGuard Home rewrites
`, cfg.NomadDefaultDatacenter, cfg.NomadDefaultNodePool,
				cfg.MCPublicDomain, cfg.NFSBasePath,
				cfg.MCPublicDomain, cfg.MCPublicDomain, cfg.CFZoneName,
				cfg.VolumeAllowlist)

			return mcp.NewGetPromptResult(
				"Homelab infrastructure context for AI assistant",
				[]mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleAssistant, mcp.NewTextContent(content)),
				},
			), nil
		}
}

func minecraftServerSizing() (mcp.Prompt, server.PromptHandlerFunc) {
	return mcp.NewPrompt("minecraft_server_sizing",
			mcp.WithPromptDescription("Resource allocation guidance for Minecraft server modpack types"),
		), func(_ context.Context, _ mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			content := `# Minecraft Server Resource Sizing Guide

| Server Type | CPU (MHz) | Memory (MB) | Notes |
|-------------|-----------|-------------|-------|
| Vanilla Java | 0 (no limit) | 2560 | MAX_MEMORY=2G; simple /data volume only |
| Light modpack (e.g., ATM9-TTS) | 12000 | 12288 | Through The Seasons variant |
| Heavy modpack (e.g., ATM10) | 21000 | 9216 | INIT_MEMORY=4G, MAX_MEMORY=8G |
| Heaviest modpack (e.g., ATM9) | 28000 | 16384 | Use as upper bound |

## Notes
- CPU=0 means Nomad does not set a CPU hard limit (uses available capacity)
- Memory values are the Nomad task memory limit (container gets this + a small buffer)
- Set INIT_MEMORY and MAX_MEMORY JVM flags in the container env vars
- Use the closest matching production job as the resource baseline
- When unsure, start with the heavy modpack values and adjust down`

			return mcp.NewGetPromptResult(
				"Minecraft server resource sizing guide",
				[]mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleAssistant, mcp.NewTextContent(content)),
				},
			), nil
		}
}

func serverNamingConvention() (mcp.Prompt, server.PromptHandlerFunc) {
	return mcp.NewPrompt("server_naming_convention",
			mcp.WithPromptDescription("How to derive job names and hostnames from a server description"),
		), func(_ context.Context, _ mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			content := `# Server Naming Convention

## Rules
- Job names must match: ^[a-z0-9][a-z0-9-]{1,48}[a-z0-9]$
- Lowercase alphanumeric with hyphens only
- 3-50 characters total
- Must start and end with alphanumeric character
- No underscores, dots, or spaces

## Minecraft Server Naming Pattern
- Prefix: "mc-"
- Followed by: abbreviated modpack name + optional instance number
- Examples:
  - mc-atm10 (All The Mods 10)
  - mc-atm9 (All The Mods 9)
  - mc-vanilla1 (Vanilla server #1)
  - mc-ftb-revelation (FTB Revelation)
  - mc-atm10-kids (ATM10 for kids)

## Generic Workload Naming
- Use descriptive lowercase-hyphenated names
- Examples: grafana, prometheus, traefik-dashboard

## Hostname Derivation
- Minecraft servers: <job-name>.<MC_PUBLIC_DOMAIN>
- Internal services: <job-name>.<internal-domain>`

			return mcp.NewGetPromptResult(
				"Server naming convention guide",
				[]mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleAssistant, mcp.NewTextContent(content)),
				},
			), nil
		}
}
