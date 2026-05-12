package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	internalauth "github.com/coretrace/dashboard/internal/auth"
	"github.com/coretrace/dashboard/internal/models"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

const (
	writeWait        = 10 * time.Second
	pongWait         = 60 * time.Second
	pingPeriod       = (pongWait * 9) / 10
	maxMessageSize   = 512 * 1024 // 512 KB
	heartbeatSweep   = 30 * time.Second
	staleThreshold   = 2 * time.Minute
	persistWorkers   = 4
	persistQueueSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type persistJob struct {
	agentID uuid.UUID
	message []byte
}

// Client represents a WebSocket connection (agent or browser).
type Client struct {
	Hub      *Hub
	Conn     *websocket.Conn
	Send     chan []byte
	IsAgent  bool
	AgentID  uuid.UUID   // only meaningful for agent clients
	lastSeen atomic.Int64 // unix seconds; updated on every received message
}

// Hub maintains the set of active clients and routes messages.
type Hub struct {
	Clients       map[*Client]bool
	Broadcast     chan []byte
	Register      chan *Client
	Unregister    chan *Client
	AgentChannels map[string]chan []byte // keyed by AgentID.String()
	DB            *gorm.DB
	JWTSecret     string
	persistCh     chan persistJob
}

// NewHub creates and wires up a Hub. Starts the DB persist worker pool immediately.
func NewHub(db *gorm.DB, jwtSecret string) *Hub {
	h := &Hub{
		Broadcast:     make(chan []byte),
		Register:      make(chan *Client),
		Unregister:    make(chan *Client),
		Clients:       make(map[*Client]bool),
		AgentChannels: make(map[string]chan []byte),
		DB:            db,
		JWTSecret:     jwtSecret,
		persistCh:     make(chan persistJob, persistQueueSize),
	}
	for i := 0; i < persistWorkers; i++ {
		go h.persistWorker()
	}
	return h
}

// Run is the hub's main event loop. Must be called in its own goroutine.
func (h *Hub) Run() {
	ticker := time.NewTicker(heartbeatSweep)
	defer ticker.Stop()
	for {
		select {
		case client := <-h.Register:
			h.Clients[client] = true
			if client.IsAgent {
				h.AgentChannels[client.AgentID.String()] = client.Send
				log.Printf("Agent %s connected", client.AgentID)
			}

		case client := <-h.Unregister:
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
				if client.IsAgent {
					delete(h.AgentChannels, client.AgentID.String())
					log.Printf("Agent %s disconnected", client.AgentID)
					if h.DB != nil {
						go h.updateAgentStatus(client.AgentID, "offline")
						go h.closeAgentSessions(client.AgentID)
					}
				}
			}

		case message := <-h.Broadcast:
			for client := range h.Clients {
				if !client.IsAgent {
					select {
					case client.Send <- message:
					default:
						close(client.Send)
						delete(h.Clients, client)
					}
				}
			}

		case <-ticker.C:
			h.sweepStaleAgents()
		}
	}
}

// sweepStaleAgents closes agent connections that have been silent for staleThreshold.
// Called from Run() so it has exclusive access to h.Clients.
func (h *Hub) sweepStaleAgents() {
	cutoff := time.Now().Add(-staleThreshold).Unix()
	for client := range h.Clients {
		if client.IsAgent && client.lastSeen.Load() < cutoff {
			log.Printf("Agent %s timed out (no message for %v)", client.AgentID, staleThreshold)
			client.Conn.Close()
		}
	}
}

// SendToAgent delivers a message to a connected agent. Returns false if the agent
// is not connected or its send buffer is full.
func (h *Hub) SendToAgent(agentIDStr string, message []byte) bool {
	ch, ok := h.AgentChannels[agentIDStr]
	if !ok {
		return false
	}
	select {
	case ch <- message:
		return true
	default:
		return false
	}
}

