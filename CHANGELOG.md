# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- `filesystem` client package for filesystem-gateway (file ops, backups, downloads)
- `create_workload_secret` tool — create generic workload secrets with arbitrary key-value data (admin only)
- `delete_workload_secret` tool — delete generic workload secrets (admin only)
- Generic secret methods in vault client (`CreateGenericSecret`, `GetGenericSecret`, `DeleteGenericSecret`)
- `FILESYSTEM_GATEWAY_URL` and `FILESYSTEM_GATEWAY_KEY` config vars
- `VOLUME_ALLOWLIST` config — comma-separated additional allowed volume mount prefixes beyond `NFS_BASE_PATH`
- `RequireAdmin` authz helper for admin-only tool enforcement
- Prompt context mentions generic workload support and allowed volume prefixes

### Changed
- All filesystem tools (init/delete dir, list/read/write/move/delete files, downloads, backups, archives) now route through filesystem-gateway instead of minecraft-gateway
- `minecraft` client slimmed to RCON only (`ExecuteRCON`, `Ping`)
- `ValidateJobSpec` now accepts multiple volume prefixes (`[]string`) instead of a single path
- `provision_nomad_workload` now requires admin role
- `destroy_nomad_workload` now requires admin role
- `deploy_generic_workload` now requires admin role

### Fixed
- `WriteFile` MCP client now uses `POST` (was incorrectly using `PUT`) to match gateway route
- `RenameServer` MCP client now calls `/migrate` (was incorrectly calling `/rename`) to match gateway route

## [v1.9.1] - 2026-03-26

### Fixed
- `CreateBackup` and `MoveFile` now pass uid:gid (1001:1001) to minecraft-gateway so backup dirs, archive files, and moved file parents are owned by the minecraft user instead of root

## [v1.9.0] - 2026-03-26

### Changed
- `destroy_minecraft_server` now runs asynchronously — returns immediately and performs cleanup (stop job, delete DNS, delete secret, delete directory) in a background goroutine to avoid breaking SSE connections

### Added
- `get_destroy_status` tool — check progress of an async server destruction (steps completed, errors, terminal status)

## [v1.8.1] - 2026-03-26

### Fixed
- Server pack detection no longer relies solely on `ServerPackFileID` — now also checks the `IsServerPack` flag and filename heuristics ("server" in filename) across version-matched files, catching untagged server packs that many modpack authors upload as additional files
- Deleting a KB entry now also removes its `.discovery` state file, so re-discovery starts fresh instead of finding stale state

## [v1.8.0] - 2026-03-26

### Added
- Modpack discovery pipeline — automated 7-stage async pipeline (resolve → download → extract → analyze → API enrich → web search → finalize) that auto-populates KB entries for unknown modpacks
- `trigger_modpack_discovery` MCP tool — starts async discovery for a modpack slug
- `get_discovery_state` MCP tool — polls discovery pipeline progress
- Modrinth API client (`internal/clients/modrinth/`) — search projects, get versions, get project details
- FTB API client (`internal/clients/ftb/`) — search packs, get pack/version details from api.modpacks.ch
- Format-specific analyzers for CurseForge manifest.json, Modrinth modrinth.index.json, and FTB API-based packs
- Mod intelligence database — client-only mod detection, extra port mods, heavy mods, world-gen mods
- Anthropic web search enrichment — uses claude-sonnet with web_search tool to find deployment guides
- Resource sizing heuristics — estimates RAM/CPU from mod count and heavy mod detection
- Java version inference from Minecraft version
- Discovery state tracking with `.discovery/<slug>.state.json` files
- 20+ new fields on ModpackKnowledge schema (source_platform, modrinth_id, ftb_id, jvm_args, gc_strategy, startup_method, level_type, additional_ports, confidence_flags, needs_review, etc.)
- `SearchModpackBySlug` method on CurseForge client
- Integration tests with mock servers for all three platform formats
- Research tool (`cmd/research/main.go`) for real-world pipeline validation

### Changed
- Web search now uses `claude-sonnet-4-6` model with 16384 max_tokens (up from 4096) to prevent response truncation
- Web search system prompt rewritten with explicit JSON-only output rules for reliable parsing
- `ConfigOverrides` type changed from `map[string]map[string]string` to `map[string]map[string]any` to handle mixed value types from LLM responses
- Downloader now tries server pack first, falls back to client pack (refactored into `downloadFile` helper)

### Fixed
- Web search retry logic: exponential backoff, rate-limit retry-after header support, truncation detection with adaptive `max_uses` reduction
- Web search rate limiting: 90-second minimum interval between API calls to stay within Anthropic API token limits
- Web search response parsing: extract JSON from responses with preamble text or markdown fences
- CurseForge mod enrichment: concurrent API calls (10-parallel, 500 cap) to resolve FileName/Slug from projectID/fileID, enabling mod intelligence (client-only detection, heavy mods)
- Mod intelligence re-runs after API enrichment to pick up newly resolved slugs/filenames
- Recursive manifest search (max depth 3) with server-pack directory fallback when manifest.json not found
- Security: zip bomb protection with per-file (1GB) and total (5GB) extraction limits
- Security: zip slip prevention with `..` path component rejection
- Security: HTTPS-only download enforcement
- Security: io.LimitReader bounds on all HTTP response reads (50MB for APIs, 2GB for downloads)
- Security: slug validation regex prevents path traversal in KB and discovery state files
- Security: sensitive keyword filtering in startup script parsing (passwords, tokens, keys)
- Security: restrictive temp directory permissions (0o700)

## [v1.5.1] - 2026-03-24

### Fixed
- Guard spec cache write against empty specs to prevent clobbering existing cached HCL
- Fix `list_archive_contents` client calling wrong endpoint URL (was 404ing)
- Add cache-busting timestamp to GitHub raw URL fetches in `fetch_artifact`

## [v1.4.1] - 2026-03-24

### Added
- `move_server_file` and `delete_server_file` MCP tools for file management on server filesystem
- RCON command blocklist: `stop`, `save-off`, `ban-ip` blocked at MCP level with clear error messages

## [v1.4.0] - 2026-03-24

### Added
- `download_to_server` MCP tool — download modpacks, mods, and configs from CurseForge/Modrinth/FTB to server filesystem
- `list_archive_contents` MCP tool — inspect zip/tar archives on server filesystem before extraction
- `list_server_files` MCP tool — browse server directory structure
- `read_server_file` MCP tool — read config files and logs from server filesystem
- `write_server_file` MCP tool — write config files to server filesystem
- `fetch_artifact` MCP tool — fetch trusted scripts from allowlisted GitHub URLs
- `search_mods` MCP tool — search CurseForge for mods by name
- Log grep parameter on `get_job_logs` for server-side log filtering

## [v1.3.3] - 2026-03-24

### Added
- Pre-flight validation checks datacenter and node_pool against expected values — catches misconfigured HCL before submission to Nomad

## [v1.3.2] - 2026-03-24

### Fixed
- `get_minecraft_server_status` no longer calls the blocking `watch_job_health` endpoint (up to 5-min timeout) — derives health from allocation ClientStatus instead, preventing connection timeouts when checking unhealthy servers

## [v1.3.1] - 2026-03-24

### Added
- `search_mods` MCP tool — search CurseForge for mods by name

## [v1.3.0] - 2026-03-24

### Added
- `search_modpacks` MCP tool — search CurseForge for modpacks by name instead of requiring project IDs

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
