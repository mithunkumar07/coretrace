#!/bin/bash
# Setup script for CoreTrace command monitoring via auditd

set -e

echo "CoreTrace Command Monitoring Setup"
echo "=================================="
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo "This script must be run as root (use sudo)"
    exit 1
fi

# Check if auditd is installed
if ! command -v auditctl &> /dev/null; then
    echo "auditctl not found. Installing auditd..."
    
    if command -v apt-get &> /dev/null; then
        # Debian/Ubuntu
        apt-get update
        apt-get install -y auditd audispd-plugins
    elif command -v yum &> /dev/null; then
        # RHEL/CentOS
        yum install -y audit
    elif command -v dnf &> /dev/null; then
        # Fedora
        dnf install -y audit
    else
        echo "Could not install auditd automatically. Please install manually."
        exit 1
    fi
fi

# Check if auditd is running
if ! systemctl is-active --quiet auditd; then
    echo "Starting auditd service..."
    systemctl start auditd
    systemctl enable auditd
fi

echo ""
echo "Adding audit rules for command monitoring..."

# Add rules to monitor execve/execveat syscalls
# These rules will log all command executions with key "session_monitoring"

# Remove existing rules with our key to avoid duplicates
auditctl -d always,exit -F arch=b64 -S execve -S execveat -k session_monitoring 2>/dev/null || true
auditctl -d always,exit -F arch=b32 -S execve -S execveat -k session_monitoring 2>/dev/null || true

# Add new rules
auditctl -a always,exit -F arch=b64 -S execve -S execveat -k session_monitoring
auditctl -a always,exit -F arch=b32 -S execve -S execveat -k session_monitoring

echo ""
echo "Current audit rules:"
auditctl -l | grep session_monitoring || echo "No session_monitoring rules found"

echo ""
echo "Making rules persistent..."

# Add rules to audit.rules for persistence
AUDIT_RULES_FILE="/etc/audit/rules.d/coretrace.rules"

cat > "$AUDIT_RULES_FILE" << 'EOF'
# CoreTrace command monitoring rules
# Monitor execve and execveat syscalls for session tracking

# 64-bit architecture
-a always,exit -F arch=b64 -S execve -S execveat -k session_monitoring

# 32-bit architecture  
-a always,exit -F arch=b32 -S execve -S execveat -k session_monitoring

# Ensure auditd continues running even if disk is full
-f 1
EOF

echo "Rules written to $AUDIT_RULES_FILE"

# Reload auditd rules
if command -v augenrules &> /dev/null; then
    augenrules --load
elif auditctl -R /etc/audit/rules.d/*.rules &> /dev/null; then
    echo "Rules loaded via auditctl"
else
    echo "Note: Please restart auditd to load persistent rules: systemctl restart auditd"
fi

echo ""
echo "=================================="
echo "Setup Complete!"
echo "=================================="
echo ""
echo "Test the setup:"
echo "1. Run: sudo ./coretrace-agent monitor --debug"
echo "2. SSH into this machine from another terminal"
echo "3. Run some commands (ls, pwd, etc.)"
echo "4. Check the session logs in /var/log/coretrace/sessions/"
echo ""
echo "To remove monitoring rules:"
echo "  sudo auditctl -d always,exit -F arch=b64 -S execve -S execveat -k session_monitoring"
echo "  sudo auditctl -d always,exit -F arch=b32 -S execve -S execveat -k session_monitoring"
echo "  sudo rm /etc/audit/rules.d/coretrace.rules"
