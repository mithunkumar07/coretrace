#!/bin/bash
set -e

# CoreTrace Agent Uninstallation Script
# Usage: curl -sSL https://raw.githubusercontent.com/mithunkumar07/coretrace/main/coretrace-agent/uninstall.sh | sudo bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    log_error "Please run as root (use sudo)"
    exit 1
fi

echo -e "${BLUE}"
echo "╔════════════════════════════════════════════════════════╗"
echo "║     CoreTrace Agent Uninstallation Script              ║"
echo "╚════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Configuration
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/coretrace"
LOG_DIR="/var/log/coretrace"
SERVICE_NAME="coretrace-agent"

echo ""
read -p "Are you sure you want to uninstall CoreTrace Agent? This will remove all data. [y/N] " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Uninstallation cancelled"
    exit 0
fi

# Step 1: Stop and disable systemd service
log_step "Stopping CoreTrace Agent service..."
if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
    systemctl stop $SERVICE_NAME
    log_info "Service stopped"
else
    log_warn "Service was not running"
fi

if systemctl is-enabled --quiet $SERVICE_NAME 2>/dev/null; then
    systemctl disable $SERVICE_NAME
    log_info "Service disabled"
fi

# Step 2: Remove systemd service file
log_step "Removing systemd service..."
if [ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]; then
    rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
    systemctl daemon-reload
    log_info "Systemd service removed"
else
    log_warn "Systemd service file not found"
fi
echo ""

# Step 3: Remove auditd rules
log_step "Removing auditd rules..."
if [ -f "/etc/audit/rules.d/coretrace.rules" ]; then
    rm -f /etc/audit/rules.d/coretrace.rules
    log_info "Audit rules file removed"
    
    # Remove active rules
    if command -v auditctl &> /dev/null; then
        auditctl -d always,exit -F arch=b64 -S execve -S execveat -k session_monitoring 2>/dev/null || true
        auditctl -d always,exit -F arch=b32 -S execve -S execveat -k session_monitoring 2>/dev/null || true
        log_info "Active audit rules removed"
        
        # Reload rules
        if command -v augenrules &> /dev/null; then
            augenrules --load 2>/dev/null || true
        fi
    fi
else
    log_warn "Audit rules not found"
fi
echo ""

# Step 4: Remove binary
log_step "Removing CoreTrace Agent binary..."
if [ -f "${INSTALL_DIR}/coretrace-agent" ]; then
    rm -f "${INSTALL_DIR}/coretrace-agent"
    log_info "Binary removed from ${INSTALL_DIR}"
else
    log_warn "Binary not found in ${INSTALL_DIR}"
fi
echo ""

# Step 5: Backup and remove configuration
log_step "Handling configuration..."
if [ -d "$CONFIG_DIR" ]; then
    read -p "Keep configuration files at ${CONFIG_DIR}? [Y/n] " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Nn]$ ]]; then
        rm -rf "$CONFIG_DIR"
        log_info "Configuration directory removed"
    else
        log_info "Configuration preserved at ${CONFIG_DIR}"
    fi
else
    log_warn "Configuration directory not found"
fi
echo ""

# Step 6: Handle log files
log_step "Handling log files..."
if [ -d "$LOG_DIR" ]; then
    echo "Log files location: ${LOG_DIR}"
    echo "  - Session logs: ${LOG_DIR}/sessions/"
    echo "  - Orphaned events: ${LOG_DIR}/sessions/orphaned_events.log"
    echo ""
    
    read -p "What would you like to do with the logs? [K]eep / [B]ackup / [R]emove: " -n 1 -r
    echo ""
    
    case ${REPLY^^} in
        K|k|'')
            log_info "Log files preserved at ${LOG_DIR}"
            ;;
        B|b)
            BACKUP_DIR="/var/backups/coretrace-$(date +%Y%m%d-%H%M%S)"
            mkdir -p "$BACKUP_DIR"
            cp -r "$LOG_DIR" "$BACKUP_DIR/"
            rm -rf "$LOG_DIR"
            log_info "Logs backed up to ${BACKUP_DIR}"
            log_info "Original logs removed from ${LOG_DIR}"
            ;;
        R|r)
            rm -rf "$LOG_DIR"
            log_info "Log files removed"
            ;;
        *)
            log_warn "Invalid option. Logs preserved at ${LOG_DIR}"
            ;;
    esac
else
    log_warn "Log directory not found"
fi
echo ""

# Step 7: Clean up any remaining processes
log_step "Checking for remaining processes..."
if pgrep -x "coretrace-agent" > /dev/null; then
    log_warn "Found running coretrace-agent processes, terminating..."
    pkill -9 -x "coretrace-agent" 2>/dev/null || true
    log_info "Remaining processes terminated"
else
    log_info "No remaining processes found"
fi
echo ""

# Final summary
echo -e "${GREEN}"
echo "╔════════════════════════════════════════════════════════╗"
echo "║        Uninstallation Complete!                        ║"
echo "╚════════════════════════════════════════════════════════╝"
echo -e "${NC}"
echo ""
log_info "CoreTrace Agent has been uninstalled"
echo ""
echo "📋 Summary of actions:"
echo "   ✓ Service stopped and disabled"
echo "   ✓ Systemd service file removed"
echo "   ✓ Auditd rules removed"
echo "   ✓ Binary removed from ${INSTALL_DIR}"
if [ -d "$CONFIG_DIR" ]; then
    echo "   ✓ Configuration preserved at ${CONFIG_DIR}"
else
    echo "   ✓ Configuration removed"
fi
if [ -d "$LOG_DIR" ]; then
    echo "   ✓ Log files preserved at ${LOG_DIR}"
else
    echo "   ✓ Log files removed"
fi
echo ""
echo "📝 Notes:"
echo "   - Auditd service is still installed (not removed)"
echo "   - System users/groups were not created (none used)"
echo ""
echo "🔧 If you want to completely remove auditd:"
echo "   Ubuntu/Debian: apt-get remove --purge auditd"
echo "   RHEL/CentOS:   yum remove audit"
echo ""
echo "❓ Need help?"
echo "   Documentation: https://github.com/mithunkumar07/coretrace"
echo ""
