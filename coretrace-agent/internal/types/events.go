package types

import (
	"time"
)

type EventType string

const (
	EventSSHLogin   EventType = "ssh_login"
	EventSSHLogout  EventType = "ssh_logout"
	EventSSHFailed  EventType = "ssh_failed"
	EventCommand    EventType = "command"
	EventFileAccess EventType = "file_access"
	EventFileChange EventType = "file_change"
)

type SSHEvent struct {
	Timestamp      time.Time `json:"timestamp"`
	EventType      EventType `json:"event_type"`
	SessionID      string    `json:"session_id"`
	Username       string    `json:"username"`
	SourceIP       string    `json:"source_ip"`
	Location       Location  `json:"location"`
	AuthMethod     string    `json:"auth_method"`
	KeyFingerprint string    `json:"key_fingerprint,omitempty"`
	UserAgent      string    `json:"user_agent,omitempty"`
	PID            int       `json:"pid"`
	Success        bool      `json:"success"`
}

type CommandEvent struct {
	Timestamp  time.Time `json:"timestamp"`
	EventType  EventType `json:"event_type"`
	SessionID  string    `json:"session_id"`
	Username   string    `json:"username"`
	PID        int       `json:"pid"`
	PPID       int       `json:"ppid"`
	Command    string    `json:"command"`
	Args       []string  `json:"args"`
	WorkingDir string    `json:"working_dir"`
	ExitCode   int       `json:"exit_code,omitempty"`
}

type FileEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	EventType   EventType `json:"event_type"`
	SessionID   string    `json:"session_id,omitempty"`
	Username    string    `json:"username,omitempty"`
	PID         int       `json:"pid,omitempty"`
	FilePath    string    `json:"file_path"`
	Operation   string    `json:"operation"` // read, write, create, delete, chmod
	Size        int64     `json:"size,omitempty"`
	Permissions string    `json:"permissions,omitempty"`
}

type Location struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	ISP         string  `json:"isp"`
}

type Session struct {
	ID             string         `json:"id"`
	Username       string         `json:"username"`
	SourceIP       string         `json:"source_ip"`
	Location       Location       `json:"location"`
	LoginTime      time.Time      `json:"login_time"`
	LogoutTime     *time.Time     `json:"logout_time,omitempty"`
	AuthMethod     string         `json:"auth_method"`
	KeyFingerprint string         `json:"key_fingerprint,omitempty"`
	PID            int            `json:"pid"`
	Commands       []CommandEvent `json:"commands,omitempty"`
	FileEvents     []FileEvent    `json:"file_events,omitempty"`
	IsActive       bool           `json:"is_active"`
}
