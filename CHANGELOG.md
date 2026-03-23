# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
