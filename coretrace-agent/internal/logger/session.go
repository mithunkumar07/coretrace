package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/coretrace/agent/internal/types"
	"go.uber.org/zap"
	"gopkg.in/natefinch/lumberjack.v2"
)

// DiskManagementConfig holds disk space management settings
type DiskManagementConfig struct {
	MaxDirSizeBytes  int64 // Maximum total size of sessions directory in bytes
	DiskThresholdPct int   // Stop logging if disk usage exceeds this percentage
}

// SessionLogger handles writing session logs to disk with rotation
type SessionLogger struct {
	logger           *zap.Logger
	sessionsDir      string
	activeFiles      map[string]*lumberjack.Logger
	sessions         map[string]*types.Session
	mutex            sync.RWMutex
	rotationConfig   RotationConfig
	maxDirSizeBytes  int64 // Maximum total size of sessions directory
	diskThresholdPct int   // Stop logging if disk usage exceeds this percentage
	writeEnabled     bool  // Set to false if disk is full
}

// RotationConfig holds log rotation settings
type RotationConfig struct {
	MaxSize    int  // Maximum size in megabytes before rotation
	MaxBackups int  // Maximum number of old log files to retain
	MaxAge     int  // Maximum number of days to retain old log files
	Compress   bool // Compress rotated files
}

// DefaultRotationConfig returns default rotation settings
func DefaultRotationConfig() RotationConfig {
	return RotationConfig{
		MaxSize:    100, // 100 MB
		MaxBackups: 10,
		MaxAge:     30, // 30 days
		Compress:   true,
	}
}

// NewSessionLogger creates a new session logger
func NewSessionLogger(logger *zap.Logger, sessionsDir string, config RotationConfig, diskConfig ...DiskManagementConfig) (*SessionLogger, error) {
	// Ensure sessions directory exists
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Use provided disk config or defaults
	var diskMgmt DiskManagementConfig
	if len(diskConfig) > 0 {
		diskMgmt = diskConfig[0]
	} else {
		diskMgmt = DiskManagementConfig{
			MaxDirSizeBytes:  10 * 1024 * 1024 * 1024, // 10GB default
			DiskThresholdPct: 90,                      // Stop writing at 90% disk usage
		}
	}

	sl := &SessionLogger{
		logger:           logger,
		sessionsDir:      sessionsDir,
		activeFiles:      make(map[string]*lumberjack.Logger),
		sessions:         make(map[string]*types.Session),
		rotationConfig:   config,
		maxDirSizeBytes:  diskMgmt.MaxDirSizeBytes,
		diskThresholdPct: diskMgmt.DiskThresholdPct,
		writeEnabled:     true,
	}

	// Initial disk space check
	if err := sl.CheckDiskSpace(); err != nil {
		logger.Warn("Failed to check initial disk space", zap.Error(err))
	}

	return sl, nil
}

// StartSession creates a new session log file when a user logs in
func (sl *SessionLogger) StartSession(event types.SSHEvent) error {
	sl.mutex.Lock()
	defer sl.mutex.Unlock()

	// Check if writing is enabled (disk space check)
	if !sl.writeEnabled {
		return fmt.Errorf("session logging disabled due to disk space constraints")
	}

	// Create session object
	session := &types.Session{
		ID:             event.SessionID,
		Username:       event.Username,
		SourceIP:       event.SourceIP,
		Location:       event.Location,
		LoginTime:      event.Timestamp,
		AuthMethod:     event.AuthMethod,
		KeyFingerprint: event.KeyFingerprint,
		PID:            event.PID,
		Commands:       make([]types.CommandEvent, 0),
		FileEvents:     make([]types.FileEvent, 0),
		IsActive:       true,
	}

	sl.sessions[event.SessionID] = session

	// Create log file with rotation support
	logFile := sl.getSessionLogPath(event.SessionID)
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    sl.rotationConfig.MaxSize,    // megabytes
		MaxBackups: sl.rotationConfig.MaxBackups, // number of backups
		MaxAge:     sl.rotationConfig.MaxAge,     // days
		Compress:   sl.rotationConfig.Compress,   // compress rotated files
	}

	sl.activeFiles[event.SessionID] = lumberjackLogger

	// Write initial session info
	if err := sl.writeSessionHeader(lumberjackLogger, session); err != nil {
		return fmt.Errorf("failed to write session header: %w", err)
	}

	sl.logger.Info("Session log started",
		zap.String("session_id", event.SessionID),
		zap.String("username", event.Username),
		zap.String("source_ip", event.SourceIP),
		zap.String("key_fingerprint", event.KeyFingerprint),
		zap.String("log_file", logFile))

	return nil
}

