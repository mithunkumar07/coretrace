package monitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/coretrace/agent/internal/types"
	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

type FileMonitor struct {
	logger       *zap.Logger
	eventChan    chan<- types.FileEvent
	watcher      *fsnotify.Watcher
	sessionMap   map[string]string // PID -> SessionID mapping
	watchedPaths map[string]bool
}

func NewFileMonitor(logger *zap.Logger, eventChan chan<- types.FileEvent) (*FileMonitor, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	return &FileMonitor{
		logger:       logger,
		eventChan:    eventChan,
		watcher:      watcher,
		sessionMap:   make(map[string]string),
		watchedPaths: make(map[string]bool),
	}, nil
}

func (fm *FileMonitor) Start(ctx context.Context) error {
	fm.logger.Info("Starting file monitor")

	go fm.watchLoop(ctx)

	// Start monitoring common directories
	watchDirs := []string{
		"/etc",
		"/home",
		"/root",
		"/var/log",
		"/opt",
		"/usr/local/bin",
		"/tmp",
		"/var/tmp",
	}

	for _, dir := range watchDirs {
		if err := fm.addWatchRecursive(dir); err != nil {
			fm.logger.Warn("Failed to watch directory", zap.String("dir", dir), zap.Error(err))
		}
	}

	return nil
}

func (fm *FileMonitor) Stop() error {
	return fm.watcher.Close()
}

func (fm *FileMonitor) watchLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fm.watcher.Events:
			if !ok {
				return
			}
			fm.handleFileEvent(event)
		case err, ok := <-fm.watcher.Errors:
			if !ok {
				return
			}
			fm.logger.Error("File watcher error", zap.Error(err))
		}
	}
}

func (fm *FileMonitor) handleFileEvent(event fsnotify.Event) {
	var eventType types.EventType
	var operation string

	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		eventType = types.EventFileChange
		operation = "create"
	case event.Op&fsnotify.Write == fsnotify.Write:
		eventType = types.EventFileChange
		operation = "write"
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		eventType = types.EventFileChange
		operation = "delete"
	case event.Op&fsnotify.Chmod == fsnotify.Chmod:
		eventType = types.EventFileChange
		operation = "chmod"
	default:
		return // Ignore other operations
	}

	fileInfo, err := os.Stat(event.Name)
	if err != nil && operation != "delete" {
		return
	}

	var size int64
	var permissions string

	if operation != "delete" {
		size = fileInfo.Size()
		permissions = fmt.Sprintf("%o", fileInfo.Mode().Perm())
	}

	sessionID, username := fm.getSessionAndUser()

	fileEvent := types.FileEvent{
		Timestamp:   time.Now(),
		EventType:   eventType,
		SessionID:   sessionID,
		Username:    username,
		FilePath:    event.Name,
		Operation:   operation,
		Size:        size,
		Permissions: permissions,
	}

	if operation == "create" && fileInfo != nil {
		// For new files, also add recursive watch if it's a directory
		if fileInfo.IsDir() {
			fm.addWatchRecursive(event.Name)
		}
	}

	select {
	case fm.eventChan <- fileEvent:
		fm.logger.Debug("File event sent",
			zap.String("file", event.Name),
			zap.String("operation", operation),
			zap.String("session_id", sessionID))
	default:
		fm.logger.Warn("Event channel full, dropping file event")
	}
}

func (fm *FileMonitor) addWatchRecursive(path string) error {
	return filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			// Skip temporary and cache directories
			if strings.Contains(walkPath, ".git") ||
				strings.Contains(walkPath, "node_modules") ||
				strings.Contains(walkPath, ".cache") {
				return filepath.SkipDir
			}

			if !fm.watchedPaths[walkPath] {
				if err := fm.watcher.Add(walkPath); err != nil {
					fm.logger.Warn("Failed to watch path", zap.String("path", walkPath), zap.Error(err))
				} else {
					fm.watchedPaths[walkPath] = true
					fm.logger.Debug("Watching path", zap.String("path", walkPath))
				}
			}
		}
		return nil
	})
}

func (fm *FileMonitor) UpdateSessionMapping(sessionID string, pid int) {
	fm.sessionMap[fmt.Sprintf("%d", pid)] = sessionID
}

func (fm *FileMonitor) getSessionAndUser() (string, string) {
	// Get parent process info
	ppid := os.Getppid()

	// Try to find session from parent process
	if sessionID, exists := fm.sessionMap[fmt.Sprintf("%d", ppid)]; exists {
		return sessionID, fm.getUsernameForPid(ppid)
	}

	// Fallback to getting current user
	return "", fm.getCurrentUser()
}

func (fm *FileMonitor) getUsernameForPid(pid int) string {
	// Read /proc/<pid>/status to get the user
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	if data, err := os.ReadFile(statusPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Uid:") {
				parts := strings.Fields(line)
				if len(parts) > 1 {
					// Get username from UID
					if uid := parts[1]; uid != "0" {
						if username, err := fm.getUsernameFromUID(uid); err == nil {
							return username
						}
					}
					return "root"
				}
			}
		}
	}
	return "unknown"
}

func (fm *FileMonitor) getCurrentUser() string {
	if syscall.Getuid() == 0 {
		return "root"
	}

	// Try to get username from environment
	if user := os.Getenv("USER"); user != "" {
		return user
	}

	return "unknown"
}

func (fm *FileMonitor) getUsernameFromUID(uid string) (string, error) {
	// Simple implementation - could use /etc/passwd parsing
	return "user", nil
}

func (fm *FileMonitor) CleanupOldSessions() {
	// This would be called periodically to clean up old PID mappings
	for pid := range fm.sessionMap {
		if !fm.pidExists(pid) {
			delete(fm.sessionMap, pid)
		}
	}
}

func (fm *FileMonitor) pidExists(pid string) bool {
	if _, err := os.Stat("/proc/" + pid); err != nil {
		return false
	}
	return true
}
