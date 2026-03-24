# homelab-mcp-server

MCP server exposing homelab gateway capabilities as AI tools for the Homelab AI Platform.
Part of the [homelab-ai](https://github.com/lobo235/homelab-ai) platform.

## Module

`github.com/lobo235/homelab-mcp-server`

## Quick Start

```bash
cp .env.example .env
# Fill in required values
go run ./cmd/server
```

## Build, Test, Run

> Go is installed at `~/bin/go/bin/go` (also on `$PATH` via `.bashrc`).

```bash
# Build
make build

# Run tests
make test

# Run tests with verbose output
go test -v ./...

# Run linter
make lint

# Coverage report (opens in browser)
make cover

# Run the server (requires .env or env vars)
make run

# Build binary
go build -o homelab-mcp-server ./cmd/server
```

## Project Layout

```
homelab-mcp-server/
├── Dockerfile
├── Makefile
├── go.mod / go.sum
├── .env.example              # dev template — never commit real values
├── .gitignore
├── .golangci.yml             # strict linter config
├── .githooks/pre-commit      # runs lint + tests; activate with `make hooks`
├── CLAUDE.md                 # this file
├── README.md
├── CHANGELOG.md
├── cmd/
│   └── server/
│       └── main.go           # entry point — MCP stdio server
└── internal/
    ├── config/
    │   ├── config.go          # ENV var loading & validation
    │   └── config_test.go
    ├── clients/
    │   ├── nomad/             # nomad-gateway HTTP client
    │   ├── adguard/           # adguard-home-gateway HTTP client
    │   ├── cloudflare/        # cloudflare-gateway HTTP client
    │   ├── minecraft/         # minecraft-gateway HTTP client
    │   ├── curseforge/        # curseforge-gateway HTTP client
    │   └── vault/             # vault-gateway HTTP client
    ├── tools/
    │   ├── atomic/            # Layer 1 — single gateway call tools
    │   ├── orchestration/     # Layer 2 — multi-step with rollback
    │   └── highlevel/         # Layer 3 — user intent fulfillment
    ├── validation/            # Job spec pre-flight checks
    ├── prompts/               # MCP prompt resources
    └── resilience/            # Retry policy, circuit breaker
```

## Configuration

All config via ENV vars. Loaded from `.env` in development (via `godotenv`; missing file silently ignored). In production, secrets are injected by the chatbot's Nomad task environment.

| Var | Required | Default | Purpose |
|-----|----------|---------|---------|
| `NOMAD_GATEWAY_URL` | yes | — | Base URL of nomad-gateway |
| `NOMAD_GATEWAY_KEY` | yes | — | API key for nomad-gateway |
| `ADGUARD_GATEWAY_URL` | yes | — | Base URL of adguard-home-gateway |
| `ADGUARD_GATEWAY_KEY` | yes | — | API key for adguard-home-gateway |
| `CF_GATEWAY_URL` | yes | — | Base URL of cloudflare-gateway |
| `CF_GATEWAY_KEY` | yes | — | API key for cloudflare-gateway |
| `MINECRAFT_GATEWAY_URL` | yes | — | Base URL of minecraft-gateway |
| `MINECRAFT_GATEWAY_KEY` | yes | — | API key for minecraft-gateway |
| `CURSEFORGE_GATEWAY_URL` | yes | — | Base URL of curseforge-gateway |
| `CURSEFORGE_GATEWAY_KEY` | yes | — | API key for curseforge-gateway |
| `VAULT_GATEWAY_URL` | yes | — | Base URL of vault-gateway |
| `VAULT_GATEWAY_KEY` | yes | — | API key for vault-gateway |
| `LOG_LEVEL` | no | `info` | Verbosity: `debug`, `info`, `warn`, `error` |
| `NOMAD_DEFAULT_DATACENTER` | yes | — | Default Nomad datacenter for job generation |
| `NOMAD_DEFAULT_NODE_POOL` | no | `default` | Default node pool for MC jobs |
| `NFS_BASE_PATH` | yes | — | NFS base path for Minecraft server volumes |
| `MC_PUBLIC_DOMAIN` | yes | — | Public domain for MC server CNAMEs |
| `MC_PUBLIC_IP` | no | — | Public IP for informational context (not used in operations) |
| `CF_ZONE_NAME` | yes | — | Cloudflare zone name |
| `ARTIFACT_ALLOWLIST` | no | — | Additional artifact source domains (comma-separated) |
| `DATA_DIR` | no | `/data` | Directory for spec cache, itzg docs, and state |
| `ITZG_DOCS_REFRESH_INTERVAL` | no | `24h` | How often to refresh itzg docs cache |

## Architecture

```
cmd/server/main.go               — entry point, wires deps, starts MCP stdio server
internal/config/config.go         — ENV-based config with validation
internal/clients/nomad/           — nomad-gateway HTTP client wrapper
internal/clients/adguard/         — adguard-home-gateway HTTP client wrapper
internal/clients/cloudflare/      — cloudflare-gateway HTTP client wrapper
internal/clients/minecraft/       — minecraft-gateway HTTP client wrapper
internal/clients/curseforge/      — curseforge-gateway HTTP client wrapper
internal/clients/vault/           — vault-gateway HTTP client wrapper
internal/tools/atomic/            — Layer 1 MCP tools (single gateway call each)
internal/tools/orchestration/     — Layer 2 MCP tools (multi-step with rollback)
internal/tools/highlevel/         — Layer 3 MCP tools (user intent fulfillment)
internal/validation/              — Job spec pre-flight validation
internal/prompts/                 — MCP prompt resources (homelab_context, etc.)
internal/resilience/              — Retry policy, circuit breaker, health checks
internal/speccache/               — Nomad job spec cache (auto-seeded on startup)
internal/itzgcache/               — itzg/docker-minecraft-server docs cache
```

## MCP Tools

All tools are registered with mcp-go v0.45.0 and served via stdio transport.

### Layer 1 — Atomic Tools

| Tool | Gateway | Description |
|------|---------|-------------|
| `list_running_jobs` | nomad | List all running Nomad jobs |
| `get_job_spec` | nomad | Get original HCL spec for a job |
| `get_job_status` | nomad | Get job status + allocations |
| `get_job_logs` | nomad | Get allocation logs (requires `task`, optional `log_type`) |
| `submit_nomad_job` | nomad | Submit HCL job spec (with pre-flight validation) |
| `stop_nomad_job` | nomad | Stop/purge a job |
| `restart_nomad_allocation` | nomad | Restart an allocation |
| `watch_job_health` | nomad | Check job health status |
| `create_cloudflare_record` | cloudflare | Create a DNS record |
| `delete_cloudflare_record` | cloudflare | Delete a DNS record |
| `create_local_dns_rewrite` | adguard | Create AdGuard DNS rewrite |
| `delete_local_dns_rewrite` | adguard | Delete AdGuard DNS rewrite |
| `create_server_secret` | vault | Create Minecraft server secrets |
| `delete_server_secret` | vault | Delete Minecraft server secrets |
| `init_server_directory` | minecraft | Initialize server NFS directory |
| `delete_server_directory` | minecraft | Delete server NFS directory |
| `execute_rcon_command` | minecraft | Send RCON command to server |
| `list_backups` | minecraft | List server backups |
| `create_backup` | minecraft | Create server backup |
| `validate_modpack` | curseforge | Validate a CurseForge modpack |
| `get_modpack_files` | curseforge | List modpack file versions |
| `get_modpack_file` | curseforge | Get specific modpack file by file ID |
| `validate_mod` | curseforge | Validate a CurseForge mod |
| `get_mod_file` | curseforge | Get specific mod file by file ID |

### Layer 2 — Orchestration Tools

| Tool | Description |
|------|-------------|
| `provision_minecraft_server` | Init dir -> create secret -> submit job -> create DNS -> wait health |
| `destroy_minecraft_server` | Stop job -> delete DNS -> delete secret -> (optionally) delete dir |
| `provision_nomad_workload` | Submit job -> create AdGuard DNS rewrite |
| `destroy_nomad_workload` | Stop job -> delete AdGuard DNS rewrite |

### Layer 3 — High-Level Task Tools

| Tool | Description |
|------|-------------|
| `create_minecraft_server` | Select reference spec, generate HCL, provision |
| `destroy_minecraft_server_by_name` | Destroy by server name |
| `upgrade_minecraft_server` | Backup -> update spec -> resubmit |
| `get_minecraft_server_status` | Aggregate job state + allocation health |
| `send_rcon_command` | Look up RCON endpoint, execute command |
| `op_player` / `deop_player` | Wrapper around RCON op/deop |
| `backup_server` | Create backup, poll for completion |
| `deploy_generic_workload` | Generate HCL from description, provision |

## Testing Approach

Tests live alongside their packages in `*_test.go` files.

Key patterns:
- Gateway client tests use `httptest.NewServer` to mock gateway HTTP APIs
- Config tests cover all required fields, defaults, and validation
- Validation tests cover all pre-flight check rules
- Table-driven tests for input validation
- Both success and error paths tested

## Naming Convention — Minecraft Servers

Minecraft server names follow a split naming convention:

| Concern | Convention | Example |
|---------|-----------|---------|
| Nomad job ID | `mc-{name}` | `mc-atm9` |
| Vault secret path | `mc-{name}` | `kv/nomad/default/mc-atm9` |
| RCON (via minecraft-gateway) | `mc-{name}` | `mc-atm9` (gateway looks up Nomad allocations) |
| NFS directory | `{name}` | `/minecraft/atm9/` |
| DNS hostname | `{name}.{domain}` | `atm9.example.com` |
| Backups (via minecraft-gateway) | `{name}` | `atm9` |

Tool layers use `validation.MCServerDir(jobID)` to strip the `mc-` prefix for NFS, backup, and DNS operations. RCON passes the full job ID because the minecraft-gateway uses it to query Nomad for the allocation's dynamic RCON port.

## Coding Conventions

- Uses `github.com/mark3labs/mcp-go` v0.45.0 with **stdio transport only** (HTTP/SSE has race condition)
- No external router, ORM, or framework — minimal dependency footprint
- All gateway calls include `Authorization: Bearer <KEY>` and `X-Trace-ID` headers
- All upstream errors wrapped with `fmt.Errorf("context: %w", err)`
- Structured JSON logging via `log/slog` to stderr (stdout reserved for MCP stdio)
- Version logged on startup
- Never log secret values (API keys, passwords, tokens)

## Security Rules

> **Claude must enforce all rules below on every commit and push without exception.**

1. **Never commit secrets:** No `.env`, tokens, API keys, passwords, or credentials of any kind.
2. **Never commit infrastructure identifiers:** No real hostnames, IP addresses, datacenter names, node pool names, Consul service names, Vault paths with real values, Traefik routing rules with real domains, or any value that reveals homelab architecture. Use generic placeholders (`dc1`, `default`, `example.com`, `your-node-pool`, `your-service`).
3. **Unknown files:** If `git status` shows a file Claude didn't create, ask the operator before staging it.
4. **Pre-commit checks (must all pass before committing):**
   - `go test ./...` — all tests must pass
   - `golangci-lint run` — no lint errors
5. **Docs accuracy:** Review all changed `.md` files before committing — documentation must reflect the current state of the code in the same commit.
6. **Version bump:** Before any `git commit`, review the changes and determine the appropriate SemVer bump (MAJOR/MINOR/PATCH). Present the rationale and proposed new version to the operator and wait for confirmation before tagging or referencing the new version.
7. **Push confirmation:** Before any `git push`, show the operator a summary of what will be pushed (commits, branch, remote) and wait for explicit confirmation.
8. **Commit messages:** Must not contain real hostnames, IPs, or infrastructure identifiers.

## Versioning & Releases

SemVer (`MAJOR.MINOR.PATCH`). Git tags are the source of truth.

```bash
git tag v1.2.3 && git push origin v1.2.3
```

This triggers the Docker workflow which publishes:
- `ghcr.io/lobo235/homelab-mcp-server:v1.2.3`
- `ghcr.io/lobo235/homelab-mcp-server:v1.2`
- `ghcr.io/lobo235/homelab-mcp-server:latest`
- `ghcr.io/lobo235/homelab-mcp-server:<short-sha>`

Version is embedded at build time: `-ldflags "-X main.version=v1.2.3"` — defaults to `"dev"` for local builds. Logged on startup.

## Docker

```bash
# Build (version defaults to "dev")
docker build -t homelab-mcp-server .

# Build with explicit version
docker build --build-arg VERSION=v1.2.3 -t homelab-mcp-server .
```

Multi-stage build: `golang:1.26-alpine` -> `alpine:3.21`. Statically compiled (`CGO_ENABLED=0`).

## Known Limitations

- **stdio transport only:** HTTP/SSE transport has an active race condition in mcp-go (issue #763). Pinned to v0.45.0.
- **No standalone Nomad job:** Runs exclusively as a subprocess of the chatbot, not as its own Nomad service.
- **Gateway dependency:** If a gateway is unreachable, its tools are marked unavailable but the MCP server continues running.
