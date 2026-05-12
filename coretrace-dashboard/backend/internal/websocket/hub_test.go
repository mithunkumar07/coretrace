package websocket

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewHub_Initialization(t *testing.T) {
	h := NewHub(nil, "test-secret")
	if h == nil {
		t.Fatal("NewHub returned nil")
	}
	if h.JWTSecret != "test-secret" {
		t.Errorf("expected JWTSecret %q, got %q", "test-secret", h.JWTSecret)
	}
	if h.Clients == nil {
		t.Error("Clients map not initialized")
	}
	if h.AgentChannels == nil {
		t.Error("AgentChannels map not initialized")
	}
	if h.persistCh == nil {
		t.Error("persistCh channel not initialized")
	}
	if h.DB != nil {
		t.Error("expected nil DB for nil argument")
	}
}

func TestNewHub_PersistQueueCapacity(t *testing.T) {
	h := NewHub(nil, "secret")
	if cap(h.persistCh) != persistQueueSize {
		t.Errorf("expected persist queue capacity %d, got %d", persistQueueSize, cap(h.persistCh))
	}
}

func TestSendToAgent_NotConnected(t *testing.T) {
	h := NewHub(nil, "secret")
	sent := h.SendToAgent("nonexistent-id", []byte("hello"))
	if sent {
		t.Error("expected false for unregistered agent, got true")
	}
}

func TestBuildEvent_DefaultSeverity(t *testing.T) {
	agentID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	msg := map[string]interface{}{
		"event_type": "ssh_login",
	}
	event := buildEvent(agentID, msg)
	if event.Severity != "info" {
		t.Errorf("expected default severity %q, got %q", "info", event.Severity)
	}
	if event.EventType != "ssh_login" {
		t.Errorf("expected event_type %q, got %q", "ssh_login", event.EventType)
	}
}

func TestBuildEvent_TimestampFallback(t *testing.T) {
	agentID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	event := buildEvent(agentID, map[string]interface{}{})
	if event.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp fallback, got zero")
	}
}

func TestBuildEvent_ParsedTimestamp(t *testing.T) {
	agentID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	msg := map[string]interface{}{
		"timestamp": "2024-01-15T12:00:00Z",
	}
	event := buildEvent(agentID, msg)
	if event.Timestamp.Year() != 2024 {
		t.Errorf("expected year 2024, got %d", event.Timestamp.Year())
	}
}
