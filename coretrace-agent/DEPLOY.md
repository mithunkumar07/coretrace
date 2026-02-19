# CoreTrace Agent Deployment Guide

## Overview

Three deployment options:
1. **Quick Start** - Manual installation (testing)
2. **Production** - Automated install with systemd (recommended)
3. **Infrastructure as Code** - Ansible/Puppet/Docker

---

## Option 1: Quick Start (Manual)

Best for testing on a single server.

### Step 1: Download and Install

```bash
# Download the binary (replace with your actual download URL)
curl -L -o coretrace-agent https://github.com/mithunkumar07/coretrace/releases/download/v1.0.0/coretrace-agent-linux-amd64
chmod +x coretrace-agent
sudo mv coretrace-agent /usr/local/bin/

# Or build from source
git clone https://github.com/mithunkumar07/coretrace.git
cd coretrace/coretrace-agent
go build -o coretrace-agent .
sudo cp coretrace-agent /usr/local/bin/
```

### Step 2: Setup Command Monitoring (One-time)

```bash
# Run the automated setup script
sudo coretrace-agent setup

# Or manually setup auditd
sudo ./setup-auditd.sh
```

### Step 3: Create Config Directory

```bash
sudo mkdir -p /etc/coretrace
sudo mkdir -p /var/log/coretrace/sessions
```

### Step 4: Start Monitoring

```bash
# Run in foreground (good for testing)
sudo coretrace-agent monitor --debug

# Or run in background
sudo nohup coretrace-agent monitor > /var/log/coretrace/agent.log 2>&1 &
```

---

## Option 2: Production Deployment (Recommended)

### Automated Install Script

Save this as `install.sh`:

