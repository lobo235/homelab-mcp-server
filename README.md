# homelab-mcp-server

MCP server exposing homelab gateway capabilities as AI tools for the Homelab AI Platform.

Part of the [homelab-ai](https://github.com/lobo235/homelab-ai) platform.

## Overview

This server implements the [Model Context Protocol (MCP)](https://modelcontextprotocol.io) and exposes tools that an AI assistant uses to manage Minecraft servers and other containerized workloads in a HashiCorp Nomad cluster. It aggregates capabilities from 6 gateway services into a unified, AI-friendly tool surface.

## Quick Start

```bash
cp .env.example .env
# Fill in required values
go run ./cmd/server
```

## Build, Test, Run

> Go is installed at `~/bin/go/bin/go` (also on `$PATH` via `.bashrc`).

```bash
make build    # Build binary
make test     # Run tests
make lint     # Run linter
make cover    # Coverage report
make run      # Run server
make hooks    # Install pre-commit hook
make clean    # Remove build artifacts
```

## Architecture

The MCP server runs as a subprocess of the chatbot, communicating via stdio transport. It delegates all infrastructure operations to gateway HTTP APIs:

```
chatbot (subprocess) → homelab-mcp-server (stdio) → gateway HTTP APIs
```

### Tool Layers

| Layer | Purpose | Examples |
|-------|---------|---------|
| Layer 1 — Atomic | Single gateway call | `list_running_jobs`, `create_cloudflare_record` |
| Layer 2 — Orchestration | Multi-step with rollback | `provision_minecraft_server`, `destroy_nomad_workload` |
| Layer 3 — High-Level | User intent fulfillment | `create_minecraft_server`, `upgrade_minecraft_server` |

## Configuration

All config via ENV vars. See `.env.example` for the full list.

## Docker

```bash
docker build -t homelab-mcp-server .
docker build --build-arg VERSION=v1.0.0 -t homelab-mcp-server .
```

## License

Private — part of the homelab-ai platform.
