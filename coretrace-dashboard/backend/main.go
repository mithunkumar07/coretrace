package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/coretrace/dashboard/internal/api"
	"github.com/coretrace/dashboard/internal/config"
	"github.com/coretrace/dashboard/internal/database"
	"github.com/coretrace/dashboard/internal/websocket"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"golang.org/x/time/rate"
)

func main() {
	godotenv.Load()

	cfg := config.Load()

	db, err := database.Initialize(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	database.SeedAdmin(db)

	hub := websocket.NewHub(db, cfg.JWTSecret)
	go hub.Run()

	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(CORSMiddleware())
	router.Use(RateLimitMiddleware())

	api.SetupRoutes(router, db, hub, cfg.JWTSecret)
	api.SetupAuthRoutes(router, db, cfg.JWTSecret)

	router.GET("/ws/agents", func(c *gin.Context) {
		websocket.ServeAgentWs(hub, c.Writer, c.Request)
	})
	router.GET("/ws/dashboard", func(c *gin.Context) {
		websocket.ServeDashboardWs(hub, c.Writer, c.Request)
	})

	go func() {
		addr := ":" + cfg.Port
		log.Printf("Starting CoreTrace Dashboard API on %s", addr)
		if err := router.Run(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
}

// CORSMiddleware allows cross-origin requests from the React dev server.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-API-Key")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// RateLimitMiddleware enforces 60 requests/second per IP with a burst of 20.
func RateLimitMiddleware() gin.HandlerFunc {
	var limiters sync.Map
	return func(c *gin.Context) {
		ip := c.ClientIP()
		v, _ := limiters.LoadOrStore(ip, rate.NewLimiter(rate.Every(time.Second/60), 20))
		if !v.(*rate.Limiter).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests"})
			return
		}
		c.Next()
	}
}
