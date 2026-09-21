package controllers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"irrigation/internal/models"
	"irrigation/internal/services"
	"irrigation/pkg/response"
)

type DeviceController struct {
	deviceService *services.DeviceService
}

func NewDeviceController() *DeviceController {
	return &DeviceController{
		deviceService: services.NewDeviceService(),
	}
}

// ListDevices godoc
// @Summary 获取设备列表
// @Description 获取所有设备，支持按区域、类型、状态筛选
// @Tags 设备管理
// @Security ApiKeyAuth
// @Produce json
// @Param zone_id query int false "区域ID"
// @Param type query string false "设备类型"
// @Param status query string false "设备状态"
// @Success 200 {array} models.Device
// @Router /api/devices [get]
func (c *DeviceController) List(ctx *gin.Context) {
	var zoneID *uint
	if zoneIDStr := ctx.Query("zone_id"); zoneIDStr != "" {
		id, _ := strconv.ParseUint(zoneIDStr, 10, 32)
		idUint := uint(id)
		zoneID = &idUint
	}

	var deviceType *string
	if t := ctx.Query("type"); t != "" {
		deviceType = &t
	}

	var status *string
	if s := ctx.Query("status"); s != "" {
		status = &s
	}

	devices, err := c.deviceService.ListDevices(zoneID, deviceType, status)
	if err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}
	response.Success(ctx, devices)
}

// GetDevice godoc
// @Summary 获取设备详情
// @Description 根据ID获取设备详情
// @Tags 设备管理
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "设备ID"
// @Success 200 {object} models.Device
// @Router /api/devices/{id} [get]
func (c *DeviceController) Get(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	device, err := c.deviceService.GetDeviceByID(uint(id))
	if err != nil {
		response.NotFound(ctx, "Device not found")
		return
	}
	response.Success(ctx, device)
}

// CreateDevice godoc
// @Summary 创建设备
// @Description 注册新设备
// @Tags 设备管理
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param request body models.Device true "设备信息"
// @Success 201 {object} models.Device
// @Router /api/devices [post]
func (c *DeviceController) Create(ctx *gin.Context) {
	var device models.Device
	if err := ctx.ShouldBindJSON(&device); err != nil {
		response.BadRequest(ctx, "Invalid request body")
		return
	}

	if err := c.deviceService.CreateDevice(&device); err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}

	response.Created(ctx, device)
}

// UpdateDevice godoc
// @Summary 更新设备
// @Description 更新设备信息
// @Tags 设备管理
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "设备ID"
// @Param request body map[string]interface{} true "更新信息"
// @Success 200 {object} response.Response
// @Router /api/devices/{id} [put]
func (c *DeviceController) Update(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	
	var updates map[string]interface{}
	if err := ctx.ShouldBindJSON(&updates); err != nil {
		response.BadRequest(ctx, "Invalid request body")
		return
	}

	if err := c.deviceService.UpdateDevice(uint(id), updates); err != nil {
		response.NotFound(ctx, err.Error())
		return
	}

	response.Success(ctx, nil)
}

// DeleteDevice godoc
// @Summary 删除设备
// @Description 删除设备
// @Tags 设备管理
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "设备ID"
// @Success 200 {object} response.Response
// @Router /api/devices/{id} [delete]
func (c *DeviceController) Delete(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	
	if err := c.deviceService.DeleteDevice(uint(id)); err != nil {
		response.NotFound(ctx, err.Error())
		return
	}

	response.Success(ctx, nil)
}

// Heartbeat godoc
// @Summary 设备心跳
// @Description 设备上报心跳
// @Tags 设备管理
// @Accept json
// @Produce json
// @Param serial path string true "设备序列号"
// @Success 200 {object} response.Response
// @Router /api/devices/{serial}/heartbeat [post]
func (c *DeviceController) Heartbeat(ctx *gin.Context) {
	serial := ctx.Param("serial")
	
	if err := c.deviceService.UpdateHeartbeat(serial); err != nil {
		response.InternalServerError(ctx, err.Error())
		return
	}

	response.Success(ctx, nil)
}
