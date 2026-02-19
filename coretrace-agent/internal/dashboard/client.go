package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/coretrace/agent/internal/types"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Client manages connection to CoreTrace Dashboard
type Client struct {
	logger       *zap.Logger
	apiKey       string
	dashboardURL string
	conn         *websocket.Conn
	eventChan    chan types.DashboardEventMessage
	stopChan     chan struct{}
	mu           sync.RWMutex
	connected    bool
	agentID      string
}

// NewClient creates a new dashboard client
func NewClient(logger *zap.Logger, dashboardURL, apiKey, agentID string) *Client {
	return &Client{
		logger:       logger,
		apiKey:       apiKey,
		dashboardURL: dashboardURL,
		agentID:      agentID,
		eventChan:    make(chan types.DashboardEventMessage, 1000),
		stopChan:     make(chan struct{}),
	}
}

// Start begins the connection to dashboard and starts event streaming
func (c *Client) Start(ctx context.Context) error {
	c.logger.Info("Starting dashboard client",
		zap.String("url", c.dashboardURL),
		zap.String("agent_id", c.agentID))

	// Start connection manager
	go c.connectionManager(ctx)

	// Start event sender
	go c.eventSender(ctx)

	// Start heartbeat
	go c.heartbeat(ctx)

	return nil
}

// Stop gracefully shuts down the dashboard client
func (c *Client) Stop() error {
	close(c.stopChan)

	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()

	return nil
}

// SendEvent queues an event to be sent to the dashboard
func (c *Client) SendEvent(eventType string, sessionID string, data map[string]interface{}, severity string) {
	select {
	case c.eventChan <- types.DashboardEventMessage{
		Type:      "event",
		Timestamp: time.Now().UTC(),
		EventType: eventType,
		SessionID: sessionID,
		Data:      data,
		Severity:  severity,
	}:
	default:
		c.logger.Warn("Dashboard event channel full, dropping event",
			zap.String("event_type", eventType))
	}
}

// connectionManager manages WebSocket connection with automatic reconnection
func (c *Client) connectionManager(ctx context.Context) {
	backoff := time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		default:
			if err := c.connect(); err != nil {
				c.logger.Error("Failed to connect to dashboard",
					zap.Error(err),
					zap.Duration("retry_after", backoff))

				time.Sleep(backoff)

				// Exponential backoff
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}

			// Reset backoff on successful connection
			backoff = time.Second

			// Wait for disconnect
			c.waitForDisconnect()

			c.logger.Warn("Disconnected from dashboard, reconnecting...")
		}
	}
}

// connect establishes WebSocket connection
func (c *Client) connect() error {
	u, err := url.Parse(c.dashboardURL)
	if err != nil {
		return fmt.Errorf("invalid dashboard URL: %w", err)
	}

	// Switch to WebSocket protocol
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/ws/agents"
	q := u.Query()
	q.Set("api_key", c.apiKey)
	u.RawQuery = q.Encode()

	c.logger.Info("Connecting to dashboard", zap.String("url", u.String()))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.Dial(u.String(), http.Header{
		"X-Agent-ID": []string{c.agentID},
	})
	if err != nil {
		if resp != nil {
			return fmt.Errorf("websocket dial failed (status %d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	c.logger.Info("Connected to dashboard")

	// Send registration message
	if err := c.sendRegistration(); err != nil {
		conn.Close()
		return fmt.Errorf("failed to send registration: %w", err)
	}

	return nil
}

// sendRegistration sends agent registration info
func (c *Client) sendRegistration() error {
	reg := map[string]interface{}{
		"type":       "register",
		"agent_id":   c.agentID,
		"hostname":   getHostname(),
		"ip_address": getIPAddress(),
		"version":    "1.0.0",
		"metadata": map[string]interface{}{
			"os":   "linux",
			"arch": "amd64",
		},
	}

	return c.sendJSON(reg)
}

// eventSender processes events from channel and sends to dashboard
func (c *Client) eventSender(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	batch := make([]types.DashboardEventMessage, 0, 10)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		case event := <-c.eventChan:
			batch = append(batch, event)

			// Send immediately if batch is full
			if len(batch) >= 10 {
				c.sendBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			// Send any pending events
			if len(batch) > 0 {
				c.sendBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

// sendBatch sends a batch of events
func (c *Client) sendBatch(events []types.DashboardEventMessage) {
	if len(events) == 1 {
		// Send single event
		c.sendJSON(events[0])
	} else {
		// Send batch
		batch := map[string]interface{}{
			"type":      "batch",
			"timestamp": time.Now().UTC(),
			"events":    events,
		}
		c.sendJSON(batch)
	}
}

// heartbeat sends periodic heartbeats
func (c *Client) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			heartbeat := map[string]interface{}{
				"type":      "heartbeat",
				"timestamp": time.Now().UTC(),
				"stats": map[string]interface{}{
					"active_sessions": 0, // TODO: Get from session manager
				},
			}
			c.sendJSON(heartbeat)
		}
	}
}

// sendJSON sends a JSON message to the dashboard
func (c *Client) sendJSON(v interface{}) error {
	c.mu.RLock()
	conn := c.conn
	connected := c.connected
	c.mu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("not connected to dashboard")
	}

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteJSON(v)
}

// waitForDisconnect blocks until connection is lost
func (c *Client) waitForDisconnect() {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return
	}

	// Read messages until error (disconnection)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			c.connected = false
			c.mu.Unlock()
			return
		}
	}
}

// Helper functions
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func getIPAddress() string {
	// TODO: Implement proper IP detection
	return "127.0.0.1"
}