// EndSession marks a session as ended and closes the log file
func (sl *SessionLogger) EndSession(sessionID string, logoutTime time.Time) error {
	sl.mutex.Lock()
	defer sl.mutex.Unlock()

	session, exists := sl.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.IsActive = false
	session.LogoutTime = &logoutTime

	// Write session summary
	if logWriter, exists := sl.activeFiles[sessionID]; exists {
		summary := map[string]interface{}{
			"event_type":       "session_end",
			"timestamp":        logoutTime,
			"session_id":       sessionID,
			"username":         session.Username,
			"login_time":       session.LoginTime,
			"logout_time":      logoutTime,
			"duration_sec":     logoutTime.Sub(session.LoginTime).Seconds(),
			"command_count":    len(session.Commands),
			"file_event_count": len(session.FileEvents),
		}

		data, err := json.Marshal(summary)
		if err != nil {
			sl.logger.Error("Failed to marshal session summary", zap.Error(err))
		} else {
			if _, err := fmt.Fprintln(logWriter, string(data)); err != nil {
				sl.logger.Error("Failed to write session summary", zap.Error(err))
			}
		}

		// Close the log file
		logWriter.Close()
		delete(sl.activeFiles, sessionID)
	}

	delete(sl.sessions, sessionID)

	sl.logger.Info("Session log ended",
		zap.String("session_id", sessionID),
		zap.Time("logout_time", logoutTime))

	return nil
}

// LogCommand records a command execution in the session log
func (sl *SessionLogger) LogCommand(sessionID string, cmd types.CommandEvent) error {
	sl.mutex.Lock()
	defer sl.mutex.Unlock()

	// Silently drop events if writing is disabled
	if !sl.writeEnabled {
		return nil
	}

	session, exists := sl.sessions[sessionID]
	if !exists {
		// Session might not be tracked, log to orphaned file
		return sl.logOrphanedEvent("command", cmd)
	}

	session.Commands = append(session.Commands, cmd)

	if logWriter, exists := sl.activeFiles[sessionID]; exists {
		data, err := json.Marshal(cmd)
		if err != nil {
			sl.logger.Error("Failed to marshal command", zap.Error(err))
			return err
		}
		if _, err := fmt.Fprintln(logWriter, string(data)); err != nil {
			sl.logger.Error("Failed to write command", zap.Error(err))
			return err
		}
	}

	return nil
}

// LogFileEvent records a file operation in the session log
func (sl *SessionLogger) LogFileEvent(sessionID string, event types.FileEvent) error {
	sl.mutex.Lock()
	defer sl.mutex.Unlock()

	// Silently drop events if writing is disabled
	if !sl.writeEnabled {
		return nil
	}

	session, exists := sl.sessions[sessionID]
	if !exists {
		// Session might not be tracked, log to orphaned file
		return sl.logOrphanedEvent("file_event", event)
	}

	session.FileEvents = append(session.FileEvents, event)

	if logWriter, exists := sl.activeFiles[sessionID]; exists {
		data, err := json.Marshal(event)
		if err != nil {
			sl.logger.Error("Failed to marshal file event", zap.Error(err))
			return err
		}
		if _, err := fmt.Fprintln(logWriter, string(data)); err != nil {
			sl.logger.Error("Failed to write file event", zap.Error(err))
			return err
		}
	}

	return nil
}

