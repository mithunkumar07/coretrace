package api

import (
	"net/http"
	"time"

	"github.com/coretrace/dashboard/internal/models"
	"github.com/coretrace/dashboard/internal/websocket"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRoutes(router *gin.Engine, db *gorm.DB, hub *websocket.Hub) {
	api := router.Group("/api/v1")

	// Health check
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
		})
	})

	// Agents
	agents := api.Group("/agents")
	{
		agents.GET("", getAgents(db))
		agents.GET("/:id", getAgent(db))
		agents.POST("", createAgent(db))
		agents.POST("/:id/config", updateAgentConfig(db, hub))
		agents.DELETE("/:id", deleteAgent(db))
	}

	// Events
	events := api.Group("/events")
	{
		events.GET("", getEvents(db))
		events.GET("/stats", getEventStats(db))
	}

	// Sessions
	sessions := api.Group("/sessions")
	{
		sessions.GET("", getSessions(db))
		sessions.GET("/active", getActiveSessions(db))
		sessions.GET("/:id", getSession(db))
	}

	// Dashboard stats
	api.GET("/stats", getDashboardStats(db))
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

		// Generate API key
		agent.APIKey = generateAPIKey()
		agent.Status = "offline"

		if err := db.Create(&agent).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, agent)
	}
}

func updateAgentConfig(db *gorm.DB, hub *websocket.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		var config map[string]interface{}
		if err := c.ShouldBindJSON(&config); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// TODO: Send config update to agent via WebSocket
		agentID := c.Param("id")

		c.JSON(http.StatusOK, gin.H{
			"message":  "Config update sent",
			"agent_id": agentID,
			"config":   config,
		})
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

		// Filters
		if agentID := c.Query("agent_id"); agentID != "" {
			query = query.Where("agent_id = ?", agentID)
		}
		if eventType := c.Query("type"); eventType != "" {
			query = query.Where("event_type = ?", eventType)
		}
		if severity := c.Query("severity"); severity != "" {
			query = query.Where("severity = ?", severity)
		}

		// Pagination
		limit := 100
		if l := c.Query("limit"); l != "" {
			// Parse limit
		}

		if err := query.Limit(limit).Find(&events).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"events": events,
			"count":  len(events),
		})
	}
}

func getEventStats(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get event counts by type
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
		var stats struct {
			TotalAgents    int64     `json:"total_agents"`
			OnlineAgents   int64     `json:"online_agents"`
			TotalEvents    int64     `json:"total_events_24h"`
			ActiveSessions int64     `json:"active_sessions"`
			LastUpdated    time.Time `json:"last_updated"`
		}

		// Get agent counts
		db.Model(&models.Agent{}).Count(&stats.TotalAgents)
		db.Model(&models.Agent{}).Where("status = ?", "online").Count(&stats.OnlineAgents)

		// Get events in last 24h
		db.Model(&models.Event{}).
			Where("timestamp > ?", time.Now().Add(-24*time.Hour)).
			Count(&stats.TotalEvents)

		// Get active sessions
		db.Model(&models.Session{}).
			Where("status = ?", "active").
			Count(&stats.ActiveSessions)

		stats.LastUpdated = time.Now()

		c.JSON(http.StatusOK, stats)
	}
}

func generateAPIKey() string {
	// Simple implementation - use UUID in production
	return "ct_" + generateRandomString(32)
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(result)
}
