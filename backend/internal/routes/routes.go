package routes

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"irrigation/internal/controllers"
	"irrigation/internal/middleware"
)

func SetupRoutes(r *gin.Engine) {
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "Service is healthy",
		})
	})

	api := r.Group("/api")
	{
		authController := controllers.NewAuthController()
		auth := api.Group("/auth")
		{
			auth.POST("/login", authController.Login)
			auth.GET("/profile", middleware.JWTAuth(), authController.GetProfile)
		}

		zones := api.Group("/zones", middleware.JWTAuth())
		{
			zoneController := controllers.NewZoneController()
			zones.GET("", zoneController.List)
			zones.GET("/:id", zoneController.Get)
			zones.POST("", zoneController.Create)
			zones.PUT("/:id", zoneController.Update)
			zones.DELETE("/:id", zoneController.Delete)
		}

		devices := api.Group("/devices")
		{
			deviceController := controllers.NewDeviceController()
			devices.POST("/:serial/heartbeat", deviceController.Heartbeat)
			
			protectedDevices := devices.Group("", middleware.JWTAuth())
			{
				protectedDevices.GET("", deviceController.List)
				protectedDevices.GET("/:id", deviceController.Get)
				protectedDevices.POST("", deviceController.Create)
				protectedDevices.PUT("/:id", deviceController.Update)
				protectedDevices.DELETE("/:id", deviceController.Delete)
			}
		}

		sensors := api.Group("/sensors")
		{
			sensorController := controllers.NewSensorController()
			sensors.POST("/data", sensorController.Create)
			sensors.POST("/data/batch", sensorController.BatchCreate)

			protectedSensors := sensors.Group("", middleware.JWTAuth())
			{
				protectedSensors.GET("/history/:device_id", sensorController.GetHistory)
				protectedSensors.GET("/latest/:device_id", sensorController.GetLatest)
			}
		}

		schedules := api.Group("/schedules", middleware.JWTAuth())
		{
			scheduleController := controllers.NewScheduleController()
			schedules.GET("", scheduleController.List)
			schedules.GET("/:id", scheduleController.Get)
			schedules.POST("", scheduleController.Create)
			schedules.PUT("/:id", scheduleController.Update)
			schedules.DELETE("/:id", scheduleController.Delete)
			schedules.POST("/:id/enable", scheduleController.Enable)
			schedules.POST("/:id/disable", scheduleController.Disable)
		}

		irrigation := api.Group("/irrigation", middleware.JWTAuth())
		{
			irrigationController := controllers.NewIrrigationController()
			irrigation.POST("/manual", irrigationController.ManualIrrigate)
			irrigation.GET("/history", irrigationController.GetHistory)
		}

		statistics := api.Group("/statistics", middleware.JWTAuth())
		{
			irrigationController := controllers.NewIrrigationController()
			statistics.GET("/water-usage", irrigationController.GetWaterUsageStats)
			statistics.GET("/zone-usage", irrigationController.GetZoneWaterUsage)
		}

		alerts := api.Group("/alerts", middleware.JWTAuth())
		{
			alertController := controllers.NewAlertController()
			alerts.GET("", alertController.List)
			alerts.GET("/unacknowledged-count", alertController.GetUnacknowledgedCount)
			alerts.GET("/:id", alertController.Get)
			alerts.POST("/:id/acknowledge", alertController.Acknowledge)
			alerts.POST("/:id/resolve", alertController.Resolve)
		}

		configs := api.Group("/configs", middleware.JWTAuth())
		{
			configController := controllers.NewConfigController()
			configs.GET("", configController.List)
			configs.GET("/:key", configController.Get)
			configs.POST("", configController.Set)
		}
	}
}
