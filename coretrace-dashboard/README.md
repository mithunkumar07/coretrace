# CoreTrace Dashboard

Real-time monitoring dashboard for CoreTrace agents.

## Quick Start

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f backend

# Access dashboard
open http://localhost:3000
```

## Architecture

- **Backend**: Go + Gin + WebSocket
- **Frontend**: React + TypeScript + Vite
- **Database**: PostgreSQL 16
- **Cache**: Redis 7

## API Endpoints

### REST API
- `GET /api/v1/health` - Health check
- `GET /api/v1/agents` - List agents
- `GET /api/v1/events` - Query events
- `GET /api/v1/sessions` - List sessions

### WebSocket
- `/ws/agents` - Agent connections
- `/ws/dashboard` - Dashboard clients

## Development

### Backend
```bash
cd backend
go run main.go
```

### Frontend
```bash
cd frontend
npm install
npm run dev
```

## Agent Configuration

Add to agent config.yaml:
```yaml
dashboard:
  enabled: true
  url: "ws://dashboard:8080"
  api_key: "your-api-key"
```
