# CoreTrace Dashboard

The web-based control panel for CoreTrace. Provides real-time visibility into agents, SSH sessions, security events, and system activity across your monitored infrastructure.

The dashboard is the **current control plane** — later it will be split into a customer-hosted local dashboard and a SaaS control plane (see [Architecture](#architecture) below).

---

## Components

```
coretrace-dashboard/
├── backend/     # Go REST + WebSocket API server
└── frontend/    # React 19 + TypeScript SPA
```

---

## Quick Start

### With Docker Compose (recommended)

```bash
cd coretrace-dashboard
docker-compose up -d
```

Services started:
- **Frontend** → http://localhost:3000
- **API** → http://localhost:8080
- **PostgreSQL** → localhost:5432

Default credentials: `admin@coretrace.io` / `admin123`

To override, set environment variables before starting:
```bash
ADMIN_EMAIL=you@example.com ADMIN_PASSWORD=yourpassword docker-compose up -d
```

### Without Docker

**Backend:**
```bash
cd backend
cp .env.example .env    # Edit DATABASE_URL, JWT_SECRET, etc.
go run .
```

**Frontend:**
```bash
cd frontend
npm install
npm run dev             # http://localhost:5173
```

---

## Architecture

The dashboard is currently a **monolithic control plane** that serves both the UI and agent WebSocket connections. The planned evolution:

```
Current (MVP)
─────────────────────────────────────────────
  Browser → React SPA → Backend API (Gin)
  Agent   → WebSocket  → Backend Hub → DB (PostgreSQL)

Phase 3 (SaaS)
─────────────────────────────────────────────
  Browser → SaaS Control Plane (multi-tenant)
              ├── policy management
              ├── intelligence layer
              └── compliance dashboards

  Agent → Local Collector → Local Dashboard (on-prem)
                         └── metadata only → SaaS Control Plane
```

**Key design constraint:** Raw event data stays on the customer's infrastructure. The SaaS layer receives only metadata aggregates and risk summaries — not full log streams. This maintains data sovereignty and avoids costly log ingestion at scale.

---

## Backend

See [`backend/README.md`](./backend/README.md) for full details.

### Tech stack

| | |
|---|---|
| Language | Go 1.23 |
| Framework | Gin |
| ORM | GORM |
| Database | PostgreSQL |
| Real-time | Gorilla WebSocket |
| Config | Environment variables (`.env`) |

### API Reference

**Auth**

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/auth/login` | Login with email + password, returns signed token |

**Agents**

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/agents` | List all registered agents |
| `GET` | `/api/v1/agents/:id` | Get agent by ID |
| `POST` | `/api/v1/agents` | Register a new agent (returns API key) |
| `POST` | `/api/v1/agents/:id/config` | Push config update to agent via WebSocket |
| `DELETE` | `/api/v1/agents/:id` | Remove agent |

**Events**

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/events` | List events (filter: `agent_id`, `type`, `severity`, `limit`) |
| `GET` | `/api/v1/events/stats` | Event counts grouped by type |

**Sessions**

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/sessions` | All sessions, newest first |
| `GET` | `/api/v1/sessions/active` | Active sessions only |
| `GET` | `/api/v1/sessions/:id` | Session by ID |

**Dashboard**

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/stats` | Aggregate counts (agents, events 24h, active sessions) |
| `GET` | `/api/v1/health` | Health check |

**WebSocket**

| Path | Used by | Purpose |
|---|---|---|
| `/ws/agents` | CoreTrace agent | Agent registers + streams events upward |
| `/ws/dashboard` | Browser | Receives live event broadcast |

### WebSocket Protocol

**Agent → Dashboard:**
```json
{
  "type": "event",
  "timestamp": "2025-03-20T10:00:00Z",
  "agent_id": "550e8400-e29b-...",
  "event_type": "ssh_login",
  "severity": "info",
  "session_id": "192.168.1.100_admin_1742428800",
  "data": { "username": "admin", "source_ip": "192.168.1.100" }
}
```

**Dashboard → Agent** (config push):
```json
{
  "type": "config_update",
  "target_agent": "550e8400-e29b-...",
  "config": { "logging.level": "debug" }
}
```

### Data Models

```
Agent       id, name, hostname, ip_address, version, status, last_seen, api_key
Event       id, agent_id, event_type, timestamp, severity, data (JSONB), session_id
Session     id, agent_id, session_id, username, source_ip, auth_method,
            login_time, logout_time, command_count, status, key_fingerprint
User        id, email, password (bcrypt), name, role (admin|operator|viewer)
```

### Environment Variables

```bash
PORT=8080
DATABASE_URL=postgresql://coretrace:coretrace@localhost:5432/coretrace?sslmode=disable
ENVIRONMENT=development          # development | production
JWT_SECRET=change-this-in-prod

# Admin user seed — used only if no users exist in DB
ADMIN_EMAIL=admin@coretrace.io
ADMIN_PASSWORD=admin123
```

---

## Frontend

See [`frontend/README.md`](./frontend/README.md) for full details.

### Tech stack

| | |
|---|---|
| Framework | React 19.2 |
| Language | TypeScript 5.9 |
| Build | Vite 7.3 |
| Styling | Custom CSS (dark security theme, no UI library) |

### Views

| View | Description |
|---|---|
| **Dashboard** | Stats overview (agents, events, sessions) + live event feed via WebSocket |
| **Agents** | All registered agents with status badge and last-seen |
| **Events** | Filterable event log (type + severity) with real-time WS updates |
| **Sessions** | SSH sessions with duration, command count, active/closed status |

### Connecting to the backend

Create `frontend/.env.local`:
```bash
VITE_API_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080
```

---

## Connecting an Agent

1. Register the agent through the dashboard or API:
   ```bash
   curl -X POST http://localhost:8080/api/v1/agents \
     -H "Content-Type: application/json" \
     -d '{"name": "web-01", "hostname": "web-01.example.com"}'
   # Returns: { "id": "...", "api_key": "ct_..." }
   ```

2. Add the dashboard config to the agent's `config.yaml`:
   ```yaml
   dashboard:
     enabled: true
     url: ws://your-dashboard-host:8080
     api_key: ct_...
   ```

3. Restart the agent — it will connect and start streaming events.

---

## Development

```bash
# Run everything
docker-compose up -d

# Watch backend logs
docker-compose logs -f backend

# Frontend hot-reload (outside Docker)
cd frontend && npm run dev

# Rebuild after backend changes
docker-compose up -d --build backend
```

---

## Production Checklist

- [ ] Set a strong `JWT_SECRET` (32+ random chars)
- [ ] Set `ADMIN_PASSWORD` to something non-default before first boot
- [ ] Set `ENVIRONMENT=production` (disables Gin debug output)
- [ ] Use a managed PostgreSQL instance or configure backups
- [ ] Put the API behind HTTPS (nginx/Caddy reverse proxy)
- [ ] Enable mTLS for agent WebSocket connections
- [ ] Manage credentials via secrets manager, not `.env` file

---

## Roadmap

### Phase 3 — SaaS Control Plane
- Multi-tenant isolation (per-org data separation)
- Licensing and seat management
- Policy management API (push monitoring rules to agents)
- Compliance dashboards — SOC2, ISO27001 evidence export
- ClickHouse for high-volume event storage (replaces PostgreSQL for events)

### Phase 4 — Intelligence Layer
- Behavioural baselines per user and per server
- Anomaly detection with configurable sensitivity thresholds
- Risk scoring at session, user, server, cluster, and org levels
- Cross-layer event correlation (host + container + Kubernetes + cloud metadata)
- Alert integrations — Slack, PagerDuty, webhook, SIEM
