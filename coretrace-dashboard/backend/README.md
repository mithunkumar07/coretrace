# CoreTrace Dashboard — Backend

Go REST + WebSocket API server. Receives events from CoreTrace agents in real-time, stores them in PostgreSQL, and serves them to the browser dashboard.

---

## Tech Stack

| | |
|---|---|
| Language | Go 1.25 |
| HTTP/WS framework | Gin + Gorilla WebSocket |
| ORM | GORM |
| Database | PostgreSQL (UUID primary keys, JSONB for event data) |
| Password hashing | bcrypt (`golang.org/x/crypto`) |
| Rate limiting | `golang.org/x/time/rate` (per-IP token bucket) |
| Config | Environment variables via `godotenv` |

---

## Project Structure

```
backend/
├── main.go                        # Server entry point — wires DB, hub, routes, rate limiter
├── internal/
│   ├── auth/
│   │   └── token.go               # SignToken, VerifyToken, MakeTokenWithExpiry (shared)
│   ├── config/
│   │   └── config.go              # Loads config from env vars
│   ├── database/
│   │   └── database.go            # PostgreSQL init, GORM auto-migration, admin seed
│   ├── models/
│   │   └── models.go              # Agent, Event, Session, User GORM models + JSONB
│   ├── api/
│   │   ├── routes.go              # REST route handlers (agents, events, sessions, stats, ingest)
│   │   └── auth.go                # POST /api/v1/auth/login, AuthMiddleware
│   └── websocket/
│       └── hub.go                 # WS hub — bounded persist pool, timeout sweep, session auto-close
├── .env.example                   # Environment variable reference
└── Dockerfile
```

---

## Running

**With Go:**
```bash
cp .env.example .env
# Edit .env — set DATABASE_URL and JWT_SECRET at minimum
go run .
```

**With Docker:**
```bash
docker build -t coretrace-backend .
docker run -p 8080:8080 --env-file .env coretrace-backend
```

---

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Listen port |
| `DATABASE_URL` | `postgresql://coretrace:coretrace@localhost:5432/coretrace?sslmode=disable` | PostgreSQL connection string |
| `ENVIRONMENT` | `development` | `development` or `production` (production enables Gin release mode) |
| `JWT_SECRET` | `your-secret-key-change-in-production` | Secret for HMAC token signing — **change this** |
| `ADMIN_EMAIL` | `admin@coretrace.io` | Seeded admin email (only used if no users exist in DB) |
| `ADMIN_PASSWORD` | `admin123` | Seeded admin password — **change this before first boot** |

---

## API

### Auth

#### `POST /api/v1/auth/login`

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@coretrace.io", "password": "admin123"}'
```

Response:
```json
{
  "token": "<base64url_payload>.<base64url_hmac_sig>",
  "user": { "id": "...", "email": "admin@coretrace.io", "name": "Admin", "role": "admin" }
}
```

Token format: `base64url(payload).base64url(hmac_sha256_sig)`. Payload has `sub` (user ID) and `exp` (24h Unix timestamp). Logic lives in `internal/auth/token.go` and is shared by the REST middleware and WS token validation.

**All API routes except `/health`, `/auth/login`, and `/events/ingest` require:**
```
Authorization: Bearer <token>
```

The dashboard WebSocket requires the token as a query param (browsers cannot set WS headers):
```
ws://host:8080/ws/dashboard?token=<token>
```

### Agents

```bash
# List all agents
GET /api/v1/agents

# Get one agent
GET /api/v1/agents/:id

# Register new agent — returns api_key (copy to agent config.yaml)
POST /api/v1/agents
Body: { "name": "web-01", "hostname": "web-01.example.com", "ip_address": "10.0.0.5" }

# Push config update to a connected agent (returns 404 if agent is not connected)
POST /api/v1/agents/:id/config
Body: { "logging.level": "debug" }

# Remove agent
DELETE /api/v1/agents/:id
```

### Events

```bash
# Query events (most recent first, default limit 100, max 1000)
GET /api/v1/events
GET /api/v1/events?type=ssh_login
GET /api/v1/events?severity=warning
GET /api/v1/events?agent_id=<uuid>
GET /api/v1/events?limit=500

# Event type counts
GET /api/v1/events/stats

