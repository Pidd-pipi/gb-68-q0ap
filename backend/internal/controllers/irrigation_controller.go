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

// ManualIrrigate godoc
// @Summary 手动灌溉
// @Description 触发手动灌溉，需携带幂等键；同一键重复调用返回原执行记录；区域已有进行中任务或熔断中时返回冲突
// @Tags 灌溉执行
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param request body object{zone_id=int,idempotency_key=string} true "区域ID与幂等键"
// @Success 200 {object} models.IrrigationLog
// @Failure 409 {object} response.Response
// @Router /api/irrigation/manual [post]
func (c *IrrigationController) ManualIrrigate(ctx *gin.Context) {
	var req struct {
		ZoneID         uint   `json:"zone_id" binding:"required"`
		IdempotencyKey string `json:"idempotency_key" binding:"required,max=100"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid request body: zone_id and idempotency_key are required")
		return
	}

	log, _, err := c.irrigationService.StartIrrigation(nil, &req.ZoneID, models.TriggerTypeManual, req.IdempotencyKey)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrZoneBusy), errors.Is(err, services.ErrBreakerOpen):
			response.Conflict(ctx, err.Error())
		default:
			response.InternalServerError(ctx, err.Error())
		}
		return
	}

	response.Success(ctx, log)
}

// CompleteIrrigate godoc
// @Summary 完成灌溉执行
// @Description 完成一条进行中的灌溉记录，按结束时间计算实际时长和用水量；重复完成返回冲突；失败完成必须提供 error_message
// @Tags 灌溉执行
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "执行记录ID"
// @Param request body object{success=bool,error_message=string,end_time=string} true "完成结果"
// @Success 200 {object} models.IrrigationLog
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/irrigation/{id}/complete [post]
func (c *IrrigationController) CompleteIrrigate(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(ctx, "Invalid irrigation log id")
		return
	}

	var req struct {
		Success      *bool   `json:"success" binding:"required"`
		ErrorMessage *string `json:"error_message" binding:"omitempty,max=500"`
		EndTime      *string `json:"end_time"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid request body: success is required")
		return
	}

	endTime := time.Now()
	if req.EndTime != nil && *req.EndTime != "" {
		parsed, err := time.Parse(time.RFC3339, *req.EndTime)
		if err != nil {
			response.BadRequest(ctx, "Invalid end_time format, expect RFC3339")
			return
		}
		endTime = parsed
	}

	log, err := c.irrigationService.CompleteIrrigation(uint(id), *req.Success, endTime, req.ErrorMessage)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrLogNotFound):
			response.NotFound(ctx, err.Error())
		case errors.Is(err, services.ErrLogNotInProgress):
			response.Conflict(ctx, err.Error())
		case errors.Is(err, services.ErrCompletionErrorMsgNeeded), errors.Is(err, services.ErrEndTimeBeforeStart):
			response.BadRequest(ctx, err.Error())
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
