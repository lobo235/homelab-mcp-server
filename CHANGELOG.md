# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [v1.2.4] - 2026-03-24

### Fixed
- URL-encode query parameters in `GetAllocationLogs`, `ListFiles`, and `ReadFile` to prevent query parameter injection

## [v1.2.3] - 2026-03-24

### Removed
- `send_rcon_command` highlevel tool (duplicate of atomic `execute_rcon_command`)
- `op_player` / `deop_player` highlevel tools (duplicate of atomic `execute_rcon_command` with op/deop commands)
- `backup_server` highlevel tool (duplicate of atomic `create_backup`)

## [v1.2.2] - 2026-03-24

### Fixed
- Job submission sends raw HCL to nomad-gateway instead of JSON-wrapping it — fixes "No argument or block type named hcl" parse errors
- Allocation ID validation accepts UUID prefixes (min 8 hex chars) for compatibility with truncated IDs from tool output

### Changed
- `get_job_logs` and `restart_nomad_allocation` tool descriptions now instruct Claude to call `get_job_status` first to obtain a live allocation ID

## [v1.2.1] - 2026-03-24

### Added
- `MCServerDir()` naming convention helper: Nomad jobs use `mc-` prefix, NFS directories and DNS hostnames use bare name (e.g., job `mc-atm9` → dir `atm9`, hostname `atm9.<domain>`)

### Fixed
- Allocation struct JSON tags changed from snake_case to PascalCase to match nomad-gateway's Nomad API native format — fixes empty allocation fields in `get_job_status` and `get_minecraft_server_status`
- RCON tools (`execute_rcon_command`, `send_rcon_command`, `op_player`, `deop_player`) now pass full job ID to minecraft-gateway instead of stripping `mc-` prefix — fixes 404 errors for RCON operations
- NFS/DNS/backup operations correctly strip `mc-` prefix from job IDs
- Pre-existing goimports formatting issues in curseforge and nomad clients

## [v1.1.0] - 2026-03-23

### Added
- `get_modpack_file` and `get_mod_file` MCP tools for single-file lookup by file ID
- `serverPackFileId` field in CurseForge file responses — enables discovering and fetching server pack files
- `DoText` method on base HTTP client for plain-text gateway responses

### Changed
- Docker build workflow resolves version from git tags for non-tag builds
- `GetAllocationLogs` returns plain text instead of JSON — matches nomad-gateway's `text/plain` response
- Minecraft backup types aligned with gateway response shapes (`BackupInfo` for list, `BackupStatus` for create/get)
- `ListServers` returns `[]string` unwrapped from `{"servers": [...]}` envelope (was `[]Server`)
- `ListFiles` unwraps `{"files": [...]}` envelope and accepts `subPath` parameter
- `ReadFile` parses `{"content": "..."}` JSON envelope and returns string (was raw bytes)
- `FileEntry` now includes `ModTime` field to match gateway response

### Fixed
- `get_job_logs` tool no longer fails with JSON decode error — nomad-gateway returns plain text, not JSON
- `list_backups` tool now correctly unwraps `{"backups": [...]}` envelope from minecraft-gateway
- `create_backup` tool now parses `{"server", "backup_id", "status"}` response from minecraft-gateway
- `ListServers` now correctly unwraps `{"servers": [...]}` envelope from minecraft-gateway (was failing with JSON decode error)
- `ListFiles` now correctly unwraps `{"files": [...]}` envelope from minecraft-gateway (was failing with JSON decode error)
- `ReadFile` now correctly parses `{"content": "..."}` JSON response (was returning raw JSON bytes)

## [v1.0.3] - 2026-03-23

### Fixed
- CurseForge client `ModpackFile` struct JSON tags now match gateway response format (camelCase, not snake_case) — fixes empty displayName, fileName, and gameVersions in file listings
- `ModpackFile.GameVersions` is now `[]string` to match the gateway's array response (was `string`)

## [v1.0.2] - 2026-03-23

### Fixed
- `get_job_spec` tool now correctly parses the `Source` field from nomad-gateway response (was reading wrong JSON field name, returning empty specs)
- `get_job_logs` tool now passes required `task` parameter to nomad-gateway logs endpoint (was returning 400 `missing_param` error)
- `get_job_logs` tool accepts optional `log_type` parameter (stdout/stderr)

## [v1.0.1] - 2026-03-23

### Changed
- `MC_PUBLIC_IP` is now optional (informational only, not used in operations)

### Fixed
- Make volume mount prefix configurable via `NFS_BASE_PATH` instead of hardcoding paths in job spec validation

## [v1.0.0] - 2026-03-23

### Fixed

- InitServer now calls `POST /servers` with JSON body (`name`, `uid`, `gid`) instead of `POST /servers/{name}` with nil body, matching the minecraft-gateway API
- DeleteServer now includes `?confirm=true` query parameter required by the minecraft-gateway API

### Added

- Initial project scaffold: go.mod, cmd/server/main.go, internal/ layout
- MCP server using mcp-go v0.45.0 with stdio transport
- Configuration loading for all 22 env vars with validation and tests
- 6 gateway HTTP client wrappers (nomad, adguard, cloudflare, minecraft, curseforge, vault)
- Base HTTP client with Bearer auth, X-Trace-ID propagation, and structured error parsing
- Client tests using httptest for nomad-gateway and vault-gateway
- Layer 1 atomic MCP tools: 22 tools wrapping individual gateway calls
- Layer 2 orchestration MCP tools: provision/destroy minecraft server (with rollback), provision/destroy nomad workload
- Layer 3 high-level MCP tools: create/destroy/upgrade minecraft server, deploy generic workload, get server status, send RCON, op/deop player, backup server
- Job spec pre-flight validation (required fields, security rules, naming, resource limits)
- Validation tests covering all pre-flight check rules
- MCP prompts: homelab_context, minecraft_server_sizing, server_naming_convention
- Retry policy (3 retries, exponential backoff from 100ms, jitter +/-25ms)
- Circuit breaker (5 failures in 30s opens for 60s)
- Startup health checks for all 6 gateways
- Makefile with standard targets (build, test, cover, lint, run, hooks, clean)
- Dockerfile (multi-stage: golang:1.26-alpine -> alpine:3.21)
- .golangci.yml with strict linter config
- Spec cache: on-disk caching of Nomad job specs, auto-seeded from nomad-gateway on startup
- itzg docs cache: background goroutine fetches and refreshes itzg/docker-minecraft-server docs
- Pre-commit hook (lint + test)

### Changed

- Provisioning order: DNS first to maximize propagation time (DNS → secret → directory → job → health)

### Fixed

- Server name validation at MCP tool boundary — all tools now validate names against `^[a-z0-9][a-z0-9-]{0,47}$` before gateway calls (prevents path traversal)
- Job ID and allocation ID validation in read operations (UUID pattern for alloc IDs)
- HCL validation: `privileged` and `network_mode` now reject variable interpolation (e.g. `var.priv`) — only safe literals accepted
- Retry and circuit breaker wired into all gateway clients via `clients.Base`
- Fixed `_ = append(steps, "job")` ineffectual assignment in orchestration provisioning
