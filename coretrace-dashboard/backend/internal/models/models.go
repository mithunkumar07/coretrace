package models

import (
	"time"

	"github.com/google/uuid"
)

// Agent represents a CoreTrace agent
type Agent struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	Name      string    `json:"name"`
	Hostname  string    `json:"hostname"`
	IPAddress string    `json:"ip_address"`
	Version   string    `json:"version"`
	Status    string    `json:"status" gorm:"default:'offline'"`
	LastSeen  time.Time `json:"last_seen"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Metadata  JSONB     `json:"metadata" gorm:"type:jsonb"`
	APIKey    string    `json:"-" gorm:"uniqueIndex"`
}

// Event represents a security event from an agent
type Event struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	AgentID   uuid.UUID `json:"agent_id" gorm:"type:uuid;index"`
	Agent     Agent     `json:"agent,omitempty" gorm:"foreignKey:AgentID"`
	EventType string    `json:"event_type" gorm:"index"`
	Timestamp time.Time `json:"timestamp" gorm:"index"`
	Severity  string    `json:"severity" gorm:"default:'info'"`
	Data      JSONB     `json:"data" gorm:"type:jsonb"`
	SessionID string    `json:"session_id" gorm:"index"`
	CreatedAt time.Time `json:"created_at"`
}

// Session represents an SSH session
type Session struct {
	ID             uuid.UUID  `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	AgentID        uuid.UUID  `json:"agent_id" gorm:"type:uuid;index"`
	Agent          Agent      `json:"agent,omitempty" gorm:"foreignKey:AgentID"`
	SessionID      string     `json:"session_id" gorm:"uniqueIndex"`
	Username       string     `json:"username" gorm:"index"`
	SourceIP       string     `json:"source_ip"`
	AuthMethod     string     `json:"auth_method"`
	LoginTime      time.Time  `json:"login_time"`
	LogoutTime     *time.Time `json:"logout_time,omitempty"`
	CommandCount   int        `json:"command_count" gorm:"default:0"`
	Status         string     `json:"status" gorm:"default:'active'"`
	KeyFingerprint string     `json:"key_fingerprint"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// User represents a dashboard user
type User struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	Email     string    `json:"email" gorm:"uniqueIndex"`
	Password  string    `json:"-"`
	Name      string    `json:"name"`
	Role      string    `json:"role" gorm:"default:'viewer'"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// JSONB is a custom type for PostgreSQL JSONB
type JSONB map[string]interface{}
