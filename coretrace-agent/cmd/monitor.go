package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coretrace/agent/internal/logger"
	"github.com/coretrace/agent/internal/monitor"
	"github.com/coretrace/agent/internal/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	// Initialize logger from config
	zapLogger, err := logger.NewLoggerFromConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer zapLogger.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create event channels
	sshEventChan := make(chan types.SSHEvent, 1000)
	fileEventChan := make(chan types.FileEvent, 1000)
	cmdEventChan := make(chan types.CommandEvent, 1000)

	// Initialize session logger
	sessionsDir := viper.GetString("session_logging.sessions_dir")
	if sessionsDir == "" {
		sessionsDir = "/var/log/coretrace/sessions"
	}

	rotationConfig := logger.RotationConfig{
		MaxSize:    viper.GetInt("session_logging.rotation.max_size_mb"),
		MaxBackups: viper.GetInt("session_logging.rotation.max_backups"),
		MaxAge:     viper.GetInt("session_logging.rotation.max_age_days"),
		Compress:   viper.GetBool("session_logging.rotation.compress"),
	}

	// Set defaults if not configured
	if rotationConfig.MaxSize == 0 {
		rotationConfig.MaxSize = 100
	}
	if rotationConfig.MaxBackups == 0 {
		rotationConfig.MaxBackups = 10
	}
	if rotationConfig.MaxAge == 0 {
		rotationConfig.MaxAge = 30
	}

	sessionLogger, err := logger.NewSessionLogger(zapLogger, sessionsDir, rotationConfig)
	if err != nil {
		zapLogger.Fatal("Failed to create session logger", zap.Error(err))
	}
	defer sessionLogger.Close()

	// Initialize monitors
	sshMonitor := monitor.NewSSHMonitor(zapLogger, sshEventChan)
	fileMonitor, err := monitor.NewFileMonitor(zapLogger, fileEventChan)
	if err != nil {
		zapLogger.Fatal("Failed to create file monitor", zap.Error(err))
	}

	// Initialize command monitor (requires auditd)
	debug := viper.GetBool("agent.debug")
	cmdMonitor := monitor.NewCommandMonitor(zapLogger, cmdEventChan, debug)

	// Start monitors
	if err := sshMonitor.Start(ctx); err != nil {
		zapLogger.Fatal("Failed to start SSH monitor", zap.Error(err))
	}

	if err := fileMonitor.Start(ctx); err != nil {
		zapLogger.Fatal("Failed to start file monitor", zap.Error(err))
	}

	if err := cmdMonitor.Start(ctx); err != nil {
		zapLogger.Fatal("Failed to start command monitor", zap.Error(err))
	}

	zapLogger.Info("CoreTrace agent started successfully",
		zap.String("sessions_dir", sessionsDir),
		zap.Int("rotation_max_size_mb", rotationConfig.MaxSize),
		zap.Int("rotation_max_backups", rotationConfig.MaxBackups),
		zap.Int("rotation_max_age_days", rotationConfig.MaxAge),
		zap.Bool("rotation_compress", rotationConfig.Compress))

	// Start event processor with session logger and command monitor
	go processEvents(ctx, zapLogger, sessionLogger, cmdMonitor, sshEventChan, fileEventChan, cmdEventChan)

	// Start session cleanup routine
	cleanupInterval := viper.GetDuration("session_logging.cleanup_interval")
	if cleanupInterval == 0 {
		cleanupInterval = 1 * time.Hour
	}
	maxSessionDuration := viper.GetDuration("session_logging.max_session_duration")
	if maxSessionDuration == 0 {
		maxSessionDuration = 24 * time.Hour
	}
	go sessionCleanupRoutine(ctx, zapLogger, sessionLogger, cleanupInterval, maxSessionDuration)

	// Wait for shutdown signal
	<-sigChan
	zapLogger.Info("Shutting down CoreTrace agent...")

	cancel()

	// Give some time for cleanup
	time.Sleep(2 * time.Second)

	// Stop file monitor
	if err := fileMonitor.Stop(); err != nil {
		zapLogger.Error("Error stopping file monitor", zap.Error(err))
	}

	zapLogger.Info("CoreTrace agent stopped")
}

