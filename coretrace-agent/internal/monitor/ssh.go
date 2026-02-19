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
	// Accepted publickey for username from IP port N ssh2: RSA SHA256:...
	sshAcceptedRegex = regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+sshd\[(\d+)\]:\s+Accepted\s+(\w+)\s+for\s+(\S+)\s+from\s+(\S+)\s+port\s+(\d+)\s+ssh2(?::\s+(.*))?`)
	// Failed password/publickey for username from IP port N ssh2
	sshFailedRegex = regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+sshd\[\d+\]:\s+Failed\s+(\w+)\s+for\s+(\S+)\s+from\s+(\S+)\s+port\s+(\d+)\s+ssh2`)
	// Connection closed by authenticating user (happens after multiple failed attempts)
	sshConnClosedRegex = regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+sshd\[\d+\]:\s+Connection\s+closed\s+by\s+authenticating\s+user\s+(\S+)\s+(\S+)\s+port\s+(\d+)\s+\[.*\]`)
	// pam_unix(sshd:session): session closed for user username
	sshLogoutRegex = regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+sshd\[(\d+)\]:\s+pam_unix\(sshd:session\):\s+session\s+closed\s+for\s+user\s+(\S+)`)
	// Extract fingerprint from key info (e.g., "RSA SHA256:abc123..." or "SHA256:abc123...")
	fingerprintRegex = regexp.MustCompile(`(?:SHA256|MD5):[A-Za-z0-9+/=]+`)
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

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := sm.tailAuthLog(ctx, authLogFile); err != nil {
				sm.logger.Error("Error tailing auth log", zap.Error(err))
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (sm *SSHMonitor) tailAuthLog(ctx context.Context, authLogFile string) error {
	file, err := os.Open(authLogFile)
	if err != nil {
		return fmt.Errorf("failed to open auth log: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	file.Seek(0, 2)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				// Check if file was rotated
				if os.IsNotExist(err) {
					sm.logger.Info("Auth log rotated, reopening")
					return nil
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}

			sm.parseAuthLine(strings.TrimSpace(line))
		}
	}
}

func (sm *SSHMonitor) parseAuthLine(line string) {
	sm.logger.Debug("Parsing auth line", zap.String("line", line))

	if matches := sshAcceptedRegex.FindStringSubmatch(line); matches != nil {
		sm.handleSSHLogin(matches, true)
	} else if matches := sshFailedRegex.FindStringSubmatch(line); matches != nil {
		sm.logger.Info("SSH failed login detected", zap.String("line", line))
		sm.handleSSHLogin(matches, false)
	} else if matches := sshConnClosedRegex.FindStringSubmatch(line); matches != nil {
		sm.handleSSHAuthFailed(matches)
	} else if matches := sshLogoutRegex.FindStringSubmatch(line); matches != nil {
		sm.handleSSHLogout(matches)
	}
}

func (sm *SSHMonitor) handleSSHLogin(matches []string, success bool) {
	timestamp := sm.parseSyslogTimestamp(matches[1])

	// matches: [0]=full, [1]=timestamp, [2]=pid, [3]=authMethod, [4]=username, [5]=sourceIP, [6]=port, [7]=keyInfo
	pid := 0
	if len(matches) > 2 {
		fmt.Sscanf(matches[2], "%d", &pid)
	}
	authMethod := matches[3]
	username := matches[4]
	sourceIP := matches[5]

	sessionID := sm.generateSessionID(sourceIP, username, timestamp)

	event := types.SSHEvent{
		Timestamp:  timestamp,
		EventType:  types.EventSSHLogin,
		SessionID:  sessionID,
		Username:   username,
		SourceIP:   sourceIP,
		Location:   sm.getLocationForIP(sourceIP),
		AuthMethod: authMethod,
		PID:        pid,
		Success:    success,
	}

	// Extract key fingerprint from the key info field (e.g., "RSA SHA256:abc123...")
	if len(matches) > 7 && matches[7] != "" {
		event.KeyFingerprint = sm.extractKeyFingerprint(matches[7])
		sm.logger.Debug("Extracted key fingerprint",
			zap.String("fingerprint", event.KeyFingerprint),
			zap.String("raw_key_info", matches[7]))
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

func (sm *SSHMonitor) parseSyslogTimestamp(timeStr string) time.Time {
	now := time.Now()
	// Try current year first
	timestamp, err := time.Parse("Jan 2 15:04:05", fmt.Sprintf("%s %d", timeStr, now.Year()))
	if err != nil {
		sm.logger.Warn("Failed to parse timestamp", zap.Error(err))
		return now
	}

	// If timestamp is in the future by more than 24 hours, assume it's from last year
	if timestamp.After(now.Add(24 * time.Hour)) {
		timestamp, err = time.Parse("Jan 2 15:04:05", fmt.Sprintf("%s %d", timeStr, now.Year()-1))
		if err != nil {
			return now
		}
	}

	return timestamp
}

func (sm *SSHMonitor) handleSSHLogout(matches []string) {
	timestamp := sm.parseSyslogTimestamp(matches[1])

	// matches: [0]=full, [1]=timestamp, [2]=pid, [3]=username
	pid := 0
	if len(matches) > 2 {
		fmt.Sscanf(matches[2], "%d", &pid)
	}
	username := matches[3]
	sessionID := sm.generateSessionID("", username, timestamp)

	event := types.SSHEvent{
		Timestamp: timestamp,
		EventType: types.EventSSHLogout,
		SessionID: sessionID,
		Username:  username,
		PID:       pid,
		Success:   true,
	}

	select {
	case sm.eventChan <- event:
		sm.logger.Debug("SSH logout event sent", zap.String("session_id", sessionID))
	default:
		sm.logger.Warn("Event channel full, dropping SSH logout event")
	}
}

func (sm *SSHMonitor) handleSSHAuthFailed(matches []string) {
	timestamp := sm.parseSyslogTimestamp(matches[1])

	// matches: [0]=full, [1]=timestamp, [2]=username, [3]=sourceIP, [4]=port
	username := matches[2]
	sourceIP := matches[3]

	sessionID := sm.generateSessionID(sourceIP, username, timestamp)

	event := types.SSHEvent{
		Timestamp:  timestamp,
		EventType:  types.EventSSHFailed,
		SessionID:  sessionID,
		Username:   username,
		SourceIP:   sourceIP,
		Location:   sm.getLocationForIP(sourceIP),
		AuthMethod: "publickey",
		PID:        0,
		Success:    false,
	}

	select {
	case sm.eventChan <- event:
		sm.logger.Info("SSH authentication failed - connection closed",
			zap.String("session_id", sessionID),
			zap.String("username", username),
			zap.String("source_ip", sourceIP))
	default:
		sm.logger.Warn("Event channel full, dropping SSH auth failed event")
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
	// Extract fingerprint from key info like "RSA SHA256:abc123..." or "SHA256:abc123..."
	if matches := fingerprintRegex.FindStringSubmatch(fingerprintData); len(matches) > 0 {
		return matches[0]
	}
	// Fallback: return the full data if we can't extract
	return fingerprintData
}
