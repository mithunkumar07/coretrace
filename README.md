# CoreTrace

**Infrastructure Runtime Intelligence for Self-Managed Environments**

CoreTrace gives DevOps and security teams real-time visibility into what's happening on their Linux servers, Kubernetes clusters, and cloud VMs — without the complexity or cost of enterprise SIEM/EDR tools.

This is **not** an SSH auditing tool. It's a security observability platform designed around runtime intelligence: what processes are running, what commands are executing, what files are changing, and whether any of it deviates from normal behaviour.

---

## Vision

| Capability | Status |
|---|---|
| SSH session monitoring + attribution | ✅ MVP |
| File integrity monitoring | ✅ MVP |
| Command execution logging (auditd) | ✅ MVP |
| Dashboard — agents, events, sessions | ✅ MVP |
| Command monitoring via eBPF (zero-setup) | 🔄 Phase 2 |
| Runtime process visibility (exec, privilege escalation) | 🔄 Phase 2 |
| Container + Kubernetes awareness | 🔄 Phase 2 |
| SaaS control plane (multi-tenant) | 🔄 Phase 3 |
| Collector protocol (on-prem data plane) | 🔄 Phase 3 |
| Compliance evidence exports (SOC2, ISO27001) | 🔄 Phase 3 |
| Behavioural baselines + anomaly detection | 🔄 Phase 4 |
| Risk scoring (server / user / cluster / org) | 🔄 Phase 4 |
| Cross-layer event correlation (host + container + cloud) | 🔄 Phase 4 |

---

## Architecture

CoreTrace is designed as a **hybrid model**:

```
┌─────────────────────────────────────────────┐
│              Customer Infrastructure         │
│                                              │
│   ┌──────────┐      ┌─────────────────────┐  │
│   │  Agent   │─────▶│  Local Dashboard    │  │
│   │ (Linux)  │  WS  │  (REST + WebSocket) │  │
│   └──────────┘      └─────────────────────┘  │
│        │                      │               │
│        │ (future)             │ (future)      │
│        ▼                      ▼               │
│   ┌──────────────────────────────────────┐   │
│   │         Local Collector              │   │
│   │   (event aggregation + storage)      │   │
│   └──────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
                        │
                        │ metadata + risk summaries only
                        │ (NOT raw logs)
                        ▼
        ┌───────────────────────────────┐
        │     SaaS Control Plane        │
        │  (multi-tenant, coming soon)  │
        │  - policy management          │
        │  - intelligence layer         │
        │  - compliance dashboards      │
        └───────────────────────────────┘
```

**Design principle:** Raw event data stays on the customer's infrastructure. The SaaS layer receives only aggregated metadata, risk summaries, and compliance evidence — not full log streams. This keeps storage costs low and avoids pushing sensitive data off-premise by default.

---

## Repository Structure

```
coretrace/
├── coretrace-agent/          # Go agent — runs on monitored Linux servers
│   ├── cmd/                  # CLI (cobra)
│   ├── internal/
│   │   ├── monitor/          # SSH, file, command monitors
│   │   ├── logger/           # Session-based JSONL logging
│   │   ├── types/            # Event type definitions
│   │   └── dashboard/        # WebSocket client to dashboard
│   ├── config.yaml           # Agent configuration
│   └── deploy/               # Systemd service + Docker
│
├── coretrace-dashboard/      # Web dashboard
│   ├── backend/              # Go API server (Gin + GORM + PostgreSQL)
│   │   └── internal/
│   │       ├── api/          # REST routes + auth
│   │       ├── models/       # GORM models
│   │       ├── database/     # DB init + migrations + seed
│   │       ├── websocket/    # WS hub (agent connections + browser)
│   │       └── config/       # Config from env vars
│   └── frontend/             # React 19 + TypeScript (Vite)
│       └── src/
│           ├── components/   # Dashboard, Agents, Events, Sessions, Login
│           ├── context/      # AuthContext, WebSocketContext
│           └── types/        # TypeScript interfaces
│
├── context.md                # Product strategy and architecture vision
├── opencode_context.md       # Current implementation state + roadmap
└── CLAUDE.md                 # AI assistant context
```

---

## Applications

### [`coretrace-agent`](./coretrace-agent/README.md)
Lightweight Go binary that runs on each monitored Linux server. Monitors SSH sessions, file changes, and command execution. Streams events to the local dashboard via WebSocket.

### [`coretrace-dashboard`](./coretrace-dashboard/README.md)
Web-based control panel. Go REST + WebSocket backend with a React frontend. Receives events from agents in real-time, stores them in PostgreSQL, and presents them in a security-focused UI.

---

## Quick Start

### Run the dashboard stack

```bash
cd coretrace-dashboard
docker-compose up -d
# Dashboard UI: http://localhost:3000
# API: http://localhost:8080
# Default credentials: admin@coretrace.io / admin123
```

### Deploy an agent

```bash
cd coretrace-agent
go build -o coretrace-agent .
sudo ./coretrace-agent monitor --config config.yaml
```

See each app's README for full setup instructions.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Agent | Go 1.24, Cobra, Viper, Zap, fsnotify, lumberjack |
| Dashboard API | Go 1.23, Gin, GORM, PostgreSQL, Gorilla WebSocket |
| Dashboard UI | React 19, TypeScript, Vite |
| Deployment | Docker Compose, Systemd, GitHub Actions |
| Future | eBPF (cilium/ebpf), ClickHouse (log storage) |

---

## Positioning

CoreTrace is not competing with CrowdStrike, Prisma Cloud, or Splunk. It's built for:

- **DevOps teams** who need security visibility without a dedicated security team
- **Self-managed Kubernetes** operators (k3s, bare metal, EC2)
- **SOC2-bound startups** who need compliance evidence without enterprise tooling
- **SMBs** who find SIEM products too heavy, too expensive, or both

The focus is **intelligence over raw log collection** — drift detection, behavioural baselines, and risk scoring rather than shipping terabytes of logs to a cloud bucket.
