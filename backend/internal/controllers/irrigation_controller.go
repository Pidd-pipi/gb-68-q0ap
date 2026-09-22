package controllers

import (
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"irrigation/internal/models"
	"irrigation/internal/services"
	"irrigation/pkg/response"
)

type IrrigationController struct {
	irrigationService *services.IrrigationService
}

func NewIrrigationController() *IrrigationController {
	return &IrrigationController{
		irrigationService: services.NewIrrigationService(),
	}
}

// StartIrrigation godoc
// @Summary 启动灌溉（安全闭环）
// @Description 带区域和幂等键启动灌溉；同一幂等键重复调用返回原执行记录；
// @Description 区域已有进行中记录时返回业务冲突；启动前两小时降雨量达到阈值时只落跳过记录
// @Tags 灌溉执行
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param request body object true "启动请求 {zone_id, idempotency_key}"
// @Success 200 {object} models.IrrigationLog
// @Failure 409 {object} response.Response "区域已有进行中灌溉或熔断中"
// @Router /api/irrigation/start [post]
func (c *IrrigationController) StartIrrigation(ctx *gin.Context) {
	var req struct {
		ZoneID         uint   `json:"zone_id" binding:"required"`
		IdempotencyKey string `json:"idempotency_key" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "zone_id and idempotency_key are required")
		return
	}

	c.start(ctx, services.StartIrrigationRequest{
		ZoneID:         req.ZoneID,
		TriggerType:    models.TriggerTypeManual,
		IdempotencyKey: req.IdempotencyKey,
	})
}

// ManualIrrigate godoc
// @Summary 手动灌溉
// @Description 触发手动灌溉（兼容接口，内部走安全闭环；幂等键可选，缺省由服务端生成）
// @Tags 灌溉执行
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param request body object true "启动请求 {zone_id, idempotency_key?}"
// @Success 200 {object} models.IrrigationLog
// @Failure 409 {object} response.Response "区域已有进行中灌溉或熔断中"
// @Router /api/irrigation/manual [post]
func (c *IrrigationController) ManualIrrigate(ctx *gin.Context) {
	var req struct {
		ZoneID         uint   `json:"zone_id" binding:"required"`
		IdempotencyKey string `json:"idempotency_key"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid request body")
		return
	}

	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = "manual-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}

	c.start(ctx, services.StartIrrigationRequest{
		ZoneID:         req.ZoneID,
		TriggerType:    models.TriggerTypeManual,
		IdempotencyKey: idempotencyKey,
	})
}

func (c *IrrigationController) start(ctx *gin.Context, req services.StartIrrigationRequest) {
	result, err := c.irrigationService.StartIrrigation(req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrZoneNotFound):
			response.NotFound(ctx, err.Error())
		case errors.Is(err, services.ErrZoneBusy):
			response.Conflict(ctx, "区域已有进行中的灌溉任务")
		case errors.Is(err, services.ErrCircuitOpen):
			response.Conflict(ctx, "区域灌溉熔断中: "+err.Error())
		default:
			response.InternalServerError(ctx, err.Error())
		}
		return
	}

	response.Success(ctx, result.Log)
}

