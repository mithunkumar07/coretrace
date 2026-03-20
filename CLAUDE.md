# CoreTrace — CLAUDE.md

## What This Is

CoreTrace is an **Infrastructure Runtime Intelligence** platform — a lightweight, modern alternative to heavy SIEM/EDR tools targeting DevOps-heavy teams on self-managed Linux infrastructure.

**This is not an SSH auditing tool.** The vision is runtime intelligence: process visibility, behavioural baselines, risk scoring, cross-layer event correlation, and compliance automation. SSH + file + command monitoring is the MVP foundation, not the product.

**Positioning:** Security observability for self-managed infra. Not competing with CrowdStrike, Prisma Cloud, or Splunk. Built for DevOps-heavy startups, self-managed Kubernetes operators, and SOC2-bound SMBs.

**Core principle:** Intelligence over raw log collection. Raw event data stays on the customer's infrastructure. The SaaS layer receives metadata aggregates and risk summaries only — not full log streams.

---

## Monorepo Layout

```
coretrace/
├── coretrace-agent/               # Go agent — runs on monitored Linux servers
│   ├── cmd/                       # CLI (cobra): root.go, monitor.go
│   ├── internal/
│   │   ├── monitor/               # ssh.go, file.go, command.go, command_auditd.go
│   │   ├── logger/                # session.go — JSONL session log writer + rotation
│   │   ├── types/                 # events.go — SSHEvent, CommandEvent, FileEvent
│   │   └── dashboard/             # client.go — WS client to dashboard
│   └── config.yaml
│
└── coretrace-dashboard/
    ├── backend/                   # Go REST + WebSocket API (Gin + GORM + PostgreSQL)
    │   └── internal/
    │       ├── api/               # routes.go, auth.go
    │       ├── models/            # models.go — Agent, Event, Session, User
    │       ├── database/          # database.go — init, migrate, SeedAdmin
    │       ├── websocket/         # hub.go — agent + browser WS connections
    │       └── config/            # config.go — env var loader
    └── frontend/                  # React 19 + TypeScript (Vite)
        └── src/
            ├── components/        # Dashboard, Agents, Events, Sessions, Login, Sidebar
            ├── context/           # AuthContext.tsx, WebSocketContext.tsx
            └── types/             # index.ts — Agent, Event, Session, DashboardStats, User
```

---

## Tech Stack

| Layer | Tech |
|-------|------|
| Agent | Go 1.24, Cobra, Viper, Zap, fsnotify, lumberjack |
| Dashboard API | Go 1.23, Gin, GORM, PostgreSQL, Gorilla WebSocket |
| Dashboard UI | React 19.2, TypeScript 5.9, Vite 7.3 |
| Auth | bcrypt passwords, HMAC-SHA256 signed tokens (no JWT library) |
| Deploy | Docker Compose, Systemd, GitHub Actions (AMD64 + ARM64 releases) |
| Future | cilium/ebpf (in go.mod, not yet wired), ClickHouse (log storage at scale) |

---

## Agent Architecture

Single static Go binary, runs privileged on Linux. Three **independent** monitoring modules — a failure in one never affects the others:

1. **SSH Monitor** (`monitor/ssh.go`) — tails auth log, regex-parses sshd events, tracks sessions by PID
2. **File Monitor** (`monitor/file.go`) — fsnotify watcher on /etc, /home, /root, /var/log, /opt, /tmp
3. **Command Monitor** (`monitor/command_auditd.go`) — tails auditd log, parses execve records; gracefully disabled if auditd absent

**Session correlation:** SSH login records sshd PID → other monitors walk `/proc` PID tree to find parent sshd → events tagged with `session_id`. No matching session → `orphaned_events.log`.

**Session logs:** JSONL files under `/var/log/coretrace/sessions/YYYY-MM-DD/`, one file per session. lumberjack handles rotation (size, age, compression, retention — all configurable in `config.yaml`).

**Dashboard client** (`dashboard/client.go`): persistent WebSocket to dashboard, exponential backoff reconnect (1s–60s), event batching >10/sec, 30s heartbeat.

---

## Dashboard Architecture

**Backend** (Gin API server):
- REST under `/api/v1/` — agents, events, sessions, stats, health
- Auth: `POST /api/v1/auth/login` → bcrypt verify → HMAC-signed token
- Admin seed: `database.SeedAdmin()` runs after migration, creates default admin if no users exist (credentials from `ADMIN_EMAIL`/`ADMIN_PASSWORD` env vars, defaults `admin@coretrace.io`/`admin123`)
- WebSocket `/ws/agents` (agents connect here) and `/ws/dashboard` (browser connects here)
- Hub broadcasts all agent messages to all browser clients; browser messages with `target_agent` are routed back to a specific agent