```bash
#!/bin/bash
set -e

# CoreTrace Agent Installation Script
# Usage: curl -sSL https://raw.githubusercontent.com/mithunkumar07/coretrace/main/coretrace-agent/install.sh | sudo bash

AGENT_VERSION="1.0.0"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/coretrace"
LOG_DIR="/var/log/coretrace"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    log_error "Please run as root (use sudo)"
    exit 1
fi

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64)
        ARCH="arm64"
        ;;
    *)
        log_error "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

log_info "Detected architecture: $ARCH"

# Create directories
log_info "Creating directories..."
mkdir -p "$CONFIG_DIR"
mkdir -p "$LOG_DIR/sessions"
chmod 755 "$LOG_DIR"

# Download agent
log_info "Downloading CoreTrace Agent v${AGENT_VERSION}..."
DOWNLOAD_URL="https://github.com/mithunkumar07/coretrace/releases/download/v${AGENT_VERSION}/coretrace-agent-linux-${ARCH}"

if command -v curl &> /dev/null; then
    curl -L -o "$INSTALL_DIR/coretrace-agent" "$DOWNLOAD_URL"
elif command -v wget &> /dev/null; then
    wget -O "$INSTALL_DIR/coretrace-agent" "$DOWNLOAD_URL"
else
    log_error "Neither curl nor wget found. Please install one of them."
    exit 1
fi

chmod +x "$INSTALL_DIR/coretrace-agent"
log_info "Agent installed to $INSTALL_DIR/coretrace-agent"

# Create default config
if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
    log_info "Creating default configuration..."
    cat > "$CONFIG_DIR/config.yaml" << 'EOF'
# CoreTrace Agent Configuration

agent:
  name: "coretrace-agent"
  version: "1.0.0"
  debug: false

logging:
  level: "info"
  format: "json"
  output: "stdout"

ssh:
  enabled: true
  auth_log_paths:
    - "/var/log/auth.log"
    - "/var/log/secure"
  session_timeout: "24h"
  max_sessions: 1000

file:
  enabled: true
  watch_paths:
    - "/etc"
    - "/home"
    - "/root"
    - "/var/log"
    - "/opt"
    - "/usr/local/bin"
    - "/tmp"
    - "/var/tmp"
  exclude_patterns:
    - "*.tmp"
    - "*.log"
    - ".git/*"
    - "node_modules/*"
    - ".cache/*"
  max_watched_files: 10000

session_logging:
  enabled: true
  sessions_dir: "/var/log/coretrace/sessions"
  max_session_duration: "24h"
  cleanup_interval: "1h"
  rotation:
    max_size_mb: 100
    max_backups: 10
    max_age_days: 30
    compress: true
EOF
    log_info "Configuration created at $CONFIG_DIR/config.yaml"
fi

# Setup auditd for command monitoring
log_info "Setting up auditd for command monitoring..."
if command -v apt-get &> /dev/null; then
    # Debian/Ubuntu
    apt-get update -qq
    apt-get install -y -qq auditd audispd-plugins || log_warn "Could not install auditd via apt"
elif command -v yum &> /dev/null; then
    # RHEL/CentOS
    yum install -y audit || log_warn "Could not install auditd via yum"
elif command -v dnf &> /dev/null; then
    # Fedora
    dnf install -y audit || log_warn "Could not install auditd via dnf"
fi

# Configure auditd rules
if command -v auditctl &> /dev/null; then
    log_info "Configuring auditd rules..."
    auditctl -a always,exit -F arch=b64 -S execve -S execveat -k session_monitoring 2>/dev/null || true
    auditctl -a always,exit -F arch=b32 -S execve -S execveat -k session_monitoring 2>/dev/null || true
    
    # Make persistent
    mkdir -p /etc/audit/rules.d
    cat > /etc/audit/rules.d/coretrace.rules << 'EOF'
# CoreTrace command monitoring rules
-a always,exit -F arch=b64 -S execve -S execveat -k session_monitoring
-a always,exit -F arch=b32 -S execve -S execveat -k session_monitoring
EOF
    
    if command -v augenrules &> /dev/null; then
        augenrules --load 2>/dev/null || true
    fi
    
    # Start auditd
    if command -v systemctl &> /dev/null; then
        systemctl restart auditd 2>/dev/null || systemctl start auditd 2>/dev/null || true
        systemctl enable auditd 2>/dev/null || true
    fi
    
    log_info "Auditd configured successfully"
else
    log_warn "auditctl not found. Command monitoring will be disabled."
    log_warn "SSH and file monitoring will still work."
fi

# Create systemd service
log_info "Creating systemd service..."
cat > /etc/systemd/system/coretrace-agent.service << EOF
[Unit]
Description=CoreTrace Agent - SSH Session Monitoring
Documentation=https://docs.coretrace.io
After=network.target auditd.service
Wants=auditd.service

[Service]
Type=simple
ExecStart=$INSTALL_DIR/coretrace-agent monitor --config $CONFIG_DIR/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=coretrace-agent

# Security settings
NoNewPrivileges=false
ProtectSystem=false
ProtectHome=false

# Resource limits
LimitNOFILE=65536
MemoryMax=512M
CPUQuota=50%

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
if command -v systemctl &> /dev/null; then
    systemctl daemon-reload
    systemctl enable coretrace-agent
    log_info "Systemd service created and enabled"
fi

# Start the service
log_info "Starting CoreTrace Agent..."
if command -v systemctl &> /dev/null; then
    systemctl start coretrace-agent
    sleep 2
    if systemctl is-active --quiet coretrace-agent; then
        log_info "CoreTrace Agent is running!"
    else
        log_error "Failed to start CoreTrace Agent"
        log_error "Check logs: journalctl -u coretrace-agent -f"
        exit 1
    fi
else
    log_warn "systemd not found. Please start the agent manually:"
    log_warn "  coretrace-agent monitor --config $CONFIG_DIR/config.yaml"
fi

log_info "Installation complete!"
log_info ""
log_info "Useful commands:"
log_info "  View status:     systemctl status coretrace-agent"
log_info "  View logs:       journalctl -u coretrace-agent -f"
log_info "  Session logs:    ls -la $LOG_DIR/sessions/"
log_info "  Edit config:     nano $CONFIG_DIR/config.yaml"
log_info "  Restart agent:   systemctl restart coretrace-agent"
log_info ""
log_info "Test the agent:"
log_info "  1. SSH into this server from another machine"
log_info "  2. Run some commands"
log_info "  3. Check: tail -f $LOG_DIR/sessions/*/*"
```

### Deploy with One Command

```bash
# Option A: Download and run
curl -sSL https://raw.githubusercontent.com/mithunkumar07/coretrace/main/coretrace-agent/install.sh | sudo bash

# Option B: Save and run locally
curl -sSL https://raw.githubusercontent.com/mithunkumar07/coretrace/main/coretrace-agent/install.sh -o install.sh
chmod +x install.sh
sudo ./install.sh
```

---

## Option 3: Ansible Playbook

Create `deploy-coretrace.yml`:

