package database

import (
	"log"
	"os"

	"github.com/coretrace/dashboard/internal/models"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Initialize(databaseURL string) (*gorm.DB, error) {
	var err error
	DB, err = gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	return DB, nil
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.Agent{},
		&models.Event{},
		&models.Session{},
		&models.User{},
	)
}

// SeedAdmin creates a default admin user if none exists.
// Credentials are read from ADMIN_EMAIL / ADMIN_PASSWORD env vars,
// falling back to admin@coretrace.io / admin123.
func SeedAdmin(db *gorm.DB) {
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count > 0 {
		return
	}

	email := os.Getenv("ADMIN_EMAIL")
	if email == "" {
		email = "admin@coretrace.io"
	}
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		password = "admin123"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash admin password: %v", err)
		return
	}

	admin := models.User{
		ID:       uuid.New(),
		Email:    email,
		Password: string(hash),
		Name:     "Admin",
		Role:     "admin",
	}
	if err := db.Create(&admin).Error; err != nil {
		log.Printf("Failed to seed admin user: %v", err)
		return
	}
	log.Printf("Admin user created: %s", email)
}

func Close() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