// writeSessionHeader writes the initial session metadata to the log
func (sl *SessionLogger) writeSessionHeader(writer *lumberjack.Logger, session *types.Session) error {
	header := map[string]interface{}{
		"event_type":      "session_start",
		"timestamp":       session.LoginTime,
		"session_id":      session.ID,
		"username":        session.Username,
		"source_ip":       session.SourceIP,
		"auth_method":     session.AuthMethod,
		"key_fingerprint": session.KeyFingerprint,
		"location":        session.Location,
		"pid":             session.PID,
	}

	data, err := json.Marshal(header)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(writer, string(data))
	return err
}

// logOrphanedEvent logs events that can't be associated with a known session
func (sl *SessionLogger) logOrphanedEvent(eventType string, event interface{}) error {
	sl.mutex.Lock()
	defer sl.mutex.Unlock()

	// Reuse orphaned events logger if already created
	orphanedLog := filepath.Join(sl.sessionsDir, "orphaned_events.log")
	var orphanedWriter *lumberjack.Logger

	// Check if we already have an orphaned events writer
	if writer, exists := sl.activeFiles["__orphaned__"]; exists {
		orphanedWriter = writer
	} else {
		orphanedWriter = &lumberjack.Logger{
			Filename:   orphanedLog,
			MaxSize:    sl.rotationConfig.MaxSize,
			MaxBackups: sl.rotationConfig.MaxBackups,
			MaxAge:     sl.rotationConfig.MaxAge,
			Compress:   sl.rotationConfig.Compress,
		}
		sl.activeFiles["__orphaned__"] = orphanedWriter
	}

	data, err := json.Marshal(map[string]interface{}{
		"event_type": eventType,
		"data":       event,
		"logged_at":  time.Now(),
	})
	if err != nil {
		sl.logger.Error("Failed to marshal orphaned event", zap.Error(err))
		return err
	}

	_, err = fmt.Fprintln(orphanedWriter, string(data))
	if err != nil {
		sl.logger.Error("Failed to write orphaned event", zap.Error(err))
	}
	return err
}

// getSessionLogPath generates the log file path for a session
func (sl *SessionLogger) getSessionLogPath(sessionID string) string {
	// Create date-based subdirectory
	dateDir := time.Now().Format("2006-01-02")
	sessionDir := filepath.Join(sl.sessionsDir, dateDir)
	os.MkdirAll(sessionDir, 0755)

	return filepath.Join(sessionDir, fmt.Sprintf("%s.jsonl", sessionID))
}

