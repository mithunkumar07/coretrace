package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coretrace/agent/internal/dashboard"
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
	zapLogger, err := logger.NewLoggerFromConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer zapLogger.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sshEventChan := make(chan types.SSHEvent, 1000)
	fileEventChan := make(chan types.FileEvent, 1000)
	cmdEventChan := make(chan types.CommandEvent, 1000)

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
	if rotationConfig.MaxSize == 0 {
		rotationConfig.MaxSize = 100
	}
	if rotationConfig.MaxBackups == 0 {
		rotationConfig.MaxBackups = 10
	}
	if rotationConfig.MaxAge == 0 {
		rotationConfig.MaxAge = 30
	}

	diskConfig := logger.DiskManagementConfig{
		MaxDirSizeBytes:  int64(viper.GetInt("disk_management.max_sessions_size_gb")) * 1024 * 1024 * 1024,
		DiskThresholdPct: viper.GetInt("disk_management.usage_threshold_percent"),
	}
	if diskConfig.MaxDirSizeBytes == 0 {
		diskConfig.MaxDirSizeBytes = 10 * 1024 * 1024 * 1024
	}
	if diskConfig.DiskThresholdPct == 0 {
		diskConfig.DiskThresholdPct = 90
	}

	sessionLogger, err := logger.NewSessionLogger(zapLogger, sessionsDir, rotationConfig, diskConfig)
	if err != nil {
		zapLogger.Fatal("Failed to create session logger", zap.Error(err))
	}
	defer sessionLogger.Close()

	// Dashboard client — optional, wired when dashboard.enabled + url + api_key are set
	var dashClient *dashboard.Client
	if viper.GetBool("dashboard.enabled") {
		dashURL := viper.GetString("dashboard.url")
		dashAPIKey := viper.GetString("dashboard.api_key")
		if dashURL != "" && dashAPIKey != "" {
			agentID := viper.GetString("dashboard.agent_id")
			if agentID == "" {
				agentID, _ = os.Hostname()
			}
			dashClient = dashboard.NewClient(zapLogger, dashURL, dashAPIKey, agentID)
			if err := dashClient.Start(ctx); err != nil {
				zapLogger.Error("Failed to start dashboard client", zap.Error(err))
				dashClient = nil
			} else {
				zapLogger.Info("Dashboard client started",
					zap.String("url", dashURL),
					zap.String("agent_id", agentID))
				defer dashClient.Stop()
				go applyConfigUpdates(ctx, zapLogger, dashClient.ConfigUpdates())
			}
		} else {
			zapLogger.Warn("Dashboard enabled but url/api_key missing, skipping")
		}
	}

	sshMonitor := monitor.NewSSHMonitor(zapLogger, sshEventChan)
	fileMonitor, err := monitor.NewFileMonitor(zapLogger, fileEventChan)
	if err != nil {
		zapLogger.Fatal("Failed to create file monitor", zap.Error(err))
	}
	debug := viper.GetBool("agent.debug")
	cmdMonitor := monitor.NewCommandMonitor(zapLogger, cmdEventChan, debug)

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

	go processEvents(ctx, zapLogger, sessionLogger, cmdMonitor, dashClient, sshEventChan, fileEventChan, cmdEventChan)

	cleanupInterval := viper.GetDuration("session_logging.cleanup_interval")
	if cleanupInterval == 0 {
		cleanupInterval = 1 * time.Hour
	}
	maxSessionDuration := viper.GetDuration("session_logging.max_session_duration")
	if maxSessionDuration == 0 {
		maxSessionDuration = 24 * time.Hour
	}
	go sessionCleanupRoutine(ctx, zapLogger, sessionLogger, cleanupInterval, maxSessionDuration)

	<-sigChan
	zapLogger.Info("Shutting down CoreTrace agent...")

	cancel()
	time.Sleep(2 * time.Second)

	if err := fileMonitor.Stop(); err != nil {
		zapLogger.Error("Error stopping file monitor", zap.Error(err))
	}
	if err := cmdMonitor.Stop(); err != nil {
		zapLogger.Error("Error stopping command monitor", zap.Error(err))
	}

	zapLogger.Info("CoreTrace agent stopped")
}

func processEvents(
	ctx context.Context,
	zapLogger *zap.Logger,
	sessionLogger *logger.SessionLogger,
	cmdMonitor *monitor.CommandMonitor,
	dashClient *dashboard.Client,
	sshEventChan <-chan types.SSHEvent,
	fileEventChan <-chan types.FileEvent,
	cmdEventChan <-chan types.CommandEvent,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case sshEvent := <-sshEventChan:
			handleSSHEvent(zapLogger, sessionLogger, cmdMonitor, dashClient, sshEvent)
		case fileEvent := <-fileEventChan:
			handleFileEvent(zapLogger, sessionLogger, dashClient, fileEvent)
		case cmdEvent := <-cmdEventChan:
			handleCommandEvent(zapLogger, sessionLogger, dashClient, cmdEvent)
		}
	}
}

