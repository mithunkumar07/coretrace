# CoreTrace Dashboard — Frontend

React 19 + TypeScript SPA for the CoreTrace dashboard. Connects to the backend over REST and WebSocket to display real-time agent status, security events, and SSH sessions.

---

## Tech Stack

| | |
|---|---|
| Framework | React 19.2 |
| Language | TypeScript 5.9 |
| Build | Vite 7.3 |
| Styling | Custom CSS — dark security theme, no UI component library |
| State | React Context (Auth + WebSocket) |
| Linting | ESLint with TypeScript + React hooks plugins |

No external UI library, no routing library, no state management library. The current scope doesn't justify the overhead.

---

## Project Structure

```
frontend/src/
├── main.tsx                    # React root mount
├── App.tsx                     # Layout shell — auth gate, sidebar + view routing
├── App.css                     # Global dark theme styles
├── index.css                   # CSS reset and root variables
│
├── types/
│   └── index.ts                # TypeScript interfaces: Agent, Event, Session, User, etc.
│
├── context/
│   ├── AuthContext.tsx          # JWT token state, login/logout, localStorage persistence
│   └── WebSocketContext.tsx     # WS connection to /ws/dashboard, live event accumulation
│
└── components/
    ├── Login.tsx                # Email + password form
    ├── Sidebar.tsx              # Navigation (Dashboard / Agents / Events / Sessions)
    ├── Dashboard.tsx            # Stats cards + live event table
    ├── Agents.tsx               # Agent list with status badge, auto-refresh
    ├── Events.tsx               # Event log with type/severity filters + live WS merge
    └── Sessions.tsx             # SSH sessions with All / Active toggle
```

---

## Running

```bash
npm install
npm run dev        # http://localhost:5173
npm run build      # production build → dist/
npm run preview    # preview production build
npm run lint       # ESLint
```

---

## Environment Variables

Create `.env.local` in this directory:

```bash
VITE_API_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080
```

Both default to `localhost:8080` if unset.

---

## Authentication

`AuthContext` manages the token lifecycle:

1. `login(email, password)` calls `POST /api/v1/auth/login`
2. Token and user object stored in `localStorage` under `ct_token` / `ct_user`
3. `isAuthenticated` is `true` as long as a token is present
4. `logout()` clears localStorage and resets state

Token expiry is not yet enforced client-side — the backend rejects expired tokens. When a 401 is returned, the user needs to log out and back in. Auto-refresh is planned for Phase 3.

---

## WebSocket

`WebSocketContext` maintains a persistent connection to `/ws/dashboard`:

- Connects on mount, reconnects on close with exponential backoff (1s → 60s max)
- Parses incoming messages that have `type === "event"` or `event_type` set
- Accumulates up to 100 recent events in state (`recentEvents`)
- Exposed via `useWebSocket()` hook — `{ connected, recentEvents }`

**Dashboard** shows the live feed directly from `recentEvents`.
**Events** merges `recentEvents` into the REST-fetched list, deduplicating by ID.

---

## Views

### Dashboard
- Polls `GET /api/v1/stats` every 15s for aggregate counts
- Shows 4 stat cards: total agents, online agents, events in last 24h, active sessions
- Live event table — top 20 from `recentEvents`
- WebSocket status badge (Live / Disconnected)

### Agents
- Fetches `GET /api/v1/agents` on mount and every 10s
- Table: name, hostname, IP, version, status badge, last seen (relative time)
- Manual refresh button

### Events
- Fetches `GET /api/v1/events` with optional `type` and `severity` filters
- Filters re-trigger the fetch immediately
- Live events from WebSocket are merged in at the top (deduped by ID, capped at 200)
- Table: time, type, severity badge, agent ID (truncated), session ID, JSON data preview

### Sessions
- Fetches `GET /api/v1/sessions` or `/active` depending on selected tab
- Auto-refreshes every 10s
- Table: username, source IP, auth method, login time, duration (live-calculated for active), command count, status badge

---

## TypeScript Interfaces

```typescript
interface Agent {
  id: string;
  name: string;
  hostname: string;
  ip_address: string;
  version: string;
  status: 'online' | 'offline' | 'error';
  last_seen: string;
}

interface Event {
  id: string;
  agent_id: string;
  event_type: string;            // ssh_login | ssh_logout | ssh_failed | command | file_*
  timestamp: string;
  severity: 'info' | 'warning' | 'error' | 'critical';
  data: Record<string, any>;
  session_id?: string;
}

interface Session {
  id: string;
  session_id: string;
  username: string;
  source_ip: string;
  auth_method: string;
  login_time: string;
  logout_time?: string;
  command_count: number;
  status: 'active' | 'closed' | 'timeout';
  key_fingerprint?: string;
}

interface DashboardStats {
  total_agents: number;
  online_agents: number;
  total_events_24h: number;
  active_sessions: number;
  last_updated: string;
}
```

---

## Styling

All styles are in `App.css` with a dark security-tool aesthetic:

| Token | Value | Use |
|---|---|---|
| `--bg` | `#0d1117` | Page background |
| `--surface` | `#161b22` | Cards, sidebar |
| `--border` | `#30363d` | Table borders, dividers |
| `--text` | `#e6edf3` | Primary text |
| `--muted` | `#8b949e` | Secondary text, labels |
| `--blue` | `#58a6ff` | Active nav, info badges |
| `--green` | `#3fb950` | Online/success |
| `--amber` | `#d29922` | Warning |
| `--red` | `#f85149` | Error |

No CSS variables defined yet — values are hardcoded in `App.css`. Extracting them to `:root` variables is planned when theming becomes a requirement.

---

## Roadmap

### Phase 3
- Session detail view with full command timeline
- Agent detail view with event history
- Token refresh / session expiry handling
- Pagination on events and sessions

### Phase 4
- Risk score display per agent / session / user
- Anomaly alerts panel
- Compliance evidence export (SOC2, ISO27001)
- Multi-tenant workspace switching
