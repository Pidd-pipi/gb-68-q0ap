package controllers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"irrigation/internal/services"
	"irrigation/pkg/response"
)

type AlertController struct {
	alertService *services.AlertService
}

func NewAlertController() *AlertController {
	return &AlertController{
		alertService: services.NewAlertService(),
	}
}

// ListAlerts godoc
// @Summary 获取告警列表
// @Description 获取所有告警，支持按状态、级别筛选
// @Tags 告警管理
// @Security ApiKeyAuth
// @Produce json
// @Param status query string false "告警状态"
// @Param level query string false "告警级别"
// @Param limit query int false "返回数量限制" default(50)
// @Success 200 {array} models.Alert
// @Router /api/alerts [get]
func (c *AlertController) List(ctx *gin.Context) {
	var status *string
	if s := ctx.Query("status"); s != "" {
		status = &s
	}

	var level *string
	if l := ctx.Query("level"); l != "" {
		level = &l
	}

	limit := 50
	if limitStr := ctx.Query("limit"); limitStr != "" {
		limit, _ = strconv.Atoi(limitStr)
	}

	alerts, err := c.alertService.ListAlerts(status, level, limit)
	if err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}

	response.Success(ctx, alerts)
}

// GetAlert godoc
// @Summary 获取告警详情
// @Description 根据ID获取告警详情
// @Tags 告警管理
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "告警ID"
// @Success 200 {object} models.Alert
// @Router /api/alerts/{id} [get]
func (c *AlertController) Get(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	alert, err := c.alertService.GetAlertByID(uint(id))
	if err != nil {
		response.NotFound(ctx, "Alert not found")
		return
	}
	response.Success(ctx, alert)
}

// AcknowledgeAlert godoc
// @Summary 确认告警
// @Description 确认告警已读
// @Tags 告警管理
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "告警ID"
// @Success 200 {object} response.Response
// @Router /api/alerts/{id}/acknowledge [post]
func (c *AlertController) Acknowledge(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	
	if err := c.alertService.AcknowledgeAlert(uint(id)); err != nil {
		response.NotFound(ctx, err.Error())
		return
	}

	response.Success(ctx, nil)
}

// ResolveAlert godoc
// @Summary 解决告警
// @Description 标记告警已解决
// @Tags 告警管理
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "告警ID"
// @Success 200 {object} response.Response
// @Router /api/alerts/{id}/resolve [post]
func (c *AlertController) Resolve(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	
	if err := c.alertService.ResolveAlert(uint(id)); err != nil {
		response.NotFound(ctx, err.Error())
		return
	}

	response.Success(ctx, nil)
}

// GetUnacknowledgedCount godoc
// @Summary 获取未确认告警数量
// @Description 获取未确认告警的数量
// @Tags 告警管理
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} map[string]int64
// @Router /api/alerts/unacknowledged-count [get]
func (c *AlertController) GetUnacknowledgedCount(ctx *gin.Context) {
	count, err := c.alertService.GetUnacknowledgedCount()
	if err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}

	response.Success(ctx, map[string]int64{"count": count})
}
