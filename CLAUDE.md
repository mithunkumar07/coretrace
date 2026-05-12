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
│   │   ├── types/                 # events.go — SSHEvent, CommandEvent, FileEvent, DashboardEventMessage
│   │   └── dashboard/             # client.go — WS client to dashboard (wired)
│   └── config.yaml
│
└── coretrace-dashboard/
    ├── backend/                   # Go REST + WebSocket API (Gin + GORM + PostgreSQL)
    │   └── internal/
    │       ├── auth/              # token.go — SignToken, VerifyToken, MakeTokenWithExpiry
    │       ├── api/               # routes.go, auth.go
    │       ├── models/            # models.go — Agent, Event, Session, User
    │       ├── database/          # database.go — init, migrate, SeedAdmin
    │       ├── websocket/         # hub.go — agent + browser WS connections
    │       └── config/            # config.go — env var loader
    └── frontend/                  # React 19 + TypeScript (Vite)
        └── src/
            ├── components/        # Dashboard, Agents, Events, Sessions, Login, Sidebar
            ├── context/           # AuthContext.tsx, WebSocketContext.tsx
            ├── utils/             # api.ts — API_BASE, SEVERITY_CLASS, apiFetch()
            └── types/             # index.ts — Agent, Event, Session, DashboardStats, User