```yaml
---
- name: Deploy CoreTrace Agent
  hosts: all
  become: yes
  vars:
    coretrace_version: "1.0.0"
    coretrace_arch: "{{ 'arm64' if ansible_architecture == 'aarch64' else 'amd64' }}"
    coretrace_download_url: "https://github.com/mithunkumar07/coretrace/releases/download/v{{ coretrace_version }}/coretrace-agent-linux-{{ coretrace_arch }}"
  
  tasks:
    - name: Create CoreTrace directories
      file:
        path: "{{ item }}"
        state: directory
        mode: '0755'
      loop:
        - /etc/coretrace
        - /var/log/coretrace/sessions
    
    - name: Download CoreTrace Agent
      get_url:
        url: "{{ coretrace_download_url }}"
        dest: /usr/local/bin/coretrace-agent
        mode: '0755'
      notify: restart coretrace-agent
    
    - name: Install auditd (Debian/Ubuntu)
      apt:
        name:
          - auditd
          - audispd-plugins
        state: present
        update_cache: yes
      when: ansible_os_family == "Debian"
    
    - name: Install auditd (RHEL/CentOS/Fedora)
      yum:
        name: audit
        state: present
      when: ansible_os_family == "RedHat"
    
    - name: Configure auditd rules
      template:
        src: coretrace.rules.j2
        dest: /etc/audit/rules.d/coretrace.rules
      notify: reload auditd
    
    - name: Ensure auditd is running
      service:
        name: auditd
        state: started
        enabled: yes
    
    - name: Deploy configuration
      template:
        src: config.yaml.j2
        dest: /etc/coretrace/config.yaml
      notify: restart coretrace-agent
    
    - name: Deploy systemd service
      template:
        src: coretrace-agent.service.j2
        dest: /etc/systemd/system/coretrace-agent.service
      notify:
        - reload systemd
        - restart coretrace-agent
    
    - name: Enable and start CoreTrace Agent
      systemd:
        name: coretrace-agent
        state: started
        enabled: yes
        daemon_reload: yes
  
  handlers:
    - name: reload auditd
      command: augenrules --load
      ignore_errors: yes
    
    - name: reload systemd
      systemd:
        daemon_reload: yes
    
    - name: restart coretrace-agent
      systemd:
        name: coretrace-agent
        state: restarted
```

Create `coretrace.rules.j2`:
```
# CoreTrace command monitoring rules
-a always,exit -F arch=b64 -S execve -S execveat -k session_monitoring
-a always,exit -F arch=b32 -S execve -S execveat -k session_monitoring
```

Run:
```bash
ansible-playbook -i inventory.ini deploy-coretrace.yml
```

---

## Option 4: Docker Deployment

### Dockerfile

```dockerfile
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y \
    auditd \
    audispd-plugins \
    && rm -rf /var/lib/apt/lists/*

COPY coretrace-agent /usr/local/bin/
COPY config.yaml /etc/coretrace/

# Setup auditd rules
RUN echo '-a always,exit -F arch=b64 -S execve -S execveat -k session_monitoring' > /etc/audit/rules.d/coretrace.rules

ENTRYPOINT ["/usr/local/bin/coretrace-agent"]
CMD ["monitor", "--config", "/etc/coretrace/config.yaml"]
```

### Docker Compose

```yaml
version: '3.8'

services:
  coretrace:
    build: .
    privileged: true
    pid: host
    volumes:
      - /var/log:/var/log:ro
      - /etc:/etc:ro
      - /proc:/proc:ro
      - /var/log/coretrace:/var/log/coretrace
      - ./config.yaml:/etc/coretrace/config.yaml:ro
    restart: unless-stopped
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "3"
```

**Note:** Docker deployment has limitations - it can't monitor the host's SSH daemon effectively unless running in very privileged mode.

---

## Option 5: Kubernetes DaemonSet

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: coretrace-agent
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app: coretrace-agent
  template:
    metadata:
      labels:
        app: coretrace-agent
    spec:
      hostNetwork: true
      hostPID: true
      containers:
      - name: coretrace-agent
        image: yourregistry/coretrace-agent:v1.0.0
        securityContext:
          privileged: true
        volumeMounts:
        - name: varlog
          mountPath: /var/log
          readOnly: true
        - name: etc
          mountPath: /etc
          readOnly: true
        - name: proc
          mountPath: /proc
          readOnly: true
        - name: sessions
          mountPath: /var/log/coretrace/sessions
        resources:
          limits:
            memory: "512Mi"
            cpu: "500m"
          requests:
            memory: "128Mi"
            cpu: "100m"
      volumes:
      - name: varlog
        hostPath:
          path: /var/log
      - name: etc
        hostPath:
          path: /etc
      - name: proc
        hostPath:
          path: /proc
      - name: sessions
        hostPath:
          path: /var/log/coretrace/sessions
          type: DirectoryOrCreate