# Ingest events via HTTP (for agents where persistent WS is not viable)
POST /api/v1/events/ingest
Header: X-API-Key: ct_xxxx
Body: [{ "event_type": "ssh_login", "severity": "info", "data": {...} }, ...]
Response: { "stored": <count> }
```

### Sessions

```bash
GET /api/v1/sessions
GET /api/v1/sessions?agent_id=<uuid>
GET /api/v1/sessions/active
GET /api/v1/sessions/:id
```

### Dashboard stats

```bash
GET /api/v1/stats
# Returns: { total_agents, online_agents, total_events_24h, active_sessions, last_updated }
# Note: 4 COUNT queries run concurrently via errgroup

GET /api/v1/health
# Returns: { status: "healthy", timestamp: <unix> }
```

---

## WebSocket

### `/ws/agents` — Agent connections

```
ws://host:8080/ws/agents?api_key=ct_xxxx
```

The hub validates the API key against the DB (returns 401 if invalid, 503 if DB unavailable). On connect, agent status is set to `online`. The agent send channel is registered in `AgentChannels` for targeted routing.

### `/ws/dashboard` — Browser connections

```
ws://host:8080/ws/dashboard?token=<jwt_token>
```

The React frontend connects here. It receives all events broadcast from connected agents. Messages sent from the browser with a `target_agent` field are routed to that specific agent.

### Hub behaviour

```
Agent connects    → DB lookup by api_key → status=online → registered in Clients + AgentChannels
Agent sends msg   → non-blocking enqueue into persistCh (512 slots, 4 workers) → broadcast to browsers
Browser sends msg → { target_agent: "..." } → SendToAgent(id, msg) → agent receives + applies config
Agent disconnects → status=offline in DB, active sessions → status=timeout
Sweep (every 30s) → agent connections silent >2min are closed (stale connection detection)
```

Ping/pong keepalive runs every 54s (90% of 60s pong timeout). Slow browser consumers are dropped rather than blocking the hub. Persist workers drop messages rather than blocking if the queue is full.

---

## Database

GORM auto-migrates all models on startup (`database.Migrate`). No manual migrations needed during development.

`database.SeedAdmin` runs after migration and creates one admin user if the `users` table is empty.

### Models

```go
Agent {
    ID        uuid.UUID   // PK
    Name      string
    Hostname  string
    IPAddress string
    Version   string
    Status    string      // online | offline | error
    LastSeen  time.Time
    Metadata  JSONB
    APIKey    string      // unique; returned once at registration
}

Event {
    ID        uuid.UUID
    AgentID   uuid.UUID   // FK → Agent
    EventType string      // ssh_login | ssh_logout | ssh_failed | command | file_<op> | ...
    Timestamp time.Time
    Severity  string      // info | warning | error | critical
    Data      JSONB
    SessionID string
}

Session {
    ID             uuid.UUID
    AgentID        uuid.UUID
    SessionID      string      // unique: ip_user_timestamp
    Username       string
    SourceIP       string
    AuthMethod     string      // publickey | password
    LoginTime      time.Time
    LogoutTime     *time.Time  // nil if active; set to now() on agent disconnect (status=timeout)
    CommandCount   int
    Status         string      // active | closed | timeout
    KeyFingerprint string
}

User {
    ID       uuid.UUID
    Email    string      // unique
    Password string      // bcrypt hash, omitted from JSON
    Name     string
    Role     string      // admin | operator | viewer
}
```

---

## Testing

### Unit tests

```bash
cd backend
go test ./internal/... -v
```

36 tests across 4 packages:
- `internal/auth` — SignToken/VerifyToken round-trips, wrong secret, expired, malformed, tampered payload
- `internal/api` — AuthMiddleware (6 cases), generateRandomString (4 cases), generateAPIKey (3 cases)
- `internal/models` — JSONB Value/Scan round-trips (9 cases)
- `internal/websocket` — NewHub initialization, persist queue capacity, SendToAgent, buildEvent (4 cases)

### Integration tests

With the stack running (`docker compose up -d` from `coretrace-dashboard/`):

```bash
cd coretrace-dashboard
./test-api.sh                      # default http://localhost:8080
./test-api.sh http://my-host:8080  # custom target
```

Covers: health, login, invalid credentials, auth guard (5 endpoints), `?limit` parsing, agent CRUD + key uniqueness, sessions/active, events/stats.

---

## Known Gaps

All MVP and Tier 1 hardening gaps are resolved. Remaining items are Tier 2+ (new architecture):

- **Rate limiter eviction** — `sync.Map` of per-IP limiters is never cleaned up; acceptable for PoC, needs TTL-based eviction for production
- **eBPF command monitoring** — agent currently requires auditd; `cilium/ebpf` in go.mod but not wired
- **Multi-tenancy** — all data is single-tenant; scoping by `tenant_id` is a Phase 3 item
