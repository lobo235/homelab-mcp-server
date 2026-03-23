# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed
- Docker build workflow resolves version from git tags for non-tag builds

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
