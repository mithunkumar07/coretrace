package monitor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coretrace/agent/internal/types"
	"go.uber.org/zap"
)

// auditdMonitor monitors auditd logs for command execution (fallback when eBPF unavailable)
type auditdMonitor struct {
	logger     *zap.Logger
	eventChan  chan<- types.CommandEvent
	sessionMap map[int]string // PID -> SessionID mapping
	ppidCache  map[int]int    // PID -> PPID mapping for quick lookup
	debug      bool
	stopChan   chan struct{}
}

// newAuditdMonitor creates a new auditd-based command monitor
func newAuditdMonitor(logger *zap.Logger, eventChan chan<- types.CommandEvent, debug bool) *auditdMonitor {
	return &auditdMonitor{
		logger:     logger,
		eventChan:  eventChan,
		sessionMap: make(map[int]string),
		ppidCache:  make(map[int]int),
		debug:      debug,
		stopChan:   make(chan struct{}),
	}
}

// Auditd log parsing regex patterns
var (
	syscallRegex = regexp.MustCompile(`type=SYSCALL msg=audit\(([\d.]+):(\d+)\):.*syscall=(\d+).*ppid=(\d+) pid=(\d+) auid=(\d+) uid=(\d+).*exe="([^"]+)".*key="([^"]+)"`)
	execveRegex  = regexp.MustCompile(`type=EXECVE msg=audit\(([\d.]+):(\d+)\): argc=(\d+) (.*)`)
	cwdRegex     = regexp.MustCompile(`type=CWD msg=audit\(([\d.]+):(\d+)\): cwd="([^"]*)"`)
	pathRegex    = regexp.MustCompile(`type=PATH msg=audit\(([\d.]+):(\d+)\): item=\d+ name="([^"]+)"`)
)

const (
	syscallExecve   = 59
	syscallExecveAt = 322
)

type PendingCommand struct {
	Timestamp  time.Time
	AuditID    string
	PID        int
	PPID       int
	UID        int
	AUID       int
	Exe        string
	CWD        string
	Args       []string
	Argc       int
	ReceivedAt time.Time
}

// Start begins monitoring auditd logs
func (cm *auditdMonitor) Start(ctx context.Context) error {
	cm.logger.Info("Starting auditd-based command monitor")

	if !cm.isAuditdRunning() {
		cm.logger.Warn("auditd service not detected")
		return fmt.Errorf("auditd not available")
	}

	go cm.monitorAuditLog(ctx)

	return nil
}

// Stop stops the auditd monitor
func (cm *auditdMonitor) Stop() error {
	close(cm.stopChan)
	return nil
}

func (cm *auditdMonitor) isAuditdRunning() bool {
	if _, err := os.Stat("/var/log/audit/audit.log"); err == nil {
		return true
	}
	if _, err := os.Stat("/var/log/audit.log"); err == nil {
		return true
	}
	return false
}

