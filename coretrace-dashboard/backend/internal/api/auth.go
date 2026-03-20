package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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

		token := signToken(user.ID.String(), jwtSecret)
		c.JSON(http.StatusOK, loginResponse{Token: token, User: user})
	}
}

// signToken creates a simple HMAC-signed token: base64(payload).base64(sig)
func signToken(userID string, secret string) string {
	payload, _ := json.Marshal(map[string]interface{}{
		"sub": userID,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%s", encoded, sig)
}
