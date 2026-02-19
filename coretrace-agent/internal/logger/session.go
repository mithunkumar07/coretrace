package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/coretrace/agent/internal/types"
	"go.uber.org/zap"
	"gopkg.in/natefinch/lumberjack.v2"
)

// SessionLogger handles writing session logs to disk with rotation
type SessionLogger struct {
	logger         *zap.Logger
	sessionsDir    string
	activeFiles    map[string]*lumberjack.Logger
	sessions       map[string]*types.Session
	mutex          sync.RWMutex
	rotationConfig RotationConfig
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
func NewSessionLogger(logger *zap.Logger, sessionsDir string, config RotationConfig) (*SessionLogger, error) {
	// Ensure sessions directory exists
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	sl := &SessionLogger{
		logger:         logger,
		sessionsDir:    sessionsDir,
		activeFiles:    make(map[string]*lumberjack.Logger),
		sessions:       make(map[string]*types.Session),
		rotationConfig: config,
	}

	return sl, nil
}

// StartSession creates a new session log file when a user logs in
func (sl *SessionLogger) StartSession(event types.SSHEvent) error {
	sl.mutex.Lock()
	defer sl.mutex.Unlock()

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

		data, _ := json.Marshal(summary)
		fmt.Fprintln(logWriter, string(data))

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

	session, exists := sl.sessions[sessionID]
	if !exists {
		// Session might not be tracked, log to orphaned file
		return sl.logOrphanedEvent("command", cmd)
	}

	session.Commands = append(session.Commands, cmd)

	if logWriter, exists := sl.activeFiles[sessionID]; exists {
		data, _ := json.Marshal(cmd)
		fmt.Fprintln(logWriter, string(data))
	}

	return nil
}

// LogFileEvent records a file operation in the session log
func (sl *SessionLogger) LogFileEvent(sessionID string, event types.FileEvent) error {
	sl.mutex.Lock()
	defer sl.mutex.Unlock()

	session, exists := sl.sessions[sessionID]
	if !exists {
		// Session might not be tracked, log to orphaned file
		return sl.logOrphanedEvent("file_event", event)
	}

	session.FileEvents = append(session.FileEvents, event)

	if logWriter, exists := sl.activeFiles[sessionID]; exists {
		data, _ := json.Marshal(event)
		fmt.Fprintln(logWriter, string(data))
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
	orphanedLog := filepath.Join(sl.sessionsDir, "orphaned_events.log")

	lumberjackLogger := &lumberjack.Logger{
		Filename:   orphanedLog,
		MaxSize:    sl.rotationConfig.MaxSize,
		MaxBackups: sl.rotationConfig.MaxBackups,
		MaxAge:     sl.rotationConfig.MaxAge,
		Compress:   sl.rotationConfig.Compress,
	}
	defer lumberjackLogger.Close()

	data, _ := json.Marshal(map[string]interface{}{
		"event_type": eventType,
		"data":       event,
		"logged_at":  time.Now(),
	})

	_, err := fmt.Fprintln(lumberjackLogger, string(data))
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
		logWriter.Close()
		sl.logger.Info("Closed session log", zap.String("session_id", sessionID))
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
				data, _ := json.Marshal(summary)
				fmt.Fprintln(logWriter, string(data))
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