// readPump pumps messages from the WebSocket connection into the hub.
func (c *Client) readPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		if c.IsAgent {
			c.lastSeen.Store(time.Now().Unix())
			select {
			case c.Hub.persistCh <- persistJob{c.AgentID, message}:
			default:
				log.Printf("persist queue full, dropping message from agent %s", c.AgentID)
			}
			c.Hub.Broadcast <- message
		} else {
			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err == nil {
				if targetAgent, ok := msg["target_agent"].(string); ok {
					c.Hub.SendToAgent(targetAgent, message)
				}
			}
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.Conn.WriteMessage(websocket.TextMessage, message)

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeAgentWs handles WebSocket upgrades for agents authenticating via API key.
func ServeAgentWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	if hub.DB == nil {
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	apiKey := r.URL.Query().Get("api_key")
	if apiKey == "" {
		http.Error(w, "Missing API key", http.StatusUnauthorized)
		return
	}

	var agent models.Agent
	if err := hub.DB.Where("api_key = ?", apiKey).First(&agent).Error; err != nil {
		http.Error(w, "Invalid API key", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		Hub:     hub,
		Conn:    conn,
		Send:    make(chan []byte, 256),
		IsAgent: true,
		AgentID: agent.ID,
	}
	client.lastSeen.Store(time.Now().Unix())

	hub.DB.Model(&agent).Updates(map[string]interface{}{
		"status":    "online",
		"last_seen": time.Now(),
	})

	client.Hub.Register <- client
	go client.writePump()
	go client.readPump()
}

// ServeDashboardWs handles WebSocket upgrades for browser clients authenticated via token.
func ServeDashboardWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Authorization required", http.StatusUnauthorized)
		return
	}
	if _, err := internalauth.VerifyToken(token, hub.JWTSecret); err != nil {
		http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		Hub:     hub,
		Conn:    conn,
		Send:    make(chan []byte, 256),
		IsAgent: false,
	}

	client.Hub.Register <- client
	go client.writePump()
	go client.readPump()
}

// persistWorker drains persistCh and writes messages to the database.
func (h *Hub) persistWorker() {
	for job := range h.persistCh {
		h.persistAgentMessage(job.agentID, job.message)
	}
}

// persistAgentMessage parses and dispatches a single agent WebSocket message to DB.
func (h *Hub) persistAgentMessage(agentID uuid.UUID, message []byte) {
	if h.DB == nil {
		return
	}
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}
	msgType, _ := msg["type"].(string)
	switch msgType {
	case "event":
		h.storeEvent(agentID, msg)
	case "batch":
		events, ok := msg["events"].([]interface{})
		if !ok {
			return
		}
		h.storeBatch(agentID, events)
	case "register":
		h.handleRegistration(agentID, msg)
	case "heartbeat":
		h.updateAgentStatus(agentID, "online")
	}
}

func (h *Hub) storeEvent(agentID uuid.UUID, msg map[string]interface{}) {
	event := buildEvent(agentID, msg)
	if err := h.DB.Create(&event).Error; err != nil {
		log.Printf("Failed to persist event from agent %s: %v", agentID, err)
	}
}

// storeBatch inserts all events from a batch message in a single DB round-trip.
func (h *Hub) storeBatch(agentID uuid.UUID, raw []interface{}) {
	events := make([]models.Event, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]interface{}); ok {
			events = append(events, buildEvent(agentID, m))
		}
	}
	if len(events) == 0 {
		return
	}
	if err := h.DB.Create(&events).Error; err != nil {
		log.Printf("Failed to persist batch from agent %s: %v", agentID, err)
	}
}

func buildEvent(agentID uuid.UUID, msg map[string]interface{}) models.Event {
	eventType, _ := msg["event_type"].(string)
	sessionID, _ := msg["session_id"].(string)
	severity, _ := msg["severity"].(string)
	if severity == "" {
		severity = "info"
	}
	data, _ := msg["data"].(map[string]interface{})

	var ts time.Time
	if tsStr, ok := msg["timestamp"].(string); ok {
		ts, _ = time.Parse(time.RFC3339Nano, tsStr)
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	return models.Event{
		AgentID:   agentID,
		EventType: eventType,
		Timestamp: ts,
		Severity:  severity,
		Data:      models.JSONB(data),
		SessionID: sessionID,
	}
}

func (h *Hub) handleRegistration(agentID uuid.UUID, msg map[string]interface{}) {
	updates := map[string]interface{}{
		"status":    "online",
		"last_seen": time.Now(),
	}
	if hostname, ok := msg["hostname"].(string); ok && hostname != "" {
		updates["hostname"] = hostname
	}
	if ip, ok := msg["ip_address"].(string); ok && ip != "" {
		updates["ip_address"] = ip
	}
	if version, ok := msg["version"].(string); ok && version != "" {
		updates["version"] = version
	}
	if err := h.DB.Model(&models.Agent{}).Where("id = ?", agentID).Updates(updates).Error; err != nil {
		log.Printf("Failed to update agent registration %s: %v", agentID, err)
	}
}

func (h *Hub) updateAgentStatus(agentID uuid.UUID, status string) {
	h.DB.Model(&models.Agent{}).Where("id = ?", agentID).Updates(map[string]interface{}{
		"status":    status,
		"last_seen": time.Now(),
	})
}

// closeAgentSessions marks all active sessions for an agent as timed-out.
func (h *Hub) closeAgentSessions(agentID uuid.UUID) {
	now := time.Now()
	h.DB.Model(&models.Session{}).
		Where("agent_id = ? AND status = ?", agentID, "active").
		Updates(map[string]interface{}{
			"status":      "timeout",
			"logout_time": now,
		})
}
