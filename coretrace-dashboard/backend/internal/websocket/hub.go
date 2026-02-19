package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512 * 1024 // 512KB
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

// Client represents a WebSocket client
type Client struct {
	Hub     *Hub
	Conn    *websocket.Conn
	Send    chan []byte
	Type    string // "agent" or "dashboard"
	AgentID string
	IsAgent bool
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	// Registered clients
	Clients map[*Client]bool

	// Inbound messages from the clients
	Broadcast chan []byte

	// Register requests from the clients
	Register chan *Client

	// Unregister requests from clients
	Unregister chan *Client

	// Agent-specific message routing
	AgentChannels map[string]chan []byte
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		Broadcast:     make(chan []byte),
		Register:      make(chan *Client),
		Unregister:    make(chan *Client),
		Clients:       make(map[*Client]bool),
		AgentChannels: make(map[string]chan []byte),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Clients[client] = true
			if client.IsAgent && client.AgentID != "" {
				h.AgentChannels[client.AgentID] = client.Send
				log.Printf("Agent %s connected", client.AgentID)
			} else {
				log.Printf("Dashboard client connected")
			}

		case client := <-h.Unregister:
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
				if client.IsAgent && client.AgentID != "" {
					delete(h.AgentChannels, client.AgentID)
					log.Printf("Agent %s disconnected", client.AgentID)
				}
			}

		case message := <-h.Broadcast:
			// Broadcast to all dashboard clients
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
		}
	}
}

// SendToAgent sends a message to a specific agent
func (h *Hub) SendToAgent(agentID string, message []byte) bool {
	if ch, ok := h.AgentChannels[agentID]; ok {
		select {
		case ch <- message:
			return true
		default:
			return false
		}
	}
	return false
}

// readPump pumps messages from the websocket connection to the hub
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

		// Process message based on client type
		if c.IsAgent {
			// Agent message - broadcast to dashboard clients
			c.Hub.Broadcast <- message
		} else {
			// Dashboard message - route to specific agent if needed
			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err == nil {
				if targetAgent, ok := msg["target_agent"].(string); ok {
					c.Hub.SendToAgent(targetAgent, message)
				}
			}
		}
	}
}

// writePump pumps messages from the hub to the websocket connection
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
				// The hub closed the channel
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

// ServeAgentWs handles WebSocket requests from agents
func ServeAgentWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	// Validate API key
	apiKey := r.URL.Query().Get("api_key")
	if apiKey == "" {
		http.Error(w, "Missing API key", http.StatusUnauthorized)
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
		Type:    "agent",
		AgentID: apiKey, // TODO: Validate and extract real agent ID
		IsAgent: true,
	}

	client.Hub.Register <- client

	// Start pumps
	go client.writePump()
	go client.readPump()
}

// ServeDashboardWs handles WebSocket requests from dashboard clients
func ServeDashboardWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	// TODO: Validate JWT token

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		Hub:     hub,
		Conn:    conn,
		Send:    make(chan []byte, 256),
		Type:    "dashboard",
		IsAgent: false,
	}

	client.Hub.Register <- client

	// Start pumps
	go client.writePump()
	go client.readPump()
}
