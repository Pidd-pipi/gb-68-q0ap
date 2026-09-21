package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"irrigation/internal/config"
	"irrigation/pkg/logger"
)

var Client *redis.Client

func Init() error {
	Client = redis.NewClient(&redis.Options{
		Addr:     config.AppSettings.Redis.Addr(),
		Password: config.AppSettings.Redis.Password,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := Client.Ping(ctx).Err(); err != nil {
		logger.Fatal("Failed to connect to Redis", zap.Error(err))
	}

	logger.Info("Redis connected successfully")
	return nil
}
