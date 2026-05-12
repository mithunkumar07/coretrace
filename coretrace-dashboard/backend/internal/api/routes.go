package api

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/coretrace/dashboard/internal/models"
	"github.com/coretrace/dashboard/internal/websocket"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

func SetupRoutes(router *gin.Engine, db *gorm.DB, hub *websocket.Hub, jwtSecret string) {
	api := router.Group("/api/v1")

	// Public endpoints
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
		})
	})
	api.POST("/events/ingest", ingestEvents(db))

	// All other API routes require a valid auth token
	protected := api.Group("", AuthMiddleware(jwtSecret))

	agents := protected.Group("/agents")
	{
		agents.GET("", getAgents(db))
		agents.GET("/:id", getAgent(db))
		agents.POST("", createAgent(db))
		agents.POST("/:id/config", updateAgentConfig(hub))
		agents.DELETE("/:id", deleteAgent(db))
	}

	events := protected.Group("/events")
	{
		events.GET("", getEvents(db))
		events.GET("/stats", getEventStats(db))
	}

	sessions := protected.Group("/sessions")
	{
		sessions.GET("", getSessions(db))
		sessions.GET("/active", getActiveSessions(db))
		sessions.GET("/:id", getSession(db))
	}

	protected.GET("/stats", getDashboardStats(db))
}

func getAgents(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var agents []models.Agent
		if err := db.Find(&agents).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, agents)
	}
}

func getAgent(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var agent models.Agent
		if err := db.First(&agent, "id = ?", c.Param("id")).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
			return
		}
		c.JSON(http.StatusOK, agent)
	}
}

func createAgent(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var agent models.Agent
		if err := c.ShouldBindJSON(&agent); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		agent.APIKey = generateAPIKey()
		agent.Status = "offline"
		if err := db.Create(&agent).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Return api_key once at creation — it is omitted from all other responses.
		c.JSON(http.StatusCreated, gin.H{
			"id":         agent.ID,
			"name":       agent.Name,
			"hostname":   agent.Hostname,
			"ip_address": agent.IPAddress,
			"version":    agent.Version,
			"status":     agent.Status,
			"api_key":    agent.APIKey,
			"created_at": agent.CreatedAt,
		})
	}
}

func updateAgentConfig(hub *websocket.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		var config map[string]interface{}
		if err := c.ShouldBindJSON(&config); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		agentID := c.Param("id")
		payload, err := json.Marshal(map[string]interface{}{
			"type":   "config",
			"config": config,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode config"})
			return
		}
		if !hub.SendToAgent(agentID, payload) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Agent not connected"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Config update sent", "agent_id": agentID})
	}
}

func deleteAgent(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := db.Delete(&models.Agent{}, "id = ?", c.Param("id")).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Agent deleted"})
	}
}

func getEvents(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var events []models.Event
		query := db.Order("timestamp desc")

		if agentID := c.Query("agent_id"); agentID != "" {
			query = query.Where("agent_id = ?", agentID)
		}
		if eventType := c.Query("type"); eventType != "" {
			query = query.Where("event_type = ?", eventType)
		}
		if severity := c.Query("severity"); severity != "" {
			query = query.Where("severity = ?", severity)
		}

		limit := 100
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
				limit = parsed
			}
		}

		if err := query.Limit(limit).Find(&events).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"events": events, "count": len(events)})
	}
}

func getEventStats(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var stats []struct {
			EventType string `json:"event_type"`
			Count     int    `json:"count"`
		}
		db.Model(&models.Event{}).
			Select("event_type, count(*) as count").
			Group("event_type").
			Scan(&stats)
		c.JSON(http.StatusOK, stats)
	}
}

func getSessions(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var sessions []models.Session
		query := db.Order("login_time desc")
		if agentID := c.Query("agent_id"); agentID != "" {
			query = query.Where("agent_id = ?", agentID)
		}
		if err := query.Limit(100).Find(&sessions).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, sessions)
	}
}

func getActiveSessions(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var sessions []models.Session
		if err := db.Where("status = ?", "active").Find(&sessions).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, sessions)
	}
}

func getSession(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var session models.Session
		if err := db.First(&session, "id = ?", c.Param("id")).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
			return
		}
		c.JSON(http.StatusOK, session)
	}
}

func getDashboardStats(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var (
			totalAgents    int64
			onlineAgents   int64
			totalEvents    int64
			activeSessions int64
		)

		g, ctx := errgroup.WithContext(c.Request.Context())
		g.Go(func() error {
			return db.WithContext(ctx).Model(&models.Agent{}).Count(&totalAgents).Error
		})
		g.Go(func() error {
			return db.WithContext(ctx).Model(&models.Agent{}).Where("status = ?", "online").Count(&onlineAgents).Error
		})
		g.Go(func() error {
			return db.WithContext(ctx).Model(&models.Event{}).
				Where("timestamp > ?", time.Now().Add(-24*time.Hour)).
				Count(&totalEvents).Error
		})
		g.Go(func() error {
			return db.WithContext(ctx).Model(&models.Session{}).
				Where("status = ?", "active").
				Count(&activeSessions).Error
		})

		if err := g.Wait(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"total_agents":     totalAgents,
			"online_agents":    onlineAgents,
			"total_events_24h": totalEvents,
			"active_sessions":  activeSessions,
			"last_updated":     time.Now(),
		})
	}
}

// ingestEvents accepts a JSON array of events from agents authenticated via X-API-Key.
func ingestEvents(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "X-API-Key required"})
			return
		}
		var agent models.Agent
		if err := db.Where("api_key = ?", apiKey).First(&agent).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			return
		}

		var raw []map[string]interface{}
		if err := c.ShouldBindJSON(&raw); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "body must be a JSON array of events"})
			return
		}

		events := make([]models.Event, 0, len(raw))
		for _, r := range raw {
			eventType, _ := r["event_type"].(string)
			sessionID, _ := r["session_id"].(string)
			severity, _ := r["severity"].(string)
			if severity == "" {
				severity = "info"
			}
			data, _ := r["data"].(map[string]interface{})

			var ts time.Time
			if tsStr, ok := r["timestamp"].(string); ok {
				ts, _ = time.Parse(time.RFC3339Nano, tsStr)
			}
			if ts.IsZero() {
				ts = time.Now()
			}

			events = append(events, models.Event{
				AgentID:   agent.ID,
				EventType: eventType,
				Timestamp: ts,
				Severity:  severity,
				Data:      models.JSONB(data),
				SessionID: sessionID,
			})
		}

		if len(events) > 0 {
			if err := db.Create(&events).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store events"})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"stored": len(events)})
	}
}

func generateAPIKey() string {
	return "ct_" + generateRandomString(32)
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	return string(result)
}