func (cm *auditdMonitor) monitorAuditLog(ctx context.Context) {
	auditLogFile := "/var/log/audit/audit.log"
	if _, err := os.Stat(auditLogFile); err != nil {
		auditLogFile = "/var/log/audit.log"
		if _, err := os.Stat(auditLogFile); err != nil {
			cm.logger.Error("Audit log file not found", zap.Error(err))
			return
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopChan:
			return
		default:
			if err := cm.tailAuditLog(ctx, auditLogFile); err != nil {
				cm.logger.Error("Error tailing audit log", zap.Error(err))
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (cm *auditdMonitor) tailAuditLog(ctx context.Context, auditLogFile string) error {
	file, err := os.Open(auditLogFile)
	if err != nil {
		return fmt.Errorf("failed to open audit log: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	file.Seek(0, 2)

	pendingCommands := make(map[string]*PendingCommand)
	cleanupTicker := time.NewTicker(30 * time.Second)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-cm.stopChan:
			return nil
		case <-cleanupTicker.C:
			cm.cleanupPendingCommands(pendingCommands)
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				// Check if file was rotated
				if os.IsNotExist(err) {
					cm.logger.Info("Audit log rotated, reopening")
					return nil
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			cm.parseAuditLine(line, pendingCommands)
		}
	}
}

func (cm *auditdMonitor) parseAuditLine(line string, pending map[string]*PendingCommand) {
	auditID := cm.extractAuditID(line)
	if auditID == "" {
		return
	}

	if strings.Contains(line, "type=SYSCALL") {
		if matches := syscallRegex.FindStringSubmatch(line); matches != nil {
			syscallNum, _ := strconv.Atoi(matches[3])
			if syscallNum != syscallExecve && syscallNum != syscallExecveAt {
				return
			}

			timestamp, _ := strconv.ParseFloat(matches[1], 64)
			ppid, _ := strconv.Atoi(matches[4])
			pid, _ := strconv.Atoi(matches[5])
			auid, _ := strconv.Atoi(matches[6])
			uid, _ := strconv.Atoi(matches[7])
			exe := matches[8]
			key := matches[9]

			if key != "session_monitoring" && !cm.isTrackedPID(pid) {
				return
			}

			pending[auditID] = &PendingCommand{
				Timestamp:  time.Unix(int64(timestamp), 0),
				AuditID:    auditID,
				PID:        pid,
				PPID:       ppid,
				UID:        uid,
				AUID:       auid,
				Exe:        exe,
				ReceivedAt: time.Now(),
			}

			cm.ppidCache[pid] = ppid
		}
		return
	}

	cmd, exists := pending[auditID]
	if !exists {
		return
	}

	if strings.Contains(line, "type=EXECVE") {
		if matches := execveRegex.FindStringSubmatch(line); matches != nil {
			argc, _ := strconv.Atoi(matches[3])
			cmd.Argc = argc
			argsStr := matches[4]
			cmd.Args = cm.parseExecveArgs(argsStr, argc)
		}
		return
	}

	if strings.Contains(line, "type=CWD") {
		if matches := cwdRegex.FindStringSubmatch(line); matches != nil {
			cmd.CWD = matches[3]
		}
		return
	}

	if strings.Contains(line, "type=PATH") && strings.Contains(line, "item=0") {
		if matches := pathRegex.FindStringSubmatch(line); matches != nil {
			if strings.HasPrefix(matches[3], "/") {
				cmd.Exe = matches[3]
			}
		}

		if cmd.CWD != "" && len(cmd.Args) > 0 {
			cm.emitCommand(cmd)
			delete(pending, auditID)
		}
		return
	}
}

func (cm *auditdMonitor) extractAuditID(line string) string {
	start := strings.Index(line, "audit(")
	if start == -1 {
		return ""
	}
	start += 6
	end := strings.Index(line[start:], ")")
	if end == -1 {
		return ""
	}
	return line[start : start+end]
}

func (cm *auditdMonitor) parseExecveArgs(argsStr string, argc int) []string {
	args := make([]string, 0, argc)
	argRegex := regexp.MustCompile(`a(\d+)="((?:[^"\\]|\\.)*)"`)
	matches := argRegex.FindAllStringSubmatch(argsStr, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			arg := strings.ReplaceAll(match[2], `\"`, `"`)
			arg = strings.ReplaceAll(arg, `\\`, `\`)
			args = append(args, arg)
		}
	}

	return args
}

func (cm *auditdMonitor) isTrackedPID(pid int) bool {
	currentPID := pid
	visited := make(map[int]bool)

	for depth := 0; depth < 20; depth++ {
		if _, tracked := cm.sessionMap[currentPID]; tracked {
			return true
		}

		ppid, exists := cm.ppidCache[currentPID]
		if !exists {
			ppid = cm.getPPIDFromProc(currentPID)
			if ppid <= 0 {
				break
			}
		}

		if visited[ppid] {
			break
		}
		visited[ppid] = true
		currentPID = ppid
	}

	return false
}

func (cm *auditdMonitor) getPPIDFromProc(pid int) int {
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return -1
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "PPid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				ppid, _ := strconv.Atoi(fields[1])
				return ppid
			}
		}
	}
	return -1
}

func (cm *auditdMonitor) emitCommand(pending *PendingCommand) {
	sessionID := cm.findSessionID(pending.PID)
	username := cm.getUsernameFromUID(pending.UID)

	command := ""
	if len(pending.Args) > 0 {
		command = pending.Args[0]
	} else {
		command = pending.Exe
	}

	event := types.CommandEvent{
		Timestamp:  pending.Timestamp,
		EventType:  types.EventCommand,
		SessionID:  sessionID,
		Username:   username,
		PID:        pending.PID,
		PPID:       pending.PPID,
		Command:    command,
		Args:       pending.Args,
		WorkingDir: pending.CWD,
		ExitCode:   0,
	}

	select {
	case cm.eventChan <- event:
		cm.logger.Debug("Command event emitted (auditd)",
			zap.String("session_id", sessionID),
			zap.String("command", command),
			zap.Int("pid", pending.PID))
	default:
		cm.logger.Warn("Command event channel full, dropping event")
	}
}

func (cm *auditdMonitor) findSessionID(pid int) string {
	currentPID := pid
	visited := make(map[int]bool)

	for depth := 0; depth < 20; depth++ {
		if sessionID, tracked := cm.sessionMap[currentPID]; tracked {
			return sessionID
		}

		ppid, exists := cm.ppidCache[currentPID]
		if !exists {
			ppid = cm.getPPIDFromProc(currentPID)
			if ppid <= 0 {
				break
			}
			cm.ppidCache[currentPID] = ppid
		}

		if visited[ppid] {
			break
		}
		visited[ppid] = true
		currentPID = ppid
	}

	return ""
}

func (cm *auditdMonitor) getUsernameFromUID(uid int) string {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return fmt.Sprintf("uid:%d", uid)
	}

	uidStr := strconv.Itoa(uid)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Split(line, ":")
		if len(fields) >= 3 && fields[2] == uidStr {
			return fields[0]
		}
	}

	return fmt.Sprintf("uid:%d", uid)
}

func (cm *auditdMonitor) cleanupPendingCommands(pending map[string]*PendingCommand) {
	cutoff := time.Now().Add(-30 * time.Second)
	for id, cmd := range pending {
		if cmd.ReceivedAt.Before(cutoff) {
			delete(pending, id)
		}
	}
}

func (cm *auditdMonitor) RegisterSession(sessionID string, pid int) {
	cm.sessionMap[pid] = sessionID
	cm.logger.Info("Registered session PID (auditd)",
		zap.String("session_id", sessionID),
		zap.Int("pid", pid))
}

func (cm *auditdMonitor) UnregisterSession(sessionID string) {
	for pid, sid := range cm.sessionMap {
		if sid == sessionID {
			delete(cm.sessionMap, pid)
		}
	}
}

func (cm *auditdMonitor) pidExists(pid int) bool {
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		return false
	}
	return true
}

// Cleanup removes stale PID mappings
func (cm *auditdMonitor) Cleanup() {
	for pid := range cm.sessionMap {
		if !cm.pidExists(pid) {
			delete(cm.sessionMap, pid)
		}
	}
}
