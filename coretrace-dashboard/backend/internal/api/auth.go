package api

import (
	"net/http"
	"strings"

	"github.com/coretrace/dashboard/internal/auth"
	"github.com/coretrace/dashboard/internal/models"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type loginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	Token string      `json:"token"`
	User  models.User `json:"user"`
}

func SetupAuthRoutes(router *gin.Engine, db *gorm.DB, jwtSecret string) {
	router.POST("/api/v1/auth/login", handleLogin(db, jwtSecret))
}

func handleLogin(db *gorm.DB, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email and password are required"})
			return
		}

		var user models.User
		if err := db.Where("email = ?", req.Email).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		token := auth.SignToken(user.ID.String(), jwtSecret)
		c.JSON(http.StatusOK, loginResponse{Token: token, User: user})
	}
}

// AuthMiddleware validates HMAC-signed Bearer tokens on protected routes.
func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
			return
		}
		userID, err := auth.VerifyToken(strings.TrimPrefix(header, "Bearer "), jwtSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}
		c.Set("user_id", userID)
		c.Next()
	}
}
