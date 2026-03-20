# CoreTrace Agent

A lightweight Go agent for Infrastructure Runtime Intelligence on Linux servers.

The agent is the **data plane component** that runs on each monitored host. It captures runtime events — SSH sessions, command execution, file changes — correlates them into sessions, and streams them to the CoreTrace dashboard.

---

## What it monitors

| Module | What it captures | How |
|---|---|---|
| **SSH** | Login/logout/failed attempts, username, source IP, auth method, key fingerprint | Tails `/var/log/auth.log` |
| **File Integrity** | Create, modify, delete, chmod on watched paths | fsnotify (kernel inotify) |
| **Commands** | Every command executed during an SSH session — binary, args, working directory, PID | auditd (MVP) → eBPF (Phase 2) |

All three modules run independently. A failure in one does not affect the others.

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                  coretrace-agent                    │
│                                                     │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────┐  │
│  │ SSH Monitor │  │ File Monitor │  │  Command  │  │
│  │  (ssh.go)  │  │  (file.go)   │  │  Monitor  │  │
│  └──────┬──────┘  └──────┬───────┘  └─────┬─────┘  │
│         │                │                │         │
│         └────────────────┴────────────────┘         │
│                          │                          │
│                  Session Correlator                 │
│               (PID tree walk → session_id)          │
│                          │                          │
│            ┌─────────────┴──────────────┐           │
│            │                            │           │
│     Session Logger                Dashboard Client  │
│  (/var/log/coretrace/sessions/)    (WebSocket)      │
└─────────────────────────────────────────────────────┘
```

**Session correlation:** When an SSH login is detected, the sshd PID is recorded. File and command monitors walk the `/proc` PID tree to find the parent sshd process and tag events with the matching `session_id`. Events with no matching session go to `orphaned_events.log`.

---

## Project Structure

```
coretrace-agent/
├── main.go                           # Entry point
├── cmd/
│   ├── root.go                       # CLI root (cobra)
│   └── monitor.go                    # `monitor` command — orchestrates all modules
├── internal/
│   ├── types/
│   │   └── events.go                 # SSHEvent, CommandEvent, FileEvent structs
│   ├── monitor/
│   │   ├── ssh.go                    # Auth log parser (regex), session tracking
│   │   ├── file.go                   # fsnotify watcher for configured paths
│   │   ├── command.go                # CommandMonitor interface
│   │   └── command_auditd.go         # Auditd log parser implementation
│   ├── logger/
│   │   └── session.go                # Per-session JSONL files + lumberjack rotation
│   └── dashboard/
│       └── client.go                 # WebSocket client, reconnect, event batching
├── config.yaml                       # Full configuration reference
├── setup-auditd.sh                   # One-time auditd setup script
└── deploy/
    ├── coretrace.service             # Systemd unit file
    ├── Dockerfile
    └── docker-compose.yml
```

---

## Build

Requires Go 1.21+. No CGO — produces a single static binary.

```bash
cd coretrace-agent
go build -o coretrace-agent .
```

**Cross-compile for Linux:**
```bash
# AMD64
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o coretrace-agent-linux-amd64 .

# ARM64 (Raspberry Pi, Graviton)
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o coretrace-agent-linux-arm64 .
```

Binary size is ~15MB unstripped, ~10MB stripped (`-ldflags="-s -w"`).

---

## Run

The agent requires **root** to read system logs and monitor file system events.

```bash
# Basic run
sudo ./coretrace-agent monitor

# With debug logging
sudo ./coretrace-agent monitor --debug

# Custom config path
sudo ./coretrace-agent monitor --config /etc/coretrace/config.yaml
```

---

## Configuration

The full reference is in `config.yaml`. Key sections:

```yaml
# SSH monitoring
ssh:
  auth_log: /var/log/auth.log         # Path to auth log (Ubuntu/Debian)
  # auth_log: /var/log/secure         # RHEL/CentOS
  session_timeout: 3600               # Seconds before inactive session is closed
  max_sessions: 1000                  # Max tracked sessions in memory

# File integrity monitoring
file_monitoring:
  watch_paths:
    - /etc
    - /home
    - /root
    - /var/log
    - /opt
    - /tmp
  exclude_patterns:
    - "*.swp"
    - ".git/*"

# Command monitoring
command_monitoring:
  backend: auditd                     # auditd | ebpf (Phase 2) | disabled
  auditd_log: /var/log/audit/audit.log

# Session log rotation
session_logging:
  base_path: /var/log/coretrace/sessions
  rotation:
    max_size_mb: 100
    max_backups: 10
    max_age_days: 30
    compress: true

# Dashboard connection
dashboard:
  url: ws://localhost:8080
  api_key: ""                         # Set after registering agent in dashboard
  reconnect_interval: 5s
