# CoreTrace Agent - Quick Start Guide

Deploy the CoreTrace Agent to a Linux server in **5 minutes**.

## Prerequisites

- Linux server (Ubuntu, CentOS, Debian, RHEL, etc.)
- Root/sudo access
- SSH access to the server

## Option 1: Automated Installation (Recommended)

### Step 1: Build the Agent

On your local machine:

```bash
cd coretrace-agent
go build -o coretrace-agent .
```

### Step 2: Copy to Server

```bash
# Copy binary to server
scp coretrace-agent user@your-server:/tmp/

# Copy install script
scp install.sh user@your-server:/tmp/
```

### Step 3: Install on Server

SSH into your server and run:

```bash
ssh user@your-server

# Move to proper location
sudo mv /tmp/coretrace-agent /usr/local/bin/
sudo chmod +x /usr/local/bin/coretrace-agent

# Run install script
sudo bash /tmp/install.sh
```

That's it! The agent is now running.

### Step 4: Verify Installation

```bash
# Check service status
systemctl status coretrace-agent

# View logs
journalctl -u coretrace-agent -f

# List session directories
ls -la /var/log/coretrace/sessions/
```

### Step 5: Test

From another machine, SSH into your server and run commands:

```bash
ssh user@your-server
ls -la
pwd
cat /etc/passwd
exit
```

Check the session logs:

```bash
# On the server
tail -f /var/log/coretrace/sessions/$(date +%Y-%m-%d)/*.jsonl
```

You should see JSON logs with your SSH session and commands.

---

## Option 2: Manual Installation

If you prefer to understand each step:

```bash
# 1. SSH into your server
ssh user@your-server

# 2. Create directories
sudo mkdir -p /usr/local/bin
sudo mkdir -p /etc/coretrace
sudo mkdir -p /var/log/coretrace/sessions

# 3. Copy binary (from your local machine)
# On local: scp coretrace-agent user@server:/tmp/
sudo mv /tmp/coretrace-agent /usr/local/bin/
sudo chmod +x /usr/local/bin/coretrace-agent

# 4. Setup auditd for command monitoring
sudo ./setup-auditd.sh

# 5. Create config
sudo tee /etc/coretrace/config.yaml << 'EOF'
agent:
  name: "coretrace-agent"
  version: "1.0.0"
  debug: false

session_logging:
  enabled: true
  sessions_dir: "/var/log/coretrace/sessions"
  rotation:
    max_size_mb: 100
    max_backups: 10
    max_age_days: 30
    compress: true
EOF

# 6. Create systemd service
sudo tee /etc/systemd/system/coretrace-agent.service << 'EOF'
[Unit]
Description=CoreTrace Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/coretrace-agent monitor --config /etc/coretrace/config.yaml
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# 7. Start service
sudo systemctl daemon-reload
sudo systemctl enable coretrace-agent
sudo systemctl start coretrace-agent

# 8. Verify
sudo systemctl status coretrace-agent
```

---

## Option 3: Docker (Testing Only)

For quick testing in a container:

```bash
# Build image
docker build -t coretrace-agent .

# Run (privileged mode required for monitoring)
docker run -d --name coretrace \
  --privileged \
  -v /var/log:/var/log:ro \
  -v /etc:/etc:ro \
  -v /proc:/proc:ro \
  coretrace-agent

# View logs
docker logs -f coretrace
```

**Note:** Docker mode has limitations for real SSH monitoring since it's isolated from host SSH daemon.

---

## Common Issues

### "command not found: coretrace-agent"

```bash
# Check if binary exists
ls -la /usr/local/bin/coretrace-agent

# If not, copy it
sudo cp /path/to/coretrace-agent /usr/local/bin/
sudo chmod +x /usr/local/bin/coretrace-agent
```

### "Failed to start service"

```bash
# Check detailed error
sudo journalctl -u coretrace-agent -n 50

# Run manually to see error
sudo /usr/local/bin/coretrace-agent monitor --debug
```

### "Commands not being logged"

```bash
# Check if auditd is running
sudo systemctl status auditd

# Check audit rules
sudo auditctl -l | grep session_monitoring

# Re-run setup
sudo ./setup-auditd.sh
```

### "Permission denied"

The agent requires root to read system logs and monitor files:

```bash
# Always run as root
sudo coretrace-agent monitor

# Or via systemd (runs as root by default)
sudo systemctl start coretrace-agent
```

---

## What's Next?

After deployment:

1. **Configure** - Edit `/etc/coretrace/config.yaml` to customize:
   - Which directories to monitor
   - Log rotation settings
   - Exclude patterns

2. **Integrate** - Send logs to your SIEM or cloud storage:
   ```bash
   # Example: Ship to Splunk
   tail -f /var/log/coretrace/sessions/*/*.jsonl | nc splunk-server 9997
   
   # Example: Sync to S3
   aws s3 sync /var/log/coretrace/sessions/ s3://your-bucket/sessions/
   ```

3. **Monitor** - Set up alerting:
   - Failed SSH attempts
   - File changes in sensitive directories
   - Unusual command patterns

4. **Scale** - Deploy to multiple servers:
   - Use Ansible playbook (see DEPLOY.md)
   - Use your configuration management tool
   - Automate with CI/CD pipeline

---

## Production Checklist

Before deploying to production:

- [ ] Agent installed and running
- [ ] Auditd configured and running
- [ ] Session logs directory created with proper permissions
- [ ] Log rotation configured
- [ ] Systemd service enabled (starts on boot)
- [ ] Tested SSH login/logout tracking
- [ ] Tested command logging
- [ ] Tested file change monitoring
- [ ] Log shipping configured (optional)
- [ ] Monitoring/alerting configured (optional)
- [ ] Documentation updated for your team

---

## Support

- **Full Deployment Guide:** See [DEPLOY.md](DEPLOY.md)
- **Configuration Reference:** See [README.md](README.md)
- **Build Instructions:** See [BUILD.md](BUILD.md)
- **Troubleshooting:** Check logs with `journalctl -u coretrace-agent -f`

---

## One-Line Deployment

For automation, you can deploy with a single command after building:

```bash
# Build locally
go build -o coretrace-agent .

# Deploy with install script
cat install.sh | ssh user@server "cat > /tmp/install.sh && sudo bash /tmp/install.sh"
```

Or use the Ansible playbook for multiple servers:

```bash
ansible-playbook -i inventory.ini deploy-coretrace.yml
```
