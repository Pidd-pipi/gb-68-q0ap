package controllers

import (
	"github.com/gin-gonic/gin"

	"irrigation/internal/services"
	"irrigation/pkg/response"
)

type ConfigController struct {
	configService *services.ConfigService
}

func NewConfigController() *ConfigController {
	return &ConfigController{
		configService: services.NewConfigService(),
	}
}

// ListConfigs godoc
// @Summary 获取所有系统配置
// @Description 获取所有系统配置项
// @Tags 系统配置
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {array} models.SystemConfig
// @Router /api/configs [get]
func (c *ConfigController) List(ctx *gin.Context) {
	configs, err := c.configService.GetAllConfigs()
	if err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}
	response.Success(ctx, configs)
}

// GetConfig godoc
// @Summary 获取指定配置
// @Description 根据key获取配置项
// @Tags 系统配置
// @Security ApiKeyAuth
// @Produce json
// @Param key path string true "配置key"
// @Success 200 {object} models.SystemConfig
// @Router /api/configs/{key} [get]
func (c *ConfigController) Get(ctx *gin.Context) {
	key := ctx.Param("key")
	config, err := c.configService.GetConfig(key)
	if err != nil {
		response.NotFound(ctx, "Config not found")
		return
	}
	response.Success(ctx, config)
}

// SetConfig godoc
// @Summary 设置配置
// @Description 设置或更新配置项
// @Tags 系统配置
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param request body map[string]string true "配置信息"
// @Success 200 {object} response.Response
// @Router /api/configs [post]
func (c *ConfigController) Set(ctx *gin.Context) {
	var req struct {
		Key         string `json:"key" binding:"required"`
		Value       string `json:"value" binding:"required"`
		Description string `json:"description"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid request body")
		return
	}

	if err := c.configService.SetConfig(req.Key, req.Value, req.Description); err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}

	response.Success(ctx, nil)
}
