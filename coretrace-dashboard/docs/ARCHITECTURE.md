# CoreTrace Dashboard Architecture

## Overview
Scalable real-time dashboard for monitoring CoreTrace agents across infrastructure.

## Architecture Components

```
┌─────────────────┐     WebSocket/HTTP     ┌──────────────────┐
│   CoreTrace     │◄──────────────────────►│  Dashboard API   │
│     Agent       │    (Events + Config)   │   (Go Backend)   │
│   (per server)  │                        └────────┬─────────┘
└─────────────────┘                                 │
      ▲                                             │
      │                                             │
      │                    ┌──────────────────┐    │
      │                    │    PostgreSQL    │    │
      └────────────────────┤   (Events DB)    │◄───┘
                           └──────────────────┘
                                    │
                           ┌────────▼─────────┐
                           │   Redis Cache    │
                           │  (Real-time)     │
                           └──────────────────┘
                                    │
                           ┌────────▼─────────┐
                           │  React Frontend  │
                           │  (Dashboard UI)  │
                           └──────────────────┘
```

## Data Flow

### 1. Agent → Dashboard (Real-time Events)
- WebSocket connection for streaming events
- Automatic reconnection with exponential backoff
- Heartbeat/ping every 30 seconds

### 2. Dashboard → Agent (Configuration)
- REST API for config updates
- Agent polls for config changes (every 60s)
- Or WebSocket push for immediate updates

### 3. Event Storage
- Hot data (last 24h): Redis + In-memory
- Warm data (7 days): PostgreSQL
- Cold data (30+ days): Compressed files/S3

## Scalability Design

### Horizontal Scaling
- Stateless API servers (can run multiple instances)
- Load balancer (nginx/traefik)
- Database read replicas for queries

### Agent Management
- Auto-discovery via registration token
- Agent groups (by environment, region, etc.)
- Bulk configuration updates

### Real-time Updates
- WebSocket per agent connection
- Event broadcasting to connected dashboards
- Efficient binary protocol (MessagePack)

## Security

### Authentication
- JWT tokens for dashboard users
- API keys for agents
- mTLS optional for agent connections

### Authorization
- Role-based access (admin, viewer, operator)
- Agent-level permissions
- Audit logging

## API Design

### REST Endpoints
```
GET  /api/v1/agents              # List all agents
GET  /api/v1/agents/:id          # Get agent details
POST /api/v1/agents/:id/config   # Update agent config
GET  /api/v1/events              # Query events (with filters)
GET  /api/v1/sessions            # List active sessions
GET  /api/v1/metrics             # System metrics
```

### WebSocket Events
```json
// Agent → Dashboard
{
  "type": "event",
  "agent_id": "agent-001",
  "timestamp": "2025-02-19T10:30:00Z",
  "data": {
    "event_type": "ssh_login",
    "username": "admin",
    "source_ip": "192.168.1.100"
  }
}

// Dashboard → Agent (config update)
{
  "type": "config_update",
  "config": {
    "logging.level": "debug"
  }
}
```

## Technology Stack

### Backend
- **Language**: Go 1.21+
- **Web Framework**: Gin or Echo
- **WebSocket**: gorilla/websocket
- **Database**: PostgreSQL 14+
- **Cache**: Redis 7+
- **Message Queue**: Redis Pub/Sub (or NATS for scale)

### Frontend
- **Framework**: React 18 + TypeScript
- **State Management**: Zustand or Redux Toolkit
- **UI Library**: Tailwind CSS + Headless UI
- **Charts**: Recharts or Chart.js
- **Real-time**: Socket.io client

### Infrastructure
- **Container**: Docker + Docker Compose
- **Reverse Proxy**: Traefik or Nginx
- **Monitoring**: Prometheus + Grafana

## Database Schema

### agents
- id (UUID, PK)
- name (string)
- hostname (string)
- ip_address (string)
- version (string)
- status (enum: online, offline, error)
- last_seen (timestamp)
- created_at (timestamp)
- metadata (JSONB)

### events
- id (UUID, PK)
- agent_id (UUID, FK)
- event_type (enum)
- timestamp (timestamp)
- data (JSONB)
- severity (enum: info, warning, error, critical)

### sessions
- id (UUID, PK)
- agent_id (UUID, FK)
- username (string)
- source_ip (string)
- login_time (timestamp)
- logout_time (timestamp)
- command_count (int)
- status (enum: active, closed, timeout)

## Development Plan

### Phase 1: MVP (Week 1)
- [x] Basic API server
- [x] Agent WebSocket connection
- [x] Simple event storage
- [x] Basic dashboard UI

### Phase 2: Features (Week 2)
- [ ] Agent authentication
- [ ] Event filtering/search
- [ ] Real-time charts
- [ ] Alerting rules

### Phase 3: Scale (Week 3)
- [ ] Horizontal scaling
- [ ] Data retention policies
- [ ] Performance optimization
- [ ] Production deployment

## Deployment

### Development
```bash
cd coretrace-dashboard
docker-compose up
```

### Production
```bash
# Kubernetes deployment
kubectl apply -f k8s/
```
