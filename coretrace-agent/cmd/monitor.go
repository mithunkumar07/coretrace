package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coretrace/agent/internal/monitor"
	"github.com/coretrace/agent/internal/types"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Start monitoring SSH sessions and file activities",
	Long: `Start the CoreTrace agent to monitor SSH sessions, login attempts,
commands, and file activities in real-time.`,
	Run: func(cmd *cobra.Command, args []string) {
		startMonitoring()
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd)
}

func startMonitoring() {
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create event channels
	sshEventChan := make(chan types.SSHEvent, 1000)
	fileEventChan := make(chan types.FileEvent, 1000)

	// Initialize monitors
	sshMonitor := monitor.NewSSHMonitor(logger, sshEventChan)
	fileMonitor, err := monitor.NewFileMonitor(logger, fileEventChan)
	if err != nil {
		logger.Fatal("Failed to create file monitor", zap.Error(err))
	}

	// Start monitors
	if err := sshMonitor.Start(ctx); err != nil {
		logger.Fatal("Failed to start SSH monitor", zap.Error(err))
	}

	if err := fileMonitor.Start(ctx); err != nil {
		logger.Fatal("Failed to start file monitor", zap.Error(err))
	}

	logger.Info("CoreTrace agent started successfully")

	// Start event processor
	go processEvents(ctx, logger, sshEventChan, fileEventChan)

	// Wait for shutdown signal
	<-sigChan
	logger.Info("Shutting down CoreTrace agent...")

	cancel()

	// Give some time for cleanup
	time.Sleep(2 * time.Second)

	// Stop file monitor
	if err := fileMonitor.Stop(); err != nil {
		logger.Error("Error stopping file monitor", zap.Error(err))
	}

	logger.Info("CoreTrace agent stopped")
}

func processEvents(ctx context.Context, logger *zap.Logger,
	sshEventChan <-chan types.SSHEvent,
	fileEventChan <-chan types.FileEvent) {

	for {
		select {
		case <-ctx.Done():
			return
		case sshEvent := <-sshEventChan:
			handleSSHEvent(logger, sshEvent)
		case fileEvent := <-fileEventChan:
			handleFileEvent(logger, fileEvent)
		}
	}
}

func handleSSHEvent(logger *zap.Logger, event types.SSHEvent) {
	switch event.EventType {
	case types.EventSSHLogin:
		if event.Success {
			logger.Info("SSH Login detected",
				zap.String("session_id", event.SessionID),
				zap.String("username", event.Username),
				zap.String("source_ip", event.SourceIP),
				zap.String("auth_method", event.AuthMethod),
				zap.Time("timestamp", event.Timestamp),
			)
		} else {
			logger.Warn("SSH Login failed",
				zap.String("username", event.Username),
				zap.String("source_ip", event.SourceIP),
				zap.String("auth_method", event.AuthMethod),
				zap.Time("timestamp", event.Timestamp),
			)
		}
	case types.EventSSHLogout:
		logger.Info("SSH Logout detected",
			zap.String("username", event.Username),
			zap.Time("timestamp", event.Timestamp),
		)
	case types.EventSSHFailed:
		logger.Warn("SSH Failed attempt",
			zap.String("username", event.Username),
			zap.String("source_ip", event.SourceIP),
			zap.Time("timestamp", event.Timestamp),
		)
	}
}

func handleFileEvent(logger *zap.Logger, event types.FileEvent) {
	logger.Info("File activity detected",
		zap.String("file_path", event.FilePath),
		zap.String("operation", event.Operation),
		zap.String("username", event.Username),
		zap.String("session_id", event.SessionID),
		zap.Time("timestamp", event.Timestamp),
	)
}
