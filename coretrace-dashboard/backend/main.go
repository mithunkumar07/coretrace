package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/coretrace/dashboard/internal/api"
	"github.com/coretrace/dashboard/internal/config"
	"github.com/coretrace/dashboard/internal/database"
	"github.com/coretrace/dashboard/internal/websocket"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	godotenv.Load()

	// Load configuration
	cfg := config.Load()

	// Initialize database
	db, err := database.Initialize(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Run migrations
	if err := database.Migrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Seed default admin user
	database.SeedAdmin(db)

	// Initialize WebSocket hub
	hub := websocket.NewHub()
	go hub.Run()

	// Setup Gin router
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(CORSMiddleware())

	// Setup API routes
	api.SetupRoutes(router, db, hub)
	api.SetupAuthRoutes(router, db, cfg.JWTSecret)

	// Setup WebSocket route
	router.GET("/ws/agents", func(c *gin.Context) {
		websocket.ServeAgentWs(hub, c.Writer, c.Request)
	})

	router.GET("/ws/dashboard", func(c *gin.Context) {
		websocket.ServeDashboardWs(hub, c.Writer, c.Request)
	})

	// Start server
	go func() {
		addr := ":" + cfg.Port
		log.Printf("Starting CoreTrace Dashboard API on %s", addr)
		if err := router.Run(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