func handleSSHEvent(
	zapLogger *zap.Logger,
	sessionLogger *logger.SessionLogger,
	cmdMonitor *monitor.CommandMonitor,
	dashClient *dashboard.Client,
	event types.SSHEvent,
) {
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
			if err := sessionLogger.StartSession(event); err != nil {
				zapLogger.Error("Failed to start session log", zap.Error(err))
			}
			if event.PID > 0 {
				cmdMonitor.RegisterSession(event.SessionID, event.PID)
			}
			if dashClient != nil {
				dashClient.SendEvent("ssh_login", event.SessionID, map[string]interface{}{
					"username":        event.Username,
					"source_ip":       event.SourceIP,
					"auth_method":     event.AuthMethod,
					"key_fingerprint": event.KeyFingerprint,
					"pid":             event.PID,
				}, "info")
			}
		} else {
			zapLogger.Warn("SSH Login failed",
				zap.String("username", event.Username),
				zap.String("source_ip", event.SourceIP),
				zap.String("auth_method", event.AuthMethod),
				zap.Time("timestamp", event.Timestamp),
			)
			if dashClient != nil {
				dashClient.SendEvent("ssh_failed", event.SessionID, map[string]interface{}{
					"username":    event.Username,
					"source_ip":   event.SourceIP,
					"auth_method": event.AuthMethod,
				}, "warning")
			}
		}

	case types.EventSSHLogout:
		zapLogger.Info("SSH Logout detected",
			zap.String("session_id", event.SessionID),
			zap.String("username", event.Username),
			zap.Time("timestamp", event.Timestamp),
		)
		if err := sessionLogger.EndSession(event.SessionID, event.Timestamp); err != nil {
			zapLogger.Error("Failed to end session log", zap.Error(err))
		}
		cmdMonitor.UnregisterSession(event.SessionID)
		if dashClient != nil {
			dashClient.SendEvent("ssh_logout", event.SessionID, map[string]interface{}{
				"username": event.Username,
			}, "info")
		}

	case types.EventSSHFailed:
		zapLogger.Warn("SSH Failed attempt",
			zap.String("username", event.Username),
			zap.String("source_ip", event.SourceIP),
			zap.Time("timestamp", event.Timestamp),
		)
		if dashClient != nil {
			dashClient.SendEvent("ssh_failed", "", map[string]interface{}{
				"username":  event.Username,
				"source_ip": event.SourceIP,
			}, "warning")
		}
	}
}

func handleFileEvent(
	zapLogger *zap.Logger,
	sessionLogger *logger.SessionLogger,
	dashClient *dashboard.Client,
	event types.FileEvent,
) {
	zapLogger.Info("File activity detected",
		zap.String("file_path", event.FilePath),
		zap.String("operation", event.Operation),
		zap.String("username", event.Username),
		zap.String("session_id", event.SessionID),
		zap.Time("timestamp", event.Timestamp),
	)
	if event.SessionID != "" {
		if err := sessionLogger.LogFileEvent(event.SessionID, event); err != nil {
			zapLogger.Debug("Failed to log file event to session", zap.Error(err))
		}
	}
	if dashClient != nil {
		eventType := "file_" + event.Operation
		dashClient.SendEvent(eventType, event.SessionID, map[string]interface{}{
			"file_path": event.FilePath,
			"operation": event.Operation,
			"username":  event.Username,
		}, "info")
	}
}

func handleCommandEvent(
	zapLogger *zap.Logger,
	sessionLogger *logger.SessionLogger,
	dashClient *dashboard.Client,
	event types.CommandEvent,
) {
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
	if event.SessionID != "" {
		if err := sessionLogger.LogCommand(event.SessionID, event); err != nil {
			zapLogger.Debug("Failed to log command to session", zap.Error(err))
		}
	}
	if dashClient != nil {
		dashClient.SendEvent("command", event.SessionID, map[string]interface{}{
			"username":    event.Username,
			"command":     event.Command,
			"args":        event.Args,
			"working_dir": event.WorkingDir,
			"pid":         event.PID,
			"ppid":        event.PPID,
		}, "info")
	}
}

// applyConfigUpdates listens for config payloads from the dashboard and applies them via viper.
func applyConfigUpdates(ctx context.Context, zapLogger *zap.Logger, updates <-chan map[string]interface{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case cfg, ok := <-updates:
			if !ok {
				return
			}
			for key, value := range cfg {
				viper.Set(key, value)
				zapLogger.Info("Config updated from dashboard",
					zap.String("key", key),
					zap.Any("value", value))
			}
		}
	}
}

func sessionCleanupRoutine(ctx context.Context, zapLogger *zap.Logger, sessionLogger *logger.SessionLogger, interval, maxDuration time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	diskCheckTicker := time.NewTicker(5 * time.Minute)
	defer diskCheckTicker.Stop()

	dirCleanupTicker := time.NewTicker(24 * time.Hour)
	defer dirCleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sessionLogger.CleanupOldSessions(maxDuration)
			activeSessions := sessionLogger.GetActiveSessions()
			dirSize := sessionLogger.GetDirectorySize()
			zapLogger.Info("Session cleanup completed",
				zap.Int("active_sessions", len(activeSessions)),
				zap.Int64("sessions_dir_size_bytes", dirSize),
				zap.Bool("write_enabled", sessionLogger.IsWriteEnabled()))
		case <-diskCheckTicker.C:
			if err := sessionLogger.CheckDiskSpace(); err != nil {
				zapLogger.Error("Failed to check disk space", zap.Error(err))
			}
		case <-dirCleanupTicker.C:
			retentionDays := viper.GetInt("session_logging.retention_days")
			if retentionDays == 0 {
				retentionDays = 30
			}
			if err := sessionLogger.CleanupOldDirectories(retentionDays); err != nil {
				zapLogger.Error("Failed to cleanup old directories", zap.Error(err))
			}
		}
	}
}
