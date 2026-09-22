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

	if err := migrate(); err != nil {
		return err
	}

	return nil
}

// migrate 执行幂等的结构迁移，保证已有数据库升级到安全闭环所需的结构。
// init.sql 仅在全新部署时执行，已有库依赖此处补齐。
func migrate() error {
	statements := []string{
		// 执行状态枚举补充 skipped（雨量跳过记录）
		`ALTER TYPE execution_status ADD VALUE IF NOT EXISTS 'skipped'`,
		// 执行记录补充幂等键
		`ALTER TABLE irrigation_logs ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(100)`,
		// 幂等键唯一：同一键重复启动返回原执行记录
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_irrigation_logs_idempotency_key
			ON irrigation_logs(idempotency_key) WHERE idempotency_key IS NOT NULL`,
		// 区域互斥：同一区域同一时刻只允许一条进行中的执行记录
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_irrigation_logs_zone_in_progress
			ON irrigation_logs(zone_id) WHERE status = 'in_progress'`,
		// 区域熔断状态
		`ALTER TABLE irrigation_zones ADD COLUMN IF NOT EXISTS consecutive_failures INTEGER DEFAULT 0`,
		`ALTER TABLE irrigation_zones ADD COLUMN IF NOT EXISTS circuit_open_until TIMESTAMP`,
		// 模型软删除字段对应列
		`ALTER TABLE irrigation_zones ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP`,
		`ALTER TABLE irrigation_schedules ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP`,
		// 安全闭环默认配置
		`INSERT INTO system_configs (key, value, description) VALUES
			('rainfall_skip_threshold_mm', '5', '启动前两小时降雨量跳过阈值（毫米）'),
			('rainfall_check_window_hours', '2', '启动前降雨量检查窗口（小时）'),
			('circuit_breaker_threshold', '3', '区域连续失败熔断阈值（次）'),
			('circuit_breaker_cooldown_minutes', '30', '熔断冷却时长（分钟）')
		ON CONFLICT (key) DO NOTHING`,
	}

	for _, stmt := range statements {
		if err := DB.Exec(stmt).Error; err != nil {
			applogger.Error("Migration failed", zap.String("statement", stmt), zap.Error(err))
			return err
		}
	}

	applogger.Info("Database migration completed")
	return nil
}
