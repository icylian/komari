# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Komari is a lightweight, self-hosted server monitoring tool written in Go. It exposes a REST/WebSocket API consumed by a separate frontend repo (`komari-web`) and lightweight agent clients.

## Build & Run

```bash
# Build (frontend assets must be present in public/ first)
go build -trimpath -ldflags="-s -w" -o komari

# Run the server (default port 25774)
./komari server
./komari server -l 0.0.0.0:25774

# Migrate existing SQLite data to MariaDB/MySQL
./komari migrate --src ./data/komari.db \
  --db-host 127.0.0.1 --db-port 3306 \
  --db-user komari --db-pass secret --db-name komari

# Run tests
go test ./...

# Run a single test
go test ./api/ -run TestLogin
```

The frontend is a separate repo (`komari-monitor/komari-web`). CI clones it and builds it before building the Go binary. For local dev without the frontend, the API still works — static file serving just won't serve the dashboard.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `KOMARI_LISTEN` | `0.0.0.0:25774` | Listen address |
| `KOMARI_DB_TYPE` | `sqlite` | `sqlite`, `mysql`, or `mariadb` |
| `KOMARI_DB_FILE` | `./data/komari.db` | SQLite file path |
| `KOMARI_DB_HOST/PORT/USER/PASSWORD/NAME` | — | MySQL connection |
| `KOMARI_ENABLE_CLOUDFLARED` | `false` | Enable Cloudflare tunnel |

## Architecture

**Entry point:** `main.go` → `cmd.Execute()` (Cobra CLI) → `cmd/server.go:RunServer()`

**Request flow:**
- Gin router with three auth tiers:
  - Public routes (`/api/login`, `/api/clients`, `/api/nodes`, etc.)
  - Token-authenticated agent routes (`/api/clients/*` via `TokenAuthMiddleware`)
  - Admin session routes (`/api/admin/*` via `AdminAuthMiddleware`)

**Key packages:**
- `api/` — HTTP handlers; top-level for auth/public, subdirs for `admin/`, `client/`, `record/`, `task/`, `jsonRpc/`, `public/`
- `database/` — GORM models and DB operations; split by domain: `accounts`, `clients`, `records`, `tasks`, `notification`, `auditlog`, `dbcore`
- `config/` — Database-backed key-value config store with pub/sub (`config.Subscribe`) for live reloads
- `utils/` — Cross-cutting: `log/`, `oauth/`, `geoip/`, `notifier/`, `messageSender/`, `cloudflared/`
- `compat/nezha/` — gRPC compatibility layer for Nezha protocol agents
- `ws/` — WebSocket session management for agent terminal connections
- `public/` — Embeds the built frontend static assets

**Scheduled work** (`cmd/server.go:DoScheduledWork`): runs every 30 min to purge old records/task results, and every 60 sec to flush in-memory client reports to DB and check traffic/expiry notifications.

**Config system:** Settings are stored in the DB and accessed via `config.GetAs[T]` / `config.GetManyAs[T]`. Components subscribe to changes via `config.Subscribe` for hot-reload (CORS, GeoIP provider, OAuth provider, notification method, Nezha compat toggle).

**Agent communication:** Agents connect via WebSocket (`/api/clients/report`) or HTTP POST (`/api/clients/report`). In-memory state is held in `api/` and flushed to DB every minute.

## Documentation

- `docs/ARCHITECTURE.md` — Architecture diagrams and flows (Chinese)
- `docs/TODO.md` — TODO list
- `docs/PROGRESS.md` — Development status and session notes