# CoreTrace Dashboard - Implementation Summary

## What Was Built

### 1. Dashboard Architecture
```
coretrace-dashboard/
├── backend/               # Go API Server
│   ├── main.go           # Entry point
│   ├── internal/
│   │   ├── api/          # REST API routes
│   │   ├── config/       # Configuration
│   │   ├── database/     # PostgreSQL connection
│   │   ├── models/       # Data models
│   │   └── websocket/    # WebSocket hub
│   ├── Dockerfile
│   └── go.mod
├── frontend/             # React Dashboard
│   ├── Dockerfile
│   └── nginx.conf
├── docker-compose.yml    # Full stack deployment
└── docs/
    ├── ARCHITECTURE.md   # Architecture docs
    └── PROTOCOL.md       # Agent communication protocol
```

### 2. Key Components

#### Backend (Go)
- **REST API**: Agent management, event queries, session tracking
- **WebSocket Hub**: Real-time bidirectional communication
- **Database**: PostgreSQL with GORM ORM
- **Models**: Agent, Event, Session, User

#### Frontend (React + TypeScript)
- **Vite**: Fast development build tool
- **WebSocket Client**: Real-time event streaming
- **Dashboard UI**: Agent list, event viewer, session monitoring

#### Communication Protocol
- **WebSocket**: Persistent connection for real-time events
- **Auto-reconnection**: Exponential backoff (1s to 60s)
- **Batching**: Events batched when rate > 10/second
- **Heartbeat**: Every 30 seconds

### 3. Agent Integration

New package: `internal/dashboard/client.go`

```go
// Initialize dashboard client
dashboardClient := dashboard.NewClient(
    logger,
    "ws://dashboard:8080",
    "your-api-key",
    "agent-001",
)

// Start connection
dashboardClient.Start(ctx)

// Send events
dashboardClient.SendEvent(
    "ssh_login",
    sessionID,
    map[string]interface{}{
        "username": "admin",
        "source_ip": "192.168.1.100",
    },
    "info",
)
```

## How to Deploy

### 1. Start Dashboard

```bash
cd coretrace-dashboard

# Start all services
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f
```

Services:
- Dashboard UI: http://localhost:3000
- API: http://localhost:8080
- PostgreSQL: localhost:5432
- Redis: localhost:6379

### 2. Register an Agent

```bash
# Get API key (create agent)
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production-server-01",
    "hostname": "server-01",
    "ip_address": "192.168.1.100"
  }'

# Response includes api_key - save this!
```

### 3. Configure Agent

Add to `/etc/coretrace/config.yaml`:

```yaml
# Dashboard integration
dashboard:
  enabled: true
  url: "ws://dashboard-server:8080"
  api_key: "ct_abc123xyz789"  # From registration
  reconnect_interval: "5s"
  batch_size: 10
```

### 4. Update Agent Code

Modify `cmd/monitor.go` to use dashboard:

```go
func startMonitoring() {
    // ... existing code ...

    // Initialize dashboard client
    var dashboardClient *dashboard.Client
    if viper.GetBool("dashboard.enabled") {
        dashboardClient = dashboard.NewClient(
            zapLogger,
            viper.GetString("dashboard.url"),
            viper.GetString("dashboard.api_key"),
            viper.GetString("agent.id"),
        )
        if err := dashboardClient.Start(ctx); err != nil {
            zapLogger.Error("Failed to start dashboard client", zap.Error(err))
        }
        defer dashboardClient.Stop()
    }

    // Update event handlers to send to dashboard
    // ...
}
```

## Scalability Features

### 1. Horizontal Scaling
- **Load Balancer**: Sticky sessions for WebSocket
- **Multiple API Instances**: Stateless design
- **Redis**: Cross-instance message routing

### 2. Agent Management
- **Auto-Discovery**: Agents register on connect
- **Health Monitoring**: Heartbeat every 30s
- **Auto-Cleanup**: Disconnected agents after 5 min

### 3. Performance
- **Event Batching**: 10 events per WebSocket message
- **Connection Pooling**: Persistent WebSocket
- **Efficient Serialization**: JSON with optional MessagePack

## API Endpoints

### Agents
```
GET    /api/v1/agents           # List all agents
GET    /api/v1/agents/:id       # Get agent details
POST   /api/v1/agents           # Register new agent
POST   /api/v1/agents/:id/config # Update config
DELETE /api/v1/agents/:id       # Remove agent
```

### Events
```
GET /api/v1/events              # Query events
    ?agent_id=xxx
    ?type=ssh_login
    ?severity=warning
    ?limit=100
    
GET /api/v1/events/stats        # Event statistics
```

### Sessions
```
GET /api/v1/sessions            # List sessions
GET /api/v1/sessions/active     # Active sessions only
GET /api/v1/sessions/:id        # Session details
```

### Dashboard
```
GET /api/v1/stats               # Dashboard statistics
GET /api/v1/health              # Health check
```

## WebSocket Protocol

### Agent → Dashboard
```json
// Registration
{
  "type": "register",
  "agent_id": "agent-001",
  "hostname": "server-01",
  "version": "1.0.0"
}

// Event
{
  "type": "event",
  "timestamp": "2025-02-19T10:30:00Z",
  "event_type": "ssh_login",
  "data": {
    "username": "admin",
    "source_ip": "192.168.1.100"
  }
}

// Heartbeat
{
  "type": "heartbeat",
  "timestamp": "2025-02-19T10:30:00Z",
  "stats": {
    "active_sessions": 5
  }
}
```

### Dashboard → Agent
```json
// Config update
{
  "type": "config_update",
  "config": {
    "logging.level": "debug"
  }
}
```

## Next Steps

### Phase 2 Features (Week 2)
1. **Frontend UI**: Build React components
   - Agent list with status
   - Real-time event feed
   - Session viewer with timeline
   - Charts and metrics

2. **Authentication**: JWT tokens for dashboard users

3. **Alerting**: Rules engine for critical events

### Phase 3 Scale (Week 3)
1. **Kubernetes Deployment**: Helm charts
2. **Data Retention**: Automated cleanup policies
3. **Performance**: ClickHouse for high-volume events

## Testing

```bash
# 1. Start dashboard
docker-compose up -d

# 2. Register agent
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{"name": "test-agent", "hostname": "test"}'

# 3. Check dashboard
open http://localhost:3000

# 4. View API docs
curl http://localhost:8080/api/v1/agents
curl http://localhost:8080/api/v1/stats
```

## Production Checklist

- [ ] Change JWT_SECRET in docker-compose.yml
- [ ] Enable mTLS for agent connections
- [ ] Set up PostgreSQL backups
- [ ] Configure log rotation
- [ ] Set up monitoring (Prometheus/Grafana)
- [ ] Use external Redis cluster
- [ ] Enable API rate limiting
- [ ] Set up SSL/TLS certificates

## Architecture Benefits

1. **Real-time**: WebSocket for instant event visibility
2. **Scalable**: Horizontal scaling with load balancer
3. **Reliable**: Auto-reconnection and event batching
4. **Flexible**: REST API + WebSocket for different use cases
5. **Secure**: API key authentication, optional mTLS
