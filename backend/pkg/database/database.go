package database

import (
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"irrigation/internal/config"
	applogger "irrigation/pkg/logger"
)

var DB *gorm.DB

func Init() error {
	dsn := config.AppSettings.Postgres.DSN()

	var logLevel gormlogger.LogLevel
	if config.AppSettings.App.Env == "development" {
		logLevel = gormlogger.Info
	} else {
		logLevel = gormlogger.Error
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(logLevel),
	})

	if err != nil {
		applogger.Fatal("Failed to connect to database", zap.Error(err))
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	applogger.Info("Database connected successfully")
	return nil
}
