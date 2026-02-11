package monitor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/coretrace/agent/internal/types"
	"go.uber.org/zap"
)

type SSHMonitor struct {
	logger    *zap.Logger
	eventChan chan<- types.SSHEvent
}

func NewSSHMonitor(logger *zap.Logger, eventChan chan<- types.SSHEvent) *SSHMonitor {
	return &SSHMonitor{
		logger:    logger,
		eventChan: eventChan,
	}
}

var (
	sshAcceptedRegex = regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+sshd\[\d+\]:\s+Accepted\s+(\w+)\s+for\s+(\w+)\s+from\s+(\S+)\s+port\s+(\d+)\s+ssh2(\s+\[([^\]]+)\])?`)
	sshFailedRegex   = regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+sshd\[\d+\]:\s+Failed\s+(\w+)\s+for\s+(\w+)\s+from\s+(\S+)\s+port\s+(\d+)\s+ssh2`)
	sshLogoutRegex   = regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+sshd\[\d+\]:\s+pam_unix\(sshd:session\):\s+session\s+closed\s+for\s+user\s+(\w+)`)
)

func (sm *SSHMonitor) Start(ctx context.Context) error {
	sm.logger.Info("Starting SSH monitor")

	go sm.monitorAuthLog(ctx)

	return nil
}

func (sm *SSHMonitor) monitorAuthLog(ctx context.Context) {
	authLogFile := "/var/log/auth.log"
	if _, err := os.Stat("/var/log/secure"); err == nil {
		authLogFile = "/var/log/secure"
	}

	file, err := os.Open(authLogFile)
	if err != nil {
		sm.logger.Error("Failed to open auth log", zap.String("file", authLogFile), zap.Error(err))
		return
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	// Seek to end of file for real-time monitoring
	file.Seek(0, 2)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			sm.parseAuthLine(strings.TrimSpace(line))
		}
	}
}

func (sm *SSHMonitor) parseAuthLine(line string) {
	if matches := sshAcceptedRegex.FindStringSubmatch(line); matches != nil {
		sm.handleSSHLogin(matches, true)
	} else if matches := sshFailedRegex.FindStringSubmatch(line); matches != nil {
		sm.handleSSHLogin(matches, false)
	} else if matches := sshLogoutRegex.FindStringSubmatch(line); matches != nil {
		sm.handleSSHLogout(matches)
	}
}

func (sm *SSHMonitor) handleSSHLogin(matches []string, success bool) {
	timestamp, err := time.Parse("Jan 2 15:04:05", fmt.Sprintf("%s %s", matches[1], time.Now().Format("2006")))
	if err != nil {
		sm.logger.Warn("Failed to parse timestamp", zap.Error(err))
		timestamp = time.Now()
	}

	authMethod := matches[2]
	username := matches[3]
	sourceIP := matches[4]

	sessionID := sm.generateSessionID(sourceIP, username, timestamp)

	event := types.SSHEvent{
		Timestamp:  timestamp,
		EventType:  types.EventSSHLogin,
		SessionID:  sessionID,
		Username:   username,
		SourceIP:   sourceIP,
		Location:   sm.getLocationForIP(sourceIP),
		AuthMethod: authMethod,
		Success:    success,
	}

	if len(matches) > 6 && matches[7] != "" {
		event.KeyFingerprint = sm.extractKeyFingerprint(matches[7])
	}

	if !success {
		event.EventType = types.EventSSHFailed
	}

	select {
	case sm.eventChan <- event:
		sm.logger.Debug("SSH event sent", zap.String("session_id", sessionID), zap.Bool("success", success))
	default:
		sm.logger.Warn("Event channel full, dropping SSH event")
	}
}

func (sm *SSHMonitor) handleSSHLogout(matches []string) {
	timestamp, err := time.Parse("Jan 2 15:04:05", fmt.Sprintf("%s %s", matches[1], time.Now().Format("2006")))
	if err != nil {
		sm.logger.Warn("Failed to parse timestamp", zap.Error(err))
		timestamp = time.Now()
	}

	username := matches[2]
	sessionID := sm.generateSessionID("", username, timestamp)

	event := types.SSHEvent{
		Timestamp: timestamp,
		EventType: types.EventSSHLogout,
		SessionID: sessionID,
		Username:  username,
		Success:   true,
	}

	select {
	case sm.eventChan <- event:
		sm.logger.Debug("SSH logout event sent", zap.String("session_id", sessionID))
	default:
		sm.logger.Warn("Event channel full, dropping SSH logout event")
	}
}

func (sm *SSHMonitor) generateSessionID(sourceIP, username string, timestamp time.Time) string {
	return fmt.Sprintf("%s_%s_%d", sourceIP, username, timestamp.Unix())
}

func (sm *SSHMonitor) getLocationForIP(ip string) types.Location {
	// TODO: Implement geolocation lookup
	return types.Location{
		Country: "Unknown",
		City:    "Unknown",
	}
}

func (sm *SSHMonitor) extractKeyFingerprint(fingerprintData string) string {
	// TODO: Parse SSH key fingerprint from auth log
	return fingerprintData
}