```

---

## Post-Deployment Verification

### 1. Check Agent Status

```bash
# Systemd service
systemctl status coretrace-agent

# View logs
journalctl -u coretrace-agent -f

# Check process
ps aux | grep coretrace-agent
```

### 2. Verify Session Logs

```bash
# List session directories
ls -la /var/log/coretrace/sessions/

# Check today's sessions
ls -la /var/log/coretrace/sessions/$(date +%Y-%m-%d)/

# View a session log
tail -f /var/log/coretrace/sessions/$(date +%Y-%m-%d)/*.jsonl
```

### 3. Test SSH Monitoring

```bash
# From another machine, SSH into the server
ssh user@your-server-ip

# Run some commands
ls -la
pwd
cat /etc/passwd

# Logout
exit

# On the server, check the session log
ls /var/log/coretrace/sessions/$(date +%Y-%m-%d)/
cat /var/log/coretrace/sessions/$(date +%Y-%m-%d)/*_user_*.jsonl
```

### 4. Test File Monitoring

```bash
# Create a test file
touch /tmp/coretrace-test.txt

# Check if it appears in logs
grep "coretrace-test" /var/log/coretrace/sessions/*/*.jsonl
```

---

## Configuration Management

### Environment Variables

Override config with environment variables:

```bash
export CORETRACE_AGENT_DEBUG=true
export CORETRACE_SESSION_LOGGING_SESSIONS_DIR=/custom/log/path
export CORETRACE_SSH_ENABLED=false  # Disable SSH monitoring
```

### Multiple Config Files

```bash
# Production config
sudo coretrace-agent monitor --config /etc/coretrace/prod.yaml

# Staging config
sudo coretrace-agent monitor --config /etc/coretrace/staging.yaml
```

---

## Troubleshooting

### Agent Won't Start

```bash
# Check for errors
journalctl -u coretrace-agent -n 50

# Run manually to see errors
sudo /usr/local/bin/coretrace-agent monitor --debug

# Check permissions
ls -la /var/log/coretrace/
ls -la /usr/local/bin/coretrace-agent
```

### Commands Not Being Logged

```bash
# Check if auditd is running
systemctl status auditd

# Verify audit rules
sudo auditctl -l | grep session_monitoring

# Check audit logs
sudo tail -f /var/log/audit/audit.log | grep session_monitoring
```

### High CPU/Memory Usage

```bash
# Monitor resource usage
top -p $(pgrep coretrace-agent)

# Adjust limits in config.yaml
performance:
  max_cpu_percent: 25
  max_memory_mb: 256

# Or use systemd limits
systemctl edit coretrace-agent
# Add:
# [Service]
# MemoryMax=256M
# CPUQuota=25%
```

---

## Security Considerations

1. **Run as root** - Required for monitoring (can use capabilities on newer kernels)
2. **Log permissions** - Session logs contain sensitive data, restrict access:
   ```bash
   chmod 750 /var/log/coretrace
   chown root:root /var/log/coretrace/sessions
   ```
3. **Network** - Agent doesn't need network access unless sending to cloud collector
4. **Audit logs** - Ensure auditd logs are rotated to prevent disk full

---

## Updates

```bash
# Download new version
curl -L -o /usr/local/bin/coretrace-agent https://github.com/mithunkumar07/coretrace/releases/download/v1.0.0/coretrace-agent-v1.1.0
chmod +x /usr/local/bin/coretrace-agent

# Restart service
sudo systemctl restart coretrace-agent

# Verify
sudo systemctl status coretrace-agent
```

## Uninstall

```bash
# Stop and disable service
sudo systemctl stop coretrace-agent
sudo systemctl disable coretrace-agent

# Remove files
sudo rm /usr/local/bin/coretrace-agent
sudo rm /etc/systemd/system/coretrace-agent.service
sudo rm -rf /etc/coretrace

# Optional: Remove logs
sudo rm -rf /var/log/coretrace

# Optional: Remove auditd rules
sudo rm /etc/audit/rules.d/coretrace.rules
sudo augenrules --load
```