```

---

## Session Logs

Each SSH session gets its own JSONL log file, organised by date:

```
/var/log/coretrace/sessions/
├── 2025-03-20/
│   ├── 192.168.1.100_admin_1742428800.jsonl
│   ├── 10.0.0.5_deploy_1742428850.jsonl
│   └── ...
└── orphaned_events.log               # Events with no matching session
```

Each file contains the full session timeline in order:

```jsonl
{"event_type":"session_start","timestamp":"2025-03-20T10:00:00Z","session_id":"192.168.1.100_admin_1742428800","username":"admin","source_ip":"192.168.1.100","auth_method":"publickey","key_fingerprint":"SHA256:abc123...","pid":1234}
{"event_type":"command","timestamp":"2025-03-20T10:00:05Z","session_id":"192.168.1.100_admin_1742428800","command":"cat","args":["cat","/etc/passwd"],"working_dir":"/home/admin","pid":5678,"ppid":1234}
{"event_type":"file_change","timestamp":"2025-03-20T10:00:12Z","session_id":"192.168.1.100_admin_1742428800","file_path":"/etc/nginx/nginx.conf","operation":"write"}
{"event_type":"session_end","timestamp":"2025-03-20T10:14:35Z","session_id":"192.168.1.100_admin_1742428800","duration_sec":875,"command_count":18,"file_event_count":3}
```

---

## Command Monitoring Setup

SSH and file monitoring work with no setup. Command logging requires auditd.

### Automated (recommended)

```bash
sudo ./setup-auditd.sh
```

This installs auditd if needed, adds persistent execve monitoring rules, and starts the service.

### Manual

```bash
# Install auditd
sudo apt-get install -y auditd          # Debian/Ubuntu
sudo yum install -y audit               # RHEL/CentOS

# Add rules
sudo auditctl -a always,exit -F arch=b64 -S execve -S execveat -k coretrace
sudo auditctl -a always,exit -F arch=b32 -S execve -S execveat -k coretrace

# Make persistent
echo "-a always,exit -F arch=b64 -S execve -S execveat -k coretrace" | \
  sudo tee /etc/audit/rules.d/coretrace.rules
sudo augenrules --load
```

### Without auditd

The agent degrades gracefully:

| Capability | Without auditd |
|---|---|
| SSH login/logout tracking | ✅ Works |
| File integrity monitoring | ✅ Works |
| Session logs created | ✅ Works |
| Command logging | ❌ Disabled (logged at startup) |

---

## Production Deployment

### Systemd service

```bash
sudo cp coretrace-agent /usr/local/bin/
sudo cp deploy/coretrace.service /etc/systemd/system/
sudo mkdir -p /etc/coretrace
sudo cp config.yaml /etc/coretrace/config.yaml
sudo systemctl enable coretrace
sudo systemctl start coretrace
sudo journalctl -u coretrace -f
```

### Docker

```bash
docker run --privileged \
  -v /var/log:/var/log:ro \
  -v /etc:/etc:ro \
  -v /var/log/coretrace:/var/log/coretrace \
  coretrace/agent:latest
```

---

## Troubleshooting

**Commands not appearing in logs**
```bash
sudo systemctl status auditd
sudo auditctl -l | grep coretrace
sudo tail -f /var/log/audit/audit.log
```

**Agent not detecting SSH logins**
- Check the auth log path in `config.yaml` (`/var/log/auth.log` on Ubuntu, `/var/log/secure` on RHEL)
- Confirm the agent is running as root
- Run with `--debug` and SSH in from another terminal

**High disk usage**
- Tune `session_logging.rotation.max_age_days` and `max_backups` in `config.yaml`
- The agent logs disk usage warnings when thresholds are exceeded

---

## Roadmap

### Phase 1 — MVP ✅
- SSH session monitoring
- File integrity monitoring
- Auditd-based command logging
- Session-based JSONL logging with rotation
- WebSocket client for dashboard integration

### Phase 2 — Zero-Dependency (next)
- **eBPF command monitoring** — replaces auditd, zero customer setup, works on kernel 4.18+
- Fallback chain: eBPF → auditd → disabled
- **Runtime process visibility** — `execve`, privilege escalation, setuid tracking
- **Container awareness** — cgroup detection, container ID tagging

### Phase 3 — SaaS Integration
- Collector protocol — forward aggregated events to CoreTrace cloud
- Policy management via control plane API
- Compliance evidence packaging (SOC2, ISO27001)

### Phase 4 — Intelligence
- Behavioural baselines — learn what normal looks like per user/server
- Anomaly scoring — deviation from baseline triggers risk elevation
- Risk scores at session, user, server, and cluster levels
- Alerting integrations — Slack, PagerDuty, webhook, SIEM

---

## Dependencies

| Library | Purpose |
|---|---|
| `github.com/fsnotify/fsnotify` | File system monitoring via inotify |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/spf13/viper` | Configuration management |
| `go.uber.org/zap` | Structured logging |
| `gopkg.in/natefinch/lumberjack.v2` | Log rotation |
| `github.com/cilium/ebpf` | eBPF (Phase 2 — imported, not yet wired) |
| `github.com/gorilla/websocket` | Dashboard WebSocket client |
