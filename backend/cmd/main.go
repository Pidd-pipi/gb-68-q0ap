package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"irrigation/internal/config"
	"irrigation/internal/routes"
	"irrigation/internal/scheduler"
	"irrigation/pkg/database"
	"irrigation/pkg/logger"
	redispkg "irrigation/pkg/redis"
)

// @title 智能灌溉管理系统 API
// @version 1.0
// @description 面向庭院花园和社区绿化场景的智能灌溉管理系统
// @host localhost:3109
// @BasePath /api
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
// @description Bearer Token (格式: Bearer {token})
func main() {
	config.Load()

	logger.Init(config.AppSettings.Log.Level)
	defer logger.Log.Sync()

	logger.Info("Application starting",
		zap.String("env", config.AppSettings.App.Env),
		zap.String("port", config.AppSettings.App.Port),
	)

	if err := database.Init(); err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}

	if err := redispkg.Init(); err != nil {
		logger.Fatal("Failed to initialize redis", zap.Error(err))
	}

	if config.AppSettings.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	routes.SetupRoutes(r)

	scheduler := scheduler.NewIrrigationScheduler()
	scheduler.Start()

	addr := fmt.Sprintf(":%s", config.AppSettings.App.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		logger.Info("Server starting on", zap.String("address", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited properly")
}