```

---

## Tech Stack

| Layer | Tech |
|-------|------|
| Agent | Go 1.24, Cobra, Viper, Zap, fsnotify, lumberjack, Gorilla WebSocket |
| Dashboard API | Go 1.25, Gin, GORM, PostgreSQL, Gorilla WebSocket, golang.org/x/time/rate |
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

**Dashboard client** (`dashboard/client.go`): persistent WebSocket to dashboard, exponential backoff reconnect (1s–60s), event batching ≥10/sec, 30s heartbeat. **Wired into monitor.go** — all SSH/file/command events are forwarded via `SendEvent()` when `dashboard.enabled = true`. Incoming `type=config` messages from the dashboard are dispatched through `ConfigUpdates()` and applied via `viper.Set()`.

To enable: set `dashboard.enabled: true`, `dashboard.url`, and `dashboard.api_key` in `config.yaml`. The `api_key` is returned when registering the agent via `POST /api/v1/agents`.

---

## Dashboard Architecture

**Backend** (Gin API server):
- REST under `/api/v1/` — agents, events, sessions, stats, health, events/ingest
- Auth: `POST /api/v1/auth/login` → bcrypt verify → HMAC-signed token (logic in `internal/auth` package)
- Admin seed: `database.SeedAdmin()` runs after migration, creates default admin if no users exist (credentials from `ADMIN_EMAIL`/`ADMIN_PASSWORD` env vars, defaults `admin@coretrace.io`/`admin123`)
- WebSocket `/ws/agents` (agents connect here) and `/ws/dashboard` (browser connects here)
- Hub broadcasts all agent messages to all browser clients; browser messages with `target_agent` are routed back to a specific agent
- Rate limiting: per-IP token bucket (`golang.org/x/time/rate`), 60 req/s burst 20, applied globally in `main.go`
- Dashboard stats endpoint runs 4 COUNT queries concurrently via `errgroup`

**Hub behaviour (hub.go):**
- Agent connects → DB lookup by api_key (returns 401/503 on failure) → registered in `Clients` + `AgentChannels`
- Agent sends message → non-blocking enqueue into `persistCh` (bounded 512-slot queue, 4 workers) → broadcast to browsers
- Heartbeat timeout sweep: every 30s, any agent connection silent for >2min has its WS connection closed
- Agent disconnects → status=offline in DB, all `status=active` sessions set to `status=timeout`
- Config update from browser → `POST /api/v1/agents/:id/config` → marshalled + `hub.SendToAgent()` → agent receives and applies via viper

**Frontend** (React SPA):
- `AuthContext` — token in localStorage, `login()`/`logout()`, calls `/api/v1/auth/login`
- `WebSocketContext` — connects to `/ws/dashboard`, exponential backoff reconnect, accumulates last 100 live events
- Views: Dashboard (stats cards + live feed), Agents (table + status badge), Events (filterable + WS merge), Sessions (All/Active toggle)
- Shared utilities in `src/utils/api.ts`: `API_BASE`, `SEVERITY_CLASS`, `apiFetch(url, token)`
- Dark security-tool CSS theme in `App.css` — no UI library

---

## Key Files

| File | Role |
|------|------|
| `coretrace-agent/cmd/monitor.go` | Main monitoring orchestration — wires dashboard client, event handlers |
| `coretrace-agent/internal/types/events.go` | Canonical event structs + DashboardEventMessage |
| `coretrace-agent/internal/logger/session.go` | Session JSONL writer + lumberjack rotation |
| `coretrace-agent/internal/dashboard/client.go` | WS client with reconnect, batching, config dispatch |
| `coretrace-agent/config.yaml` | Full agent config reference (includes dashboard section) |
| `coretrace-dashboard/backend/main.go` | Server wiring — DB, hub, routes, auth, seed, rate limiting |
| `coretrace-dashboard/backend/internal/auth/token.go` | SignToken, VerifyToken, MakeTokenWithExpiry (shared) |
| `coretrace-dashboard/backend/internal/api/routes.go` | REST handlers + ingestEvents |
| `coretrace-dashboard/backend/internal/api/auth.go` | Login endpoint + AuthMiddleware |
| `coretrace-dashboard/backend/internal/database/database.go` | DB init, migrations, SeedAdmin |
| `coretrace-dashboard/backend/internal/websocket/hub.go` | WS hub — bounded persist pool, timeout sweep, session auto-close |
| `coretrace-dashboard/frontend/src/utils/api.ts` | Shared API_BASE, SEVERITY_CLASS, apiFetch |
| `coretrace-dashboard/frontend/src/App.tsx` | App shell — auth gate + view routing |
| `coretrace-dashboard/frontend/src/context/AuthContext.tsx` | Auth state |
| `coretrace-dashboard/frontend/src/context/WebSocketContext.tsx` | Live event stream |
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

## Connecting an Agent

1. Start the dashboard stack: `cd coretrace-dashboard && docker-compose up -d`
2. Register the agent via REST (get back an `api_key`):
   ```bash
   curl -s -X POST http://localhost:8080/api/v1/agents \
     -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     -d '{"name":"web-01","hostname":"web-01.example.com","ip_address":"10.0.0.5"}'
   ```
3. Copy the returned `api_key` (format: `ct_xxxxxxxx...`) into the agent's `config.yaml`:
   ```yaml
   dashboard:
     enabled: true
     url: "http://dashboard-host:8080"
     api_key: "ct_xxxxxxxx..."
   ```
4. Start the agent: `sudo ./coretrace-agent monitor`

The agent connects via WS, sends a registration message, then streams SSH/file/command events. Config updates pushed from the dashboard UI are applied live via viper.

---

## Implementation Status — v2.0.0 (merged to `main`, released 2026-05-12)

### Session 1 — MVP bug fixes

| Bug | Fix |
|-----|-----|
| `generateRandomString` not random | Replaced time-based impl with `crypto/rand.Int` per character |
| `limit` query param stub | `strconv.Atoi` with 1–1000 bounds; applied to GORM query |
| Auth middleware not enforced | `AuthMiddleware` on all non-public routes; frontend sends `Authorization: Bearer`; dashboard WS validates `?token=` |
| WS events not persisted to DB | Hub holds `*gorm.DB`; `persistAgentMessage` writes event/batch to DB; `JSONB.Value`/`Scan` implemented |

### Session 2 — Code quality + Tier 1 hardening

| Item | What was done |
|------|---------------|
| Shared auth package | `internal/auth/token.go` with `SignToken`/`VerifyToken` — eliminates duplication between api and websocket packages |
| nil-DB auth bypass | `ServeAgentWs` returns 503 instead of silently using zero UUID |
| Goroutine explosion | Bounded 4-worker pool + 512-slot `persistCh` replaces unbounded goroutine-per-message |
| Batch insert | `storeBatch` calls `DB.Create(&events)` once (was N individual inserts) |
| uuid.Parse overhead | `Client.AgentID` is now `uuid.UUID`; no parsing in DB methods |
| Sequential stats queries | `getDashboardStats` runs 4 COUNTs concurrently via `errgroup` |
| Heartbeat timeout sweep | `Run()` ticks every 30s; agent connections silent >2min are closed |
| Session auto-close | Agent disconnect → `status=active` sessions → `status=timeout, logout_time=now()` |
| updateAgentConfig wired | Handler marshals config JSON and calls `hub.SendToAgent()`; returns 404 if agent offline |
| REST event ingest | `POST /api/v1/events/ingest` — `X-API-Key` auth, JSON array body, single batch insert |
| Rate limiting | Per-IP `rate.Limiter` in `main.go` (60 req/s, burst 20) |
| Frontend deduplication | `src/utils/api.ts` with `API_BASE`, `SEVERITY_CLASS`, `apiFetch()` shared across all components |

### Session 3 — Agent wiring

| Item | What was done |
|------|---------------|
| gorilla/websocket dep | Added missing dep to agent `go.mod` |
| Config message handling | `client.go`: `configChan`, `ConfigUpdates()`, `waitForDisconnect` parses incoming frames |
| Dashboard client wired | `monitor.go` creates `dashboard.Client` from config, passes to all event handlers |
| Event forwarding | SSH login/logout/failed, file ops, commands all call `dashClient.SendEvent()` |
| Config apply | `applyConfigUpdates` goroutine calls `viper.Set()` for each key in received config payloads |
| config.yaml | Added `dashboard:` section with `enabled/url/api_key/agent_id` |

**Tests:** 36 backend unit tests across `internal/api`, `internal/auth`, `internal/models`, `internal/websocket`. All pass. Integration test: `coretrace-dashboard/test-api.sh`.

---

## Known Gaps

All Tier 1 hardening items are complete. Remaining gaps are Tier 2+ (new architecture):

- **eBPF command monitoring** — `cilium/ebpf` in go.mod, not wired; current monitor requires auditd
- **Process tree tracker** — current `/proc` walk for session correlation is slow and racy; needs live PID cache from eBPF events
- **Container awareness** — events are not tagged with container_id/image when agent runs on a container host
- **Rate limiter eviction** — the per-IP `sync.Map` of limiters is never cleaned up (acceptable for PoC, needs TTL for production)

---

## Roadmap

| Phase | Status | Goal |
|-------|--------|------|
| MVP (v1.x) | ✓ Released | SSH + file + auditd command monitoring, session logging, dashboard UI |
| v2.0.0 | ✓ Released | Security hardening, agent↔dashboard pipeline, Tier 1 fixes, 36 unit tests |
| Phase 2 | Next | eBPF command monitoring, runtime process visibility, container awareness |
| Phase 3 | Planned | SaaS control plane (multi-tenant), on-prem collector, compliance exports |
| Phase 4 | Planned | Behavioural baselines, risk scoring, anomaly detection, alerting |

---

## What's Next — Prioritised Implementation Plan

### Tier 2 — Phase 2 Agent (eBPF + process visibility)

1. **eBPF command monitor** (`agent/internal/monitor/command_ebpf.go`)
   - Use `cilium/ebpf` (already in go.mod) to attach a `kprobe` on `execve`
   - Capture: PID, PPID, UID, command, argv, working directory
   - Wire into fallback chain: eBPF → auditd → disabled (controlled in `config.yaml`)
   - Eliminates the auditd dependency; works in environments without auditd

2. **Process tree tracker** (`agent/internal/monitor/process.go`)
   - Maintain an in-memory `map[int]ProcessInfo` updated via eBPF proc events
   - Walk the tree from any PID up to sshd to find the owning SSH session
   - Replace the current `/proc` walk (slow, racy) with this live cache

3. **Container awareness** (`agent/internal/monitor/container.go`)
   - Detect cgroup namespace: if PID is inside a Docker/containerd cgroup, tag events with `container_id`, `container_name`, `image`
   - Read `/proc/<pid>/cgroup` and cross-reference with the Docker socket or containerd API

### Tier 3 — Phase 3 (SaaS control plane foundation)

4. **Multi-tenant data model**
   - Add `Organisation` and `Tenant` models
   - Scope all `Agent`, `Event`, `Session` queries by `tenant_id`
   - JWT payload extended with `tenant_id` claim

5. **On-prem collector protocol**
   - Define a lightweight gRPC protocol between agent and collector
   - Collector aggregates events, compresses, and ships metadata summaries to SaaS
   - Raw events stay on-prem; SaaS receives only risk hashes and counts

### Tier 4 — Phase 4 (Intelligence layer)

6. **Behavioural baselines**
   - Per-user, per-host sliding window of: command frequency, login hours, source IPs
   - Store as time-bucketed counters in ClickHouse (PostgreSQL for MVP)
   - Deviation from baseline triggers `warning` or `critical` severity event

7. **Risk scoring**
   - Server risk score = weighted sum of recent critical events, failed logins, unusual commands
   - User risk score = cross-server aggregation of user activity
   - Scores stored as JSONB snapshots updated on each new event batch

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
- Auth token format: `base64url(json_payload).base64url(hmac_sha256)` — no external JWT library; shared logic lives in `internal/auth/token.go`
- Dashboard client in agent is nil-safe: all `if dashClient != nil` guards ensure monitoring continues if dashboard is unreachable

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