// GetActiveSessions returns a list of currently active sessions
func (sl *SessionLogger) GetActiveSessions() []*types.Session {
	sl.mutex.RLock()
	defer sl.mutex.RUnlock()

	sessions := make([]*types.Session, 0, len(sl.sessions))
	for _, session := range sl.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// Close closes all active log files
func (sl *SessionLogger) Close() {
	sl.mutex.Lock()
	defer sl.mutex.Unlock()

	for sessionID, logWriter := range sl.activeFiles {
		if sessionID == "__orphaned__" {
			sl.logger.Info("Closed orphaned events log")
		} else {
			sl.logger.Info("Closed session log", zap.String("session_id", sessionID))
		}
		logWriter.Close()
	}
	sl.activeFiles = make(map[string]*lumberjack.Logger)
}

// CleanupOldSessions removes stale sessions that have been active too long
func (sl *SessionLogger) CleanupOldSessions(maxDuration time.Duration) {
	sl.mutex.Lock()
	defer sl.mutex.Unlock()

	now := time.Now()
	for sessionID, session := range sl.sessions {
		if session.IsActive && now.Sub(session.LoginTime) > maxDuration {
			session.IsActive = false
			if logWriter, exists := sl.activeFiles[sessionID]; exists {
				summary := map[string]interface{}{
					"event_type":   "session_timeout",
					"timestamp":    now,
					"session_id":   sessionID,
					"reason":       "max_duration_exceeded",
					"max_duration": maxDuration.String(),
				}
				data, err := json.Marshal(summary)
				if err != nil {
					sl.logger.Error("Failed to marshal timeout summary", zap.Error(err))
				} else {
					if _, err := fmt.Fprintln(logWriter, string(data)); err != nil {
						sl.logger.Error("Failed to write timeout summary", zap.Error(err))
					}
				}
				logWriter.Close()
				delete(sl.activeFiles, sessionID)
			}
			delete(sl.sessions, sessionID)
			sl.logger.Warn("Session cleaned up due to timeout",
				zap.String("session_id", sessionID),
				zap.Duration("duration", now.Sub(session.LoginTime)))
		}
	}
}

// CheckDiskSpace checks available disk space and disables writing if threshold exceeded
func (sl *SessionLogger) CheckDiskSpace() error {
	sl.mutex.Lock()
	defer sl.mutex.Unlock()

	// Get disk usage for the sessions directory
	var stat syscall.Statfs_t
	if err := syscall.Statfs(sl.sessionsDir, &stat); err != nil {
		return fmt.Errorf("failed to stat sessions directory: %w", err)
	}

	// Calculate disk usage percentage
	total := stat.Blocks * uint64(stat.Bsize)
	available := stat.Bavail * uint64(stat.Bsize)
	used := total - available
	usagePct := int((float64(used) / float64(total)) * 100)

	// Check if we've exceeded the threshold
	if sl.diskThresholdPct > 0 && usagePct >= sl.diskThresholdPct {
		if sl.writeEnabled {
			sl.logger.Error("Disk space threshold exceeded, disabling session logging",
				zap.Int("usage_percent", usagePct),
				zap.Int("threshold_percent", sl.diskThresholdPct),
				zap.String("sessions_dir", sl.sessionsDir))
			sl.writeEnabled = false
		}
	} else {
		// Re-enable if we're back under threshold (with 5% hysteresis)
		if !sl.writeEnabled && usagePct < (sl.diskThresholdPct-5) {
			sl.logger.Info("Disk space recovered, re-enabling session logging",
				zap.Int("usage_percent", usagePct))
			sl.writeEnabled = true
		}
	}

	return nil
}

// CleanupOldDirectories removes session directories older than retention days
func (sl *SessionLogger) CleanupOldDirectories(retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}

	entries, err := os.ReadDir(sl.sessionsDir)
	if err != nil {
		return fmt.Errorf("failed to read sessions directory: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	deleted := 0
	freedBytes := int64(0)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Parse directory name as date (YYYY-MM-DD format)
		dirDate, err := time.Parse("2006-01-02", entry.Name())
		if err != nil {
			continue // Skip non-date directories
		}

		if dirDate.Before(cutoff) {
			dirPath := filepath.Join(sl.sessionsDir, entry.Name())
			size := sl.getDirSize(dirPath)

			if err := os.RemoveAll(dirPath); err != nil {
				sl.logger.Warn("Failed to remove old session directory",
					zap.String("path", dirPath),
					zap.Error(err))
			} else {
				deleted++
				freedBytes += size
				sl.logger.Info("Removed old session directory",
					zap.String("path", dirPath),
					zap.Int64("freed_bytes", size))
			}
		}
	}

	sl.logger.Info("Directory cleanup completed",
		zap.Int("directories_removed", deleted),
		zap.Int64("total_freed_bytes", freedBytes))

	return nil
}

// getDirSize calculates the total size of a directory
func (sl *SessionLogger) getDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// GetDirectorySize returns the total size of the sessions directory
func (sl *SessionLogger) GetDirectorySize() int64 {
	return sl.getDirSize(sl.sessionsDir)
}

// IsWriteEnabled returns true if writing is currently enabled
func (sl *SessionLogger) IsWriteEnabled() bool {
	sl.mutex.RLock()
	defer sl.mutex.RUnlock()
	return sl.writeEnabled
}
