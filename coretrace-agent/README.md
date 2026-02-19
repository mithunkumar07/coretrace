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
│   ├── monitor/
│   │   ├── ssh.go         # SSH log monitoring
│   │   ├── file.go        # File system monitoring
│   │   ├── command.go     # Command monitoring interface
│   │   └── command_auditd.go  # Auditd backend
│   └── logger/
│       └── session.go     # Session logging with rotation
├── config.yaml            # Configuration file
├── setup-auditd.sh        # One-time auditd setup
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

## Session Logs

Each SSH session gets its own log file with rotation support:

```
/var/log/coretrace/sessions/
├── 2024-01-15/
│   ├── 192.168.1.100_admin_1705316245.jsonl
│   └── 192.168.1.101_root_1705316300.jsonl
└── orphaned_events.log
```

Session logs contain:
- SSH login/logout events with key fingerprints
- **All commands executed during the session** (requires auditd setup)
- File operations performed
- Session duration and activity summary

### Log Rotation

Configure rotation in `config.yaml`:
```yaml
session_logging:
  rotation:
    max_size_mb: 100      # Rotate at 100MB
    max_backups: 10       # Keep 10 backups
    max_age_days: 30      # Delete after 30 days
    compress: true        # Gzip old files
```

## Command Monitoring Setup

**Important:** The agent works without any setup for SSH and file monitoring. Command logging requires one-time auditd configuration, which can be automated.

### Automated Setup (Recommended)

Run the included setup script that handles everything:

```bash
sudo ./setup-auditd.sh
```

This script:
- Installs auditd if not present
- Configures execve monitoring rules
- Makes rules persistent across reboots
- Starts the auditd service

### What if I can't use auditd?

The agent gracefully handles missing auditd:
- ✅ SSH login/logout tracking works
- ✅ File change monitoring works
- ✅ Session logs are created
- ⚠️ Command logging is disabled (shown in logs)

**For SaaS deployments:** The setup script can be integrated into your deployment automation (Ansible, Puppet, Chef, cloud-init, etc.).

### Manual Setup (if needed)

If you prefer manual configuration:

```bash
# 1. Install auditd
sudo apt-get install -y auditd  # Debian/Ubuntu
sudo yum install -y audit        # RHEL/CentOS

# 2. Add monitoring rules
sudo auditctl -a always,exit -F arch=b64 -S execve -S execveat -k session_monitoring
sudo auditctl -a always,exit -F arch=b32 -S execve -S execveat -k session_monitoring

# 3. Make persistent
sudo tee /etc/audit/rules.d/coretrace.rules << EOF
-a always,exit -F arch=b64 -S execve -S execveat -k session_monitoring
-a always,exit -F arch=b32 -S execve -S execveat -k session_monitoring
EOF
sudo augenrules --load
```

## Session Log Example

```json
{"event_type":"session_start","timestamp":"2024-01-15T10:30:45Z","session_id":"192.168.1.100_admin_1705316245","username":"admin","source_ip":"192.168.1.100","auth_method":"publickey","key_fingerprint":"SHA256:abc123def456...","pid":1234}
{"timestamp":"2024-01-15T10:30:50Z","event_type":"command","session_id":"192.168.1.100_admin_1705316245","username":"admin","command":"ls","args":["ls","-la","/etc"],"working_dir":"/home/admin","pid":5678,"ppid":1234}
{"timestamp":"2024-01-15T10:31:05Z","event_type":"file_change","session_id":"192.168.1.100_admin_1705316245","username":"admin","file_path":"/etc/nginx/nginx.conf","operation":"write"}
{"event_type":"session_end","timestamp":"2024-01-15T10:45:30Z","session_id":"192.168.1.100_admin_1705316245","duration_sec":885,"command_count":15,"file_event_count":3}
```

## Troubleshooting

### Commands not appearing in session logs

1. Check if auditd is running:
```bash
sudo systemctl status auditd
sudo auditctl -l | grep session_monitoring
```

2. Verify audit log is being written:
```bash
sudo tail -f /var/log/audit/audit.log | grep session_monitoring
```

3. Run agent with debug mode to see parsing details:
```bash
sudo ./coretrace-agent monitor --debug
```

### Permission denied errors

The agent requires root to:
- Read `/var/log/auth.log` or `/var/log/secure`
- Read `/var/log/audit/audit.log`
- Monitor file system events
- Access process information in `/proc`

## Product Roadmap

### Phase 1: MVP (Current)
- ✅ SSH session monitoring
- ✅ File integrity monitoring  
- ✅ Session-based logging with rotation
- ✅ Auditd-based command logging

### Phase 2: Zero-Dependency Command Monitoring
**Goal:** Remove auditd requirement using eBPF

- **eBPF-based execve tracing** - No external dependencies, works on modern kernels (4.18+)
- **Graceful fallback** - eBPF → auditd → disabled
- **Self-contained binary** - eBPF bytecode embedded in agent

**Why eBPF?**
- Zero customer setup required
- Better performance than auditd parsing
- Modern standard (Cilium, Pixie, Falco use it)
- Safe and kernel-verified

### Phase 3: SaaS Integration
- **Collector protocol** - Send events to CoreTrace cloud
- **Control plane API** - Policy management from SaaS
- **Compliance dashboards** - SOC2/ISO27001 evidence exports

### Phase 4: Intelligence
- **Behavioral baselines** - Learn normal vs. anomalous activity
- **Risk scoring** - Per-session, per-user, per-server risk metrics
- **Integration ecosystem** - Slack, PagerDuty, SIEM webhooks

## Current Limitations & Workarounds

| Feature | Current | Future |
|---------|---------|--------|
| Command logging | Requires auditd setup | eBPF (no setup) |
| Kernel support | All Linux | 4.18+ for eBPF |
| Architecture | AMD64 | ARM64 support |
| Container-aware | Host only | Container + K8s |