// CompleteIrrigation godoc
// @Summary 完成灌溉
// @Description 只处理进行中记录，按结束时间计算实际时长和用水量；
// @Description 重复完成返回业务冲突且不再次累计；非法参数不改动执行记录或告警
// @Tags 灌溉执行
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "执行记录ID"
// @Param request body object true "完成请求 {success, end_time?, error_message?}"
// @Success 200 {object} models.IrrigationLog
// @Failure 400 {object} response.Response "非法参数"
// @Failure 409 {object} response.Response "记录不在进行中状态"
// @Router /api/irrigation/{id}/complete [post]
func (c *IrrigationController) CompleteIrrigation(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(ctx, "Invalid irrigation log id")
		return
	}

	var req struct {
		Success  *bool      `json:"success" binding:"required"`
		EndTime  *time.Time `json:"end_time"`
		ErrorMsg *string    `json:"error_message"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "success field is required")
		return
	}

	log, err := c.irrigationService.CompleteIrrigation(uint(id), services.CompleteIrrigationRequest{
		Success:  *req.Success,
		EndTime:  req.EndTime,
		ErrorMsg: req.ErrorMsg,
	})
	if err != nil {
		switch {
		case errors.Is(err, services.ErrLogNotFound):
			response.NotFound(ctx, err.Error())
		case errors.Is(err, services.ErrInvalidCompletion):
			response.BadRequest(ctx, err.Error())
		case errors.Is(err, services.ErrNotInProgress):
			response.Conflict(ctx, "灌溉记录不在进行中状态，可能已完成")
		default:
			response.InternalServerError(ctx, err.Error())
		}
		return
	}

	response.Success(ctx, log)
}

// GetIrrigationHistory godoc
// @Summary 获取灌溉历史
// @Description 获取灌溉执行历史记录
// @Tags 灌溉执行
// @Security ApiKeyAuth
// @Produce json
// @Param zone_id query int false "区域ID"
// @Param start_time query string false "开始时间 (RFC3339)"
// @Param end_time query string false "结束时间 (RFC3339)"
// @Param limit query int false "返回数量限制" default(100)
// @Success 200 {array} models.IrrigationLog
// @Router /api/irrigation/history [get]
func (c *IrrigationController) GetHistory(ctx *gin.Context) {
	var zoneID *uint
	if zoneIDStr := ctx.Query("zone_id"); zoneIDStr != "" {
		id, _ := strconv.ParseUint(zoneIDStr, 10, 32)
		idUint := uint(id)
		zoneID = &idUint
	}

	var startTime, endTime time.Time
	if startStr := ctx.Query("start_time"); startStr != "" {
		startTime, _ = time.Parse(time.RFC3339, startStr)
	}
	if endStr := ctx.Query("end_time"); endStr != "" {
		endTime, _ = time.Parse(time.RFC3339, endStr)
	}

	limit := 100
	if limitStr := ctx.Query("limit"); limitStr != "" {
		limit, _ = strconv.Atoi(limitStr)
	}

	logs, err := c.irrigationService.GetIrrigationHistory(zoneID, startTime, endTime, limit)
	if err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}

	response.Success(ctx, logs)
}

// GetWaterUsageStats godoc
// @Summary 获取用水量统计
// @Description 获取指定时间段的用水量统计
// @Tags 用水统计
// @Security ApiKeyAuth
// @Produce json
// @Param zone_id query int false "区域ID"
// @Param start_time query string false "开始时间 (RFC3339)"
// @Param end_time query string false "结束时间 (RFC3339)"
// @Success 200 {object} services.WaterUsageStats
// @Router /api/statistics/water-usage [get]
func (c *IrrigationController) GetWaterUsageStats(ctx *gin.Context) {
	var zoneID *uint
	if zoneIDStr := ctx.Query("zone_id"); zoneIDStr != "" {
		id, _ := strconv.ParseUint(zoneIDStr, 10, 32)
		idUint := uint(id)
		zoneID = &idUint
	}

	var startTime, endTime time.Time
	if startStr := ctx.Query("start_time"); startStr != "" {
		startTime, _ = time.Parse(time.RFC3339, startStr)
	}
	if endStr := ctx.Query("end_time"); endStr != "" {
		endTime, _ = time.Parse(time.RFC3339, endStr)
	}

	stats, err := c.irrigationService.GetWaterUsageStats(zoneID, startTime, endTime)
	if err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}

	response.Success(ctx, stats)
}

// GetZoneWaterUsage godoc
// @Summary 获取各区域用水量
// @Description 获取各区域的用水量分布
// @Tags 用水统计
// @Security ApiKeyAuth
// @Produce json
// @Param start_time query string false "开始时间 (RFC3339)"
// @Param end_time query string false "结束时间 (RFC3339)"
// @Success 200 {array} services.ZoneWaterUsage
// @Router /api/statistics/zone-usage [get]
func (c *IrrigationController) GetZoneWaterUsage(ctx *gin.Context) {
	var startTime, endTime time.Time
	if startStr := ctx.Query("start_time"); startStr != "" {
		startTime, _ = time.Parse(time.RFC3339, startStr)
	} else {
		startTime = time.Now().AddDate(0, 0, -7)
	}
	if endStr := ctx.Query("end_time"); endStr != "" {
		endTime, _ = time.Parse(time.RFC3339, endStr)
	} else {
		endTime = time.Now()
	}

	usage, err := c.irrigationService.GetZoneWaterUsage(startTime, endTime)
	if err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}

	response.Success(ctx, usage)
}
