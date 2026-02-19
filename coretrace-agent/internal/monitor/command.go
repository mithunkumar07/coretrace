package monitor

import (
	"context"

	"github.com/coretrace/agent/internal/types"
	"go.uber.org/zap"
)

// CommandMonitor monitors command execution using the best available backend
type CommandMonitor struct {
	logger    *zap.Logger
	backend   *auditdMonitor
	eventChan chan<- types.CommandEvent
	debug     bool
}

// NewCommandMonitor creates a new command monitor
func NewCommandMonitor(logger *zap.Logger, eventChan chan<- types.CommandEvent, debug bool) *CommandMonitor {
	return &CommandMonitor{
		logger:    logger,
		eventChan: eventChan,
		debug:     debug,
	}
}

// Start begins monitoring command execution
func (cm *CommandMonitor) Start(ctx context.Context) error {
	cm.logger.Info("Starting command monitor")

	// Use auditd backend for now
	// TODO: Add eBPF backend with build tags when bpf2go is configured
	backend := newAuditdMonitor(cm.logger, cm.eventChan, cm.debug)

	if err := backend.Start(ctx); err != nil {
		cm.logger.Warn("Command monitoring unavailable - auditd not configured",
			zap.Error(err))
		cm.logger.Info("To enable command monitoring, run:")
		cm.logger.Info("  sudo ./setup-auditd.sh")
		cm.logger.Info("Command monitoring will be disabled but SSH and file monitoring continue")
		return nil
	}

	cm.backend = backend
	cm.logger.Info("Command monitoring active via auditd")
	return nil
}

// Stop gracefully shuts down the command monitor
func (cm *CommandMonitor) Stop() error {
	cm.logger.Info("Stopping command monitor")
	if cm.backend != nil {
		return cm.backend.Stop()
	}
	return nil
}

// RegisterSession associates a PID with a session ID
func (cm *CommandMonitor) RegisterSession(sessionID string, pid int) {
	if cm.backend != nil {
		cm.backend.RegisterSession(sessionID, pid)
	}
}

// UnregisterSession removes a session tracking
func (cm *CommandMonitor) UnregisterSession(sessionID string) {
	if cm.backend != nil {
		cm.backend.UnregisterSession(sessionID)
	}
}

// Cleanup removes stale PID mappings
func (cm *CommandMonitor) Cleanup() {
	if cm.backend != nil {
		cm.backend.Cleanup()
	}
}

// IsAvailable returns true if command monitoring is active
func (cm *CommandMonitor) IsAvailable() bool {
	return cm.backend != nil
}
