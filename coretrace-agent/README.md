# CoreTrace SSH Session Monitoring Agent

A lightweight Go agent for monitoring SSH sessions, login attempts, command execution, and file activities in real-time.

## Features

- **SSH Session Monitoring**: Detect successful/failed SSH logins with source IP and geolocation
- **Command Logging**: Track commands executed within SSH sessions
- **File Activity Monitoring**: Monitor file access, creation, modification, and deletion
- **Session Correlation**: Correlate all events to specific SSH sessions
- **Key Fingerprinting**: Identify SSH keys used for authentication
- **Real-time Processing**: Event-driven architecture with low overhead

## Architecture

```
├── main.go                 # Entry point
├── cmd/
│   ├── root.go            # CLI root command
│   └── monitor.go         # Monitor command implementation
├── internal/
│   ├── types/
│   │   └── events.go      # Event type definitions
│   └── monitor/
│       ├── ssh.go         # SSH log monitoring
│       └── file.go        # File system monitoring
├── config.yaml            # Configuration file
└── coretrace-agent        # Compiled binary
```

## Usage

### Build
```bash
go build -o coretrace-agent .
```

### Run Monitor
```bash
sudo ./coretrace-agent monitor
```

### Run with Debug
```bash
sudo ./coretrace-agent monitor --debug
```

### Use Custom Config
```bash
sudo ./coretrace-agent monitor --config /path/to/config.yaml
```

## Monitoring Capabilities

### SSH Events
- Successful SSH logins (username, IP, auth method, key fingerprint)
- Failed SSH login attempts
- SSH logout events
- Real-time geolocation lookup

### File Events
- File creation, modification, deletion
- Permission changes
- Directory monitoring with recursive watch
- Configurable include/exclude patterns

### Session Correlation
- All events linked to specific SSH sessions
- PID-based process tracking
- User attribution for file operations

## Configuration

Edit `config.yaml` to customize:
- Log paths for SSH monitoring
- File paths to watch
- Event buffer sizes
- Geolocation settings
- Performance limits

## Requirements

- Linux system
- Root privileges (for log access and file monitoring)
- Go 1.21+ (for building)

## Output Format

Events are logged in structured JSON format:
```json
{
  "timestamp": "2024-01-15T10:30:45Z",
  "event_type": "ssh_login",
  "session_id": "192.168.1.100_admin_1705316245",
  "username": "admin",
  "source_ip": "192.168.1.100",
  "location": {
    "country": "Unknown",
    "city": "Unknown"
  },
  "auth_method": "publickey",
  "success": true
}
```

## Next Steps

To complete the full implementation:

1. **Command Logging**: Implement auditd/eBPF integration for command tracking
2. **Session Correlation**: Build session state management across event types
3. **Geolocation**: Add IP geolocation lookup service integration
4. **Key Fingerprinting**: Parse SSH key fingerprints from auth logs
5. **Output Integration**: Add support for sending events to downstream collectors