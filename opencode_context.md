# CoreTrace Agent - Current State

## What's Implemented

### ✅ MVP Features (Working Now)
1. **SSH Session Monitoring**
   - Detects successful/failed SSH logins
   - Captures username, source IP, auth method
   - Extracts SSH key fingerprints from auth logs
   - Tracks session start/end times

2. **Session-Based Logging**
   - Each SSH session gets its own JSONL log file
   - Date-organized directory structure: `/var/log/coretrace/sessions/YYYY-MM-DD/`
   - Automatic log rotation (size, backups, age, compression)
   - Orphaned events tracked separately

3. **File Integrity Monitoring**
   - Real-time file change detection via fsnotify
   - Monitors: /etc, /home, /root, /var/log, /opt, /tmp
   - Tracks create, modify, delete, chmod operations
   - Correlates file events to SSH sessions via PID

4. **Command Logging** (via auditd)
   - Captures all commands executed during SSH sessions
   - Tracks: command, arguments, working directory, PID, PPID
   - Requires one-time auditd setup via `setup-auditd.sh`
   - Graceful fallback if auditd unavailable

### 📁 Project Structure
```
coretrace-agent/
├── main.go                          # Entry point
├── cmd/
│   ├── root.go                      # CLI commands
│   └── monitor.go                   # Main monitoring orchestration
├── internal/
│   ├── types/
│   │   └── events.go                # Event type definitions
│   ├── monitor/
│   │   ├── ssh.go                   # SSH auth log monitoring
│   │   ├── file.go                  # File system monitoring
│   │   ├── command.go               # Command monitor interface
│   │   └── command_auditd.go        # Auditd backend
│   └── logger/
│       └── session.go               # Session logging with rotation
├── config.yaml                      # Configuration
├── setup-auditd.sh                  # Automated auditd setup
├── README.md                        # User documentation
└── BUILD.md                         # Build instructions
```

### 🔧 Build & Deploy
```bash
# Build
cd coretrace-agent
go build -o coretrace-agent .

# Setup (one-time)
sudo ./setup-auditd.sh

# Run
sudo ./coretrace-agent monitor
```

## Architecture Decisions

### Why auditd for MVP?
- Works on all Linux kernels
- No CGO/complex build process
- Battle-tested approach
- Setup can be automated

### Why eBPF for Future?
- Zero customer setup required
- Better performance
- Modern standard (Cilium, Falco use it)
- Self-contained in binary

## Current Limitations

1. **Command logging requires auditd setup**
   - Mitigation: Automated setup script
   - Plan: Add eBPF support for zero-setup

2. **Kernel 4.18+ needed for eBPF**
   - Current: Works on all kernels with auditd
   - Future: eBPF primary, auditd fallback

3. **No network event correlation yet**
   - Not capturing network connections per session
   - Plan: Add via eBPF or netlink

## Product Roadmap

### Phase 1: MVP (Current) ✅
- SSH session monitoring
- File integrity monitoring
- Session logging with rotation
- Auditd-based command logging

### Phase 2: Zero-Dependency (Q2 2024)
- eBPF-based command monitoring
- Removes auditd requirement
- Graceful fallback chain: eBPF → auditd → disabled

### Phase 3: SaaS Integration (Q3 2024)
- Collector protocol for cloud
- Control plane API
- Compliance dashboards

### Phase 4: Intelligence (Q4 2024)
- Behavioral baselines
- Risk scoring
- Anomaly detection

## Key Technical Insights

1. **Session Correlation Strategy:**
   - SSH login captures the sshd PID
   - Command/file monitors walk PID tree to find session
   - Events tagged with session_id for aggregation

2. **Log Rotation:**
   - Uses lumberjack library
   - Per-session files rotated independently
   - Date-based organization for easy cleanup

3. **Error Handling:**
   - Each monitor is independent
   - Failures don't crash other monitors
   - Graceful degradation (e.g., commands disabled if auditd missing)

## Dependencies

**Runtime:**
- Linux (any modern version)
- Root privileges
- Optional: auditd (for commands)

**Build:**
- Go 1.21+
- No CGO required

**Go Libraries:**
- github.com/fsnotify/fsnotify (file monitoring)
- github.com/spf13/cobra (CLI)
- github.com/spf13/viper (config)
- go.uber.org/zap (logging)
- gopkg.in/natefinch/lumberjack.v2 (log rotation)

## Testing Strategy

```bash
# Build
go build -o coretrace-agent .

# Test SSH monitoring
sudo ./coretrace-agent monitor --debug
# Then SSH in and check logs

# Test file monitoring
sudo ./coretrace-agent monitor
touch /tmp/test-file
# Check session logs for file_create event

# Test command monitoring (requires auditd)
sudo ./setup-auditd.sh
sudo ./coretrace-agent monitor --debug
# SSH in, run commands, check session logs
```

## Notes for Future Development

- eBPF will require: clang, llvm, kernel headers
- Consider BTF for portable eBPF (no kernel headers needed)
- Container/K8s support needs cgroup awareness
- Windows support would require completely different approach
- macOS support limited (no auditd, eBPF different)
