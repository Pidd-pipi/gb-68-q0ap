package controllers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"irrigation/internal/models"
	"irrigation/internal/services"
	"irrigation/pkg/response"
)

type ScheduleController struct {
	scheduleService *services.ScheduleService
}

func NewScheduleController() *ScheduleController {
	return &ScheduleController{
		scheduleService: services.NewScheduleService(),
	}
}

// ListSchedules godoc
// @Summary 获取灌溉计划列表
// @Description 获取所有灌溉计划，支持按区域、状态筛选
// @Tags 灌溉计划
// @Security ApiKeyAuth
// @Produce json
// @Param zone_id query int false "区域ID"
// @Param status query string false "计划状态"
// @Success 200 {array} models.IrrigationSchedule
// @Router /api/schedules [get]
func (c *ScheduleController) List(ctx *gin.Context) {
	var zoneID *uint
	if zoneIDStr := ctx.Query("zone_id"); zoneIDStr != "" {
		id, _ := strconv.ParseUint(zoneIDStr, 10, 32)
		idUint := uint(id)
		zoneID = &idUint
	}

	var status *string
	if s := ctx.Query("status"); s != "" {
		status = &s
	}

	schedules, err := c.scheduleService.ListSchedules(zoneID, status)
	if err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}

	response.Success(ctx, schedules)
}

// GetSchedule godoc
// @Summary 获取灌溉计划详情
// @Description 根据ID获取灌溉计划详情
// @Tags 灌溉计划
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "计划ID"
// @Success 200 {object} models.IrrigationSchedule
// @Router /api/schedules/{id} [get]
func (c *ScheduleController) Get(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	schedule, err := c.scheduleService.GetScheduleByID(uint(id))
	if err != nil {
		response.NotFound(ctx, "Schedule not found")
		return
	}
	response.Success(ctx, schedule)
}

// CreateSchedule godoc
// @Summary 创建灌溉计划
// @Description 创建新的灌溉计划
// @Tags 灌溉计划
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param request body models.IrrigationSchedule true "计划信息"
// @Success 201 {object} models.IrrigationSchedule
// @Router /api/schedules [post]
func (c *ScheduleController) Create(ctx *gin.Context) {
	var schedule models.IrrigationSchedule
	if err := ctx.ShouldBindJSON(&schedule); err != nil {
		response.BadRequest(ctx, "Invalid request body")
		return
	}

	if err := c.scheduleService.CreateSchedule(&schedule); err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}

	response.Created(ctx, schedule)
}

// UpdateSchedule godoc
// @Summary 更新灌溉计划
// @Description 更新灌溉计划信息
// @Tags 灌溉计划
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "计划ID"
// @Param request body map[string]interface{} true "更新信息"
// @Success 200 {object} response.Response
// @Router /api/schedules/{id} [put]
func (c *ScheduleController) Update(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	
	var updates map[string]interface{}
	if err := ctx.ShouldBindJSON(&updates); err != nil {
		response.BadRequest(ctx, "Invalid request body")
		return
	}

	if err := c.scheduleService.UpdateSchedule(uint(id), updates); err != nil {
		response.NotFound(ctx, err.Error())
		return
	}

	response.Success(ctx, nil)
}

// DeleteSchedule godoc
// @Summary 删除灌溉计划
// @Description 删除灌溉计划
// @Tags 灌溉计划
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "计划ID"
// @Success 200 {object} response.Response
// @Router /api/schedules/{id} [delete]
func (c *ScheduleController) Delete(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	
	if err := c.scheduleService.DeleteSchedule(uint(id)); err != nil {
		response.NotFound(ctx, err.Error())
		return
	}

	response.Success(ctx, nil)
}

// EnableSchedule godoc
// @Summary 启用灌溉计划
// @Description 启用指定灌溉计划
// @Tags 灌溉计划
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "计划ID"
// @Success 200 {object} response.Response
// @Router /api/schedules/{id}/enable [post]
func (c *ScheduleController) Enable(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	
	if err := c.scheduleService.SetScheduleStatus(uint(id), models.ScheduleStatusActive); err != nil {
		response.NotFound(ctx, err.Error())
		return
	}

	response.Success(ctx, nil)
}

// DisableSchedule godoc
// @Summary 禁用灌溉计划
// @Description 禁用指定灌溉计划
// @Tags 灌溉计划
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "计划ID"
// @Success 200 {object} response.Response
// @Router /api/schedules/{id}/disable [post]
func (c *ScheduleController) Disable(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	
	if err := c.scheduleService.SetScheduleStatus(uint(id), models.ScheduleStatusInactive); err != nil {
		response.NotFound(ctx, err.Error())
		return
	}

	response.Success(ctx, nil)
}
