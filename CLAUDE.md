# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A proof-of-concept Go HTTP service that accepts natural language prompts, uses Google Gemini AI to generate HTML/CSS/JS web apps, writes the output to disk, and serves each generated project via a dedicated nginx Docker container fronted by a Traefik reverse proxy.

## Commands

All commands run from `code-generation/`:

```bash
# Start infrastructure (required before running the service)
docker compose -f traefik-compose.yml up -d   # Traefik reverse proxy
docker compose -f docker-compose.yaml up -d    # PostgreSQL

# Run the service
go run .

# Build
go build .

# Live reload (air — uses tmp/ as output dir)
air
```

## Architecture

The codebase is a single `package main` Go module with these files:

- **`main.go`** — wires up Gemini client, Postgres (GORM), Docker client, and Gin routes. DB DSN and Gemini API key are hardcoded here (POC).
- **`genai.go`** — Gemini client (`gemini-2.5-pro`), system prompt constraining output to a JSON array of `{filename, code}` tuples with `.html`/`.css`/`.js` files using Tailwind CDN.
- **`handlers.go`** — three Gin handlers: `POST /action/generate` (calls Gemini, writes files to `~/code-generation/<projectId>/`), `POST /project` (creates DB record + Docker nginx container), `GET /ping`.
- **`container.go`** — Docker operations via moby client: creates and starts an nginx container on the `code_generation` Docker network with Traefik labels routing `<random8chars>.localhost` to the container.
- **`models.go`** — GORM models: `ProjectModel`, `ContainerModel`, `ProjectFileModel`. Schema is auto-migrated on startup.

### Request flow

1. `POST /project` → creates a project dir at `~/code-generation/<uuid>/`, spins up an nginx container mounting that dir, saves to DB, returns `{ projectId, host, containerId, hostPort }`.
2. `POST /action/generate` with `{ projectId, prompt }` → Gemini returns JSON files → written to `~/code-generation/<projectId>/` → nginx serves them immediately at `<host>.localhost`.

### Infrastructure dependencies

- **Traefik** (`traefik-compose.yml`): runs on the `code_generation` Docker network, listens on port 80, routes by hostname label.
- **PostgreSQL** (`docker-compose.yaml`): `postgres:17`, port 5432, password `postgres`.
- All nginx containers must be on the `code_generation` Docker network to be reachable by Traefik.
