package database

import (
	"github.com/coretrace/dashboard/internal/models"
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

func Close() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
