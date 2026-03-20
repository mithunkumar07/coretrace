# CoreTrace Dashboard — Backend

Go REST + WebSocket API server. Receives events from CoreTrace agents in real-time, stores them in PostgreSQL, and serves them to the browser dashboard.

---

## Tech Stack

| | |
|---|---|
| Language | Go 1.23 |
| HTTP/WS framework | Gin + Gorilla WebSocket |
| ORM | GORM |
| Database | PostgreSQL (UUID primary keys, JSONB for event data) |
| Password hashing | bcrypt (`golang.org/x/crypto`) |
| Config | Environment variables via `godotenv` |

---

## Project Structure

```
backend/
├── main.go                        # Server entry point — wires everything together
├── internal/
│   ├── config/
│   │   └── config.go              # Loads config from env vars
│   ├── database/
│   │   └── database.go            # PostgreSQL init, GORM auto-migration, admin seed
│   ├── models/
│   │   └── models.go              # Agent, Event, Session, User GORM models
│   ├── api/
│   │   ├── routes.go              # REST route handlers (agents, events, sessions, stats)
│   │   └── auth.go                # POST /api/v1/auth/login, HMAC token signing
│   └── websocket/
│       └── hub.go                 # WS hub — manages agent + browser connections
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
  "token": "eyJ....<base64_payload>.<hmac_sig>",
  "user": {
    "id": "550e8400-...",
    "email": "admin@coretrace.io",
    "name": "Admin",
    "role": "admin"
  }
}
```

Token format: `base64url(payload).base64url(hmac_sha256_sig)`. Payload contains `sub` (user ID) and `exp` (24h expiry). Validated server-side on protected routes.

### Agents

```bash
# List all agents
GET /api/v1/agents

# Get one agent
GET /api/v1/agents/:id

# Register new agent
POST /api/v1/agents
Body: { "name": "web-01", "hostname": "web-01.example.com", "ip_address": "10.0.0.5" }
Returns: agent object with generated api_key

# Push config update to agent
POST /api/v1/agents/:id/config
Body: { "logging.level": "debug" }

# Remove agent
DELETE /api/v1/agents/:id
```

### Events

```bash
# Query events (most recent first, limit 100)
GET /api/v1/events
GET /api/v1/events?type=ssh_login
GET /api/v1/events?severity=warning
GET /api/v1/events?agent_id=<uuid>

# Event type counts
GET /api/v1/events/stats
```

### Sessions

```bash
# All sessions
GET /api/v1/sessions
GET /api/v1/sessions?agent_id=<uuid>

# Active sessions only
GET /api/v1/sessions/active

# Single session
GET /api/v1/sessions/:id
```

### Dashboard stats

```bash
GET /api/v1/stats
# Returns: { total_agents, online_agents, total_events_24h, active_sessions, last_updated }

GET /api/v1/health
# Returns: { status: "healthy", timestamp: <unix> }
```

---

## WebSocket

Two endpoints, each with different client types.

### `/ws/agents` — Agent connections

Agents connect here using an API key query param:
```
ws://host:8080/ws/agents?api_key=ct_xxxx
```

The hub registers the agent and makes its send channel available for targeted messages. Messages sent by the agent are broadcast to all connected browser clients.

### `/ws/dashboard` — Browser connections

The React frontend connects here. It receives all events broadcast from connected agents. Messages sent from the browser with a `target_agent` field are routed to that specific agent.

### Hub behaviour

```
Agent connects → registered in Hub.Clients + Hub.AgentChannels[agent_id]
Agent sends event → Hub.Broadcast → all dashboard browser clients receive it
Browser sends { target_agent: "..." } → Hub.SendToAgent(id, msg)
Agent disconnects → removed from Clients + AgentChannels
```

Ping/pong keepalive runs every 54s (90% of 60s pong timeout). Slow consumers are dropped rather than blocking the hub.

---

## Database

GORM auto-migrates all models on startup (`database.Migrate`). No manual migrations needed during development.

`database.SeedAdmin` runs after migration and creates one admin user if the `users` table is empty. Credentials come from `ADMIN_EMAIL` / `ADMIN_PASSWORD` env vars.

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
    Metadata  JSONB       // arbitrary key-value
    APIKey    string      // unique, returned once at registration
}

Event {
    ID        uuid.UUID
    AgentID   uuid.UUID   // FK → Agent
    EventType string      // ssh_login | ssh_logout | ssh_failed | command | file_create | ...
    Timestamp time.Time
    Severity  string      // info | warning | error | critical
    Data      JSONB       // event-specific fields
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
    LogoutTime     *time.Time  // nil if still active
    CommandCount   int
    Status         string      // active | closed | timeout
    KeyFingerprint string
}

User {
    ID       uuid.UUID
    Email    string      // unique
    Password string      // bcrypt hash, omitted from JSON responses
    Name     string
    Role     string      // admin | operator | viewer
}
```

---

## Known Gaps (to address)

- **Auth middleware** — token validation is not yet enforced on API routes (open in dev, needs middleware for production)
- **`generateRandomString`** — current implementation in `routes.go` is not cryptographically random (uses `time.Now().UnixNano() % charset`); should use `crypto/rand`
- **Agent event ingestion** — agents stream events over WebSocket but there's no handler yet that persists WS events to the DB; currently only REST-ingested events are stored
- **Pagination** — `limit` query param in `/api/v1/events` is parsed but the parse logic is a stub

These are tracked for Phase 2.