func processEvents(ctx context.Context, zapLogger *zap.Logger, sessionLogger *logger.SessionLogger, cmdMonitor *monitor.CommandMonitor,
	sshEventChan <-chan types.SSHEvent,
	fileEventChan <-chan types.FileEvent,
	cmdEventChan <-chan types.CommandEvent) {

	for {
		select {
		case <-ctx.Done():
			return
		case sshEvent := <-sshEventChan:
			handleSSHEvent(zapLogger, sessionLogger, cmdMonitor, sshEvent)
		case fileEvent := <-fileEventChan:
			handleFileEvent(zapLogger, sessionLogger, fileEvent)
		case cmdEvent := <-cmdEventChan:
			handleCommandEvent(zapLogger, sessionLogger, cmdEvent)
		}
	}
}

func handleSSHEvent(zapLogger *zap.Logger, sessionLogger *logger.SessionLogger, cmdMonitor *monitor.CommandMonitor, event types.SSHEvent) {
	switch event.EventType {
	case types.EventSSHLogin:
		if event.Success {
			zapLogger.Info("SSH Login detected",
				zap.String("session_id", event.SessionID),
				zap.String("username", event.Username),
				zap.String("source_ip", event.SourceIP),
				zap.String("auth_method", event.AuthMethod),
				zap.String("key_fingerprint", event.KeyFingerprint),
				zap.Int("pid", event.PID),
				zap.Time("timestamp", event.Timestamp),
			)
			// Start session logging
			if err := sessionLogger.StartSession(event); err != nil {
				zapLogger.Error("Failed to start session log", zap.Error(err))
			}
			// Register session PID for command tracking
			if event.PID > 0 {
				cmdMonitor.RegisterSession(event.SessionID, event.PID)
			}
		} else {
			zapLogger.Warn("SSH Login failed",
				zap.String("username", event.Username),
				zap.String("source_ip", event.SourceIP),
				zap.String("auth_method", event.AuthMethod),
				zap.Time("timestamp", event.Timestamp),
			)
		}
	case types.EventSSHLogout:
		zapLogger.Info("SSH Logout detected",
			zap.String("session_id", event.SessionID),
			zap.String("username", event.Username),
			zap.Time("timestamp", event.Timestamp),
		)
		// End session logging
		if err := sessionLogger.EndSession(event.SessionID, event.Timestamp); err != nil {
			zapLogger.Error("Failed to end session log", zap.Error(err))
		}
		// Unregister session from command monitor
		cmdMonitor.UnregisterSession(event.SessionID)
	case types.EventSSHFailed:
		zapLogger.Warn("SSH Failed attempt",
			zap.String("username", event.Username),
			zap.String("source_ip", event.SourceIP),
			zap.Time("timestamp", event.Timestamp),
		)
	}
}

func handleFileEvent(zapLogger *zap.Logger, sessionLogger *logger.SessionLogger, event types.FileEvent) {
	zapLogger.Info("File activity detected",
		zap.String("file_path", event.FilePath),
		zap.String("operation", event.Operation),
		zap.String("username", event.Username),
		zap.String("session_id", event.SessionID),
		zap.Time("timestamp", event.Timestamp),
	)

	// Log to session if associated with a session
	if event.SessionID != "" {
		if err := sessionLogger.LogFileEvent(event.SessionID, event); err != nil {
			// This is expected for events not associated with tracked sessions
			zapLogger.Debug("Failed to log file event to session", zap.Error(err))
		}
	}
}

func handleCommandEvent(zapLogger *zap.Logger, sessionLogger *logger.SessionLogger, event types.CommandEvent) {
	zapLogger.Info("Command executed",
		zap.String("session_id", event.SessionID),
		zap.String("username", event.Username),
		zap.String("command", event.Command),
		zap.Strings("args", event.Args),
		zap.String("cwd", event.WorkingDir),
		zap.Int("pid", event.PID),
		zap.Int("ppid", event.PPID),
		zap.Time("timestamp", event.Timestamp),
	)

	// Log to session if associated with a session
	if event.SessionID != "" {
		if err := sessionLogger.LogCommand(event.SessionID, event); err != nil {
			zapLogger.Debug("Failed to log command to session", zap.Error(err))
		}
	}
}

func sessionCleanupRoutine(ctx context.Context, zapLogger *zap.Logger, sessionLogger *logger.SessionLogger, interval, maxDuration time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sessionLogger.CleanupOldSessions(maxDuration)
			activeSessions := sessionLogger.GetActiveSessions()
			zapLogger.Info("Session cleanup completed", zap.Int("active_sessions", len(activeSessions)))
		}
	}
}
