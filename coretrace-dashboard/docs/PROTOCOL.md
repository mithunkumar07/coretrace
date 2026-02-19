# CoreTrace Agent ↔ Dashboard Protocol

## Overview
Real-time bidirectional communication protocol between CoreTrace agents and the dashboard backend.

## Connection

### WebSocket Endpoint
```
ws://dashboard:8080/ws/agents?api_key={AGENT_API_KEY}
```

### Connection Flow
1. Agent generates or loads API key
2. Agent establishes WebSocket connection with API key
3. Dashboard validates API key and associates connection with agent
4. Agent sends initial registration message
5. Bidirectional communication begins

## Message Types

### Agent → Dashboard

#### 1. Registration (on connect)
```json
{
  "type": "register",
  "agent_id": "agent-uuid",
  "hostname": "server-01",
  "ip_address": "192.168.1.100",
  "version": "1.0.0",
  "metadata": {
    "os": "linux",
    "arch": "amd64",
    "kernel": "5.15.0"
  }
}
```

#### 2. Event Streaming (real-time)
```json
{
  "type": "event",
  "timestamp": "2025-02-19T10:30:00Z",
  "event_type": "ssh_login",
  "session_id": "192.168.1.100_user_1739945600",
  "data": {
    "username": "admin",
    "source_ip": "192.168.1.100",
    "auth_method": "publickey",
    "key_fingerprint": "SHA256:abc123...",
    "success": true
  },
  "severity": "info"
}
```

#### 3. Heartbeat (every 30 seconds)
```json
{
  "type": "heartbeat",
  "timestamp": "2025-02-19T10:30:00Z",
  "stats": {
    "active_sessions": 5,
    "events_last_minute": 42,
    "disk_usage_percent": 45
  }
}
```

#### 4. Batch Events (high-throughput mode)
```json
{
  "type": "batch",
  "timestamp": "2025-02-19T10:30:00Z",
  "events": [
    { /* event 1 */ },
    { /* event 2 */ },
    { /* event 3 */ }
  ]
}
```

### Dashboard → Agent

#### 1. Configuration Update
```json
{
  "type": "config_update",
  "timestamp": "2025-02-19T10:30:00Z",
  "config": {
    "logging.level": "debug",
    "ssh.enabled": true,
    "file.watch_paths": ["/etc", "/home"]
  }
}
```

#### 2. Command Execution (for remote management)
```json
{
  "type": "command",
  "command_id": "cmd-uuid",
  "action": "restart",
  "params": {}
}
```

#### 3. Acknowledgment
```json
{
  "type": "ack",
  "message_id": "msg-uuid",
  "status": "received"
}
```

## Event Types

| Event Type | Description | Severity |
|------------|-------------|----------|
| `ssh_login` | Successful SSH login | info |
| `ssh_logout` | SSH session ended | info |
| `ssh_failed` | Failed SSH authentication | warning |
| `command` | Command execution | info |
| `file_change` | File created/modified/deleted | warning |
| `file_access` | File read access | info |
| `session_start` | Session logging started | info |
| `session_end` | Session logging ended | info |

## Reconnection Strategy

### Automatic Reconnection
- **Initial retry**: 1 second
- **Backoff**: Exponential (1s, 2s, 4s, 8s, 16s, 32s)
- **Max retry**: 60 seconds
- **Max attempts**: Unlimited (keep trying forever)

### Reconnection Process
1. Close existing connection
2. Wait for backoff period
3. Attempt new WebSocket connection
4. On success: Send registration message
5. On failure: Increment backoff and retry

## Error Handling

### Connection Errors
- Network unreachable → Retry with backoff
- Authentication failed → Log error, don't retry (manual intervention needed)
- Server error → Retry with backoff
- Rate limited → Wait for Retry-After header

### Message Errors
- Invalid JSON → Log and drop message
- Missing required fields → Log and drop
- Unknown message type → Log and ignore

## Security

### Authentication
- API Key in query parameter or header
- JWT token for dashboard clients
- mTLS optional for production

### Message Integrity
- All messages timestamped
- Sequence numbers for ordering
- Checksums optional for sensitive data

## Performance Considerations

### Bandwidth Optimization
- Use MessagePack for binary serialization (50% smaller than JSON)
- Batch events when rate > 100 events/second
- Compress payloads > 1KB

### Latency
- Target: < 100ms for event delivery
- WebSocket persistent connection (no HTTP overhead)
- Asynchronous message processing

## Scalability

### Horizontal Scaling
- Load balancer with sticky sessions for WebSocket
- Multiple API instances behind load balancer
- Redis for cross-instance message routing

### Agent Discovery
- Agents register on connect with unique ID
- Dashboard maintains agent registry in database
- Automatic cleanup of disconnected agents (after 5 min timeout)