**Frontend** (React SPA):
- `AuthContext` — token in localStorage, `login()`/`logout()`, calls `/api/v1/auth/login`
- `WebSocketContext` — connects to `/ws/dashboard`, exponential backoff reconnect, accumulates last 100 live events
- Views: Dashboard (stats cards + live feed), Agents (table + status badge), Events (filterable + WS merge), Sessions (All/Active toggle)
- Dark security-tool CSS theme in `App.css` — no UI library

---

## Key Files

| File | Role |
|------|------|
| `coretrace-agent/cmd/monitor.go` | Main monitoring orchestration loop |
| `coretrace-agent/internal/types/events.go` | Canonical event structs |
| `coretrace-agent/internal/logger/session.go` | Session JSONL writer + lumberjack rotation |
| `coretrace-agent/internal/dashboard/client.go` | WS client with reconnect + batching |
| `coretrace-agent/config.yaml` | Full agent config reference |
| `coretrace-dashboard/backend/main.go` | Server wiring — DB, hub, routes, auth, seed |
| `coretrace-dashboard/backend/internal/api/routes.go` | REST handlers |
| `coretrace-dashboard/backend/internal/api/auth.go` | Login endpoint + HMAC token signing |
| `coretrace-dashboard/backend/internal/database/database.go` | DB init, migrations, SeedAdmin |
| `coretrace-dashboard/backend/internal/websocket/hub.go` | WS hub — client registry + message routing |
| `coretrace-dashboard/frontend/src/App.tsx` | App shell — auth gate + view routing |
| `coretrace-dashboard/frontend/src/context/AuthContext.tsx` | Auth state |
| `coretrace-dashboard/frontend/src/context/WebSocketContext.tsx` | Live event stream |
| `coretrace-dashboard/frontend/src/types/index.ts` | All TypeScript interfaces |
| `.github/workflows/release.yml` | CI: AMD64 + ARM64 binary builds + GitHub release |

---

## Build

```bash
# Agent — single static binary, no CGO
cd coretrace-agent
go build -o coretrace-agent .
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o coretrace-agent-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o coretrace-agent-linux-arm64 .

# Dashboard — full stack via Docker Compose
cd coretrace-dashboard
docker-compose up -d
# Frontend: http://localhost:3000  API: http://localhost:8080
# Default login: admin@coretrace.io / admin123
```

---

## Known Gaps (tracked for Phase 2)

- **WS events not persisted to DB** — agents stream events over WebSocket but the hub only broadcasts them to browser clients; there's no handler writing them to the `events` table. Only REST-ingested events are stored.
- **Auth middleware not enforced** — API routes are currently open; token validation is not checked on REST endpoints. Needs middleware before production.
- **`generateRandomString` is not random** — `routes.go:251` uses `time.Now().UnixNano() % charset` producing the same character repeated. Should use `crypto/rand`.
- **`limit` query param is a stub** — parsed in `getEvents` but not applied.

---

## Roadmap

| Phase | Status | Goal |
|-------|--------|------|
| MVP | Done | SSH + file + auditd command monitoring, session logging, dashboard UI |
| Phase 2 | Next | eBPF command monitoring, runtime process visibility, container awareness |
| Phase 3 | Planned | SaaS control plane (multi-tenant), on-prem collector, compliance exports |
| Phase 4 | Planned | Behavioural baselines, risk scoring, anomaly detection, alerting |

**eBPF strategy:** Fallback chain `eBPF → auditd → disabled`. `cilium/ebpf` already in `go.mod`, not yet wired.

**Storage evolution:** PostgreSQL is fine for MVP. Phase 3 will evaluate ClickHouse or similar for high-volume event storage at scale.

---

## Conventions

- Agent modules fail independently — never let one monitor crash the whole process
- Event channels are buffered (1000 events) to absorb bursts
- No mocks for system integrations — test against real log files/auditd output
- Agent runs as root — intentional, required for log access and file monitoring
- JSONL for all session log files
- No external UI library on the frontend — CSS is custom, keep it that way unless there's a strong reason
- Auth token format: `base64url(json_payload).base64url(hmac_sha256)` — no external JWT library

---

## Docs

| File | Purpose |
|------|---------|
| `context.md` | Product strategy prompt — vision, architecture, positioning, target customers |
| `opencode_context.md` | Current implementation state and roadmap detail |
| `README.md` | Monorepo overview with architecture diagram |
| `coretrace-agent/README.md` | Agent user docs — build, config, deployment, troubleshooting |
| `coretrace-dashboard/README.md` | Dashboard overview — API reference, WS protocol, data models |
| `coretrace-dashboard/backend/README.md` | Backend internals — all routes, hub behaviour, known gaps |
| `coretrace-dashboard/frontend/README.md` | Frontend internals — views, contexts, TypeScript interfaces |
