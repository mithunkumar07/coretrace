# CoreTrace Agent - SSH Session Monitoring

## Current Focus: Building Go CLI agent for SSH session intelligence

### Core Requirements:
1. SSH login detection (successful/failed)
2. Key fingerprinting for authentication method
3. Source IP address and geolocation
4. Command logging per session
5. File access/change tracking per session
6. Session correlation across all events

### Technical Approach:
- Use auditd subsystems for SSH events
- Monitor auth logs for login attempts
- Track process execution with eBPF where possible
- File integrity monitoring via inotify
- Session ID correlation across all event types
- Structured logging with JSON output

### Architecture:
- Single binary deployment
- Configuration via YAML/JSON
- Output to stdout/file for downstream processing
- Minimal external dependencies
- Root privilege required for comprehensive monitoring