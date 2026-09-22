package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"irrigation/internal/models"
	"irrigation/pkg/database"
	"irrigation/pkg/logger"
	redispkg "irrigation/pkg/redis"
)

// 业务错误
var (
	ErrZoneBusy                 = errors.New("该区域已有进行中的灌溉任务")
	ErrBreakerOpen              = errors.New("该区域处于熔断状态，请稍后再试")
	ErrLogNotFound              = errors.New("irrigation log not found")
	ErrLogNotInProgress         = errors.New("该执行记录已结束，不可重复完成")
	ErrMissingZone              = errors.New("zone_id is required")
	ErrMissingIdempotencyKey    = errors.New("idempotency_key is required")
	ErrCompletionErrorMsgNeeded = errors.New("失败完成必须提供 error_message")
	ErrEndTimeBeforeStart       = errors.New("end_time 早于开始时间")
)

const (
	// RainfallThresholdMM 启动前两小时降雨量阈值（毫米），达到则跳过灌溉
	RainfallThresholdMM = 5.0
	// RainfallLookback 降雨量统计窗口
	RainfallLookback = 2 * time.Hour
	// WaterUsagePerSecond 用水量估算速率（升/秒）
	WaterUsagePerSecond = 0.1
	// BreakerFailureThreshold 连续失败达到该次数后触发区域熔断
	BreakerFailureThreshold = 3
	// BreakerCooldown 熔断冷却时长
	BreakerCooldown = 30 * time.Minute
)

type IrrigationService struct{}

func NewIrrigationService() *IrrigationService {
	return &IrrigationService{}
}

func breakerRedisKey(zoneID uint) string {
	return fmt.Sprintf("irrigation:zone:%d:breaker", zoneID)
}

func failureCountRedisKey(zoneID uint) string {
	return fmt.Sprintf("irrigation:zone:%d:consecutive_failures", zoneID)
}

// StartIrrigation 启动一次灌溉执行。
// 幂等：同一 idempotencyKey 重复调用返回原执行记录（deduplicated=true）。
// 互斥：区域已有进行中记录时返回 ErrZoneBusy；区域熔断中返回 ErrBreakerOpen。
// 降雨保护：启动前两小时降雨量达到阈值时不启动阀门，只落一条 skipped 记录。
func (s *IrrigationService) StartIrrigation(scheduleID *uint, zoneID *uint, triggerType models.TriggerType, idempotencyKey string) (log *models.IrrigationLog, deduplicated bool, err error) {
	if zoneID == nil {
		return nil, false, ErrMissingZone
	}
	if idempotencyKey == "" {
		return nil, false, ErrMissingIdempotencyKey
	}

	// 幂等键命中，直接返回原执行记录
	if existing, err := s.getByIdempotencyKey(idempotencyKey); err != nil {
		return nil, false, err
	} else if existing != nil {
		return existing, true, nil
	}

	// 熔断检查
	if s.isBreakerOpen(*zoneID) {
		return nil, false, ErrBreakerOpen
	}

	// 降雨保护：达到阈值只落跳过记录，不启动阀门
	rainfall, shouldSkip, err := s.checkRainfallSkip(*zoneID, scheduleID)
	if err != nil {
		return nil, false, err
	}
	if shouldSkip {
		return s.createSkippedLog(scheduleID, zoneID, triggerType, idempotencyKey, rainfall)
	}

	newLog := &models.IrrigationLog{
		ScheduleID:     scheduleID,
		ZoneID:         zoneID,
		TriggerType:    triggerType,
		IdempotencyKey: &idempotencyKey,
		StartTime:      time.Now(),
		Status:         models.ExecutionStatusInProgress,
	}

	if err := database.DB.Create(newLog).Error; err != nil {
		// 唯一约束冲突（并发场景）：幂等键冲突返回原记录，区域进行中冲突返回业务冲突
		if existing, qErr := s.getByIdempotencyKey(idempotencyKey); qErr == nil && existing != nil {
			return existing, true, nil
		}
		if current, qErr := s.GetCurrentExecution(*zoneID); qErr == nil && current != nil {
			return nil, false, ErrZoneBusy
		}
		return nil, false, err
	}

	return newLog, false, nil
}

// createSkippedLog 落一条跳过记录并说明原因；幂等键冲突时返回原记录
func (s *IrrigationService) createSkippedLog(scheduleID *uint, zoneID *uint, triggerType models.TriggerType, idempotencyKey string, rainfall float64) (*models.IrrigationLog, bool, error) {
	now := time.Now()
	duration := 0
	waterUsage := 0.0
	reason := fmt.Sprintf("启动前两小时降雨量 %.2fmm 达到 %.2fmm 阈值，跳过灌溉", rainfall, RainfallThresholdMM)

	skipped := &models.IrrigationLog{
		ScheduleID:     scheduleID,
		ZoneID:         zoneID,
		TriggerType:    triggerType,
		IdempotencyKey: &idempotencyKey,
		StartTime:      now,
		EndTime:        &now,
		Duration:       &duration,
		WaterUsage:     &waterUsage,
		Status:         models.ExecutionStatusSkipped,
		SkipReason:     &reason,
	}

	if err := database.DB.Create(skipped).Error; err != nil {
		if existing, qErr := s.getByIdempotencyKey(idempotencyKey); qErr == nil && existing != nil {
			return existing, true, nil
		}
		return nil, false, err
	}

	logger.Info("Irrigation skipped due to rainfall",
		zap.Uint("zone_id", *zoneID),
		zap.Float64("rainfall", rainfall),
	)
	return skipped, false, nil
}

// checkRainfallSkip 检查启动前两小时降雨量是否达到跳过阈值
func (s *IrrigationService) checkRainfallSkip(zoneID uint, scheduleID *uint) (float64, bool, error) {
	sensorID, err := s.findRainSensorID(zoneID, scheduleID)
	if err != nil {
		return 0, false, err
	}
	if sensorID == nil {
		return 0, false, nil
	}

	rainfall, err := NewSensorService().CheckRecentRainfall(*sensorID, RainfallLookback)
	if err != nil {
		return 0, false, err
	}
	return rainfall, rainfall >= RainfallThresholdMM, nil
}

// findRainSensorID 优先使用计划绑定的雨量传感器，否则取区域内第一个雨量传感器
func (s *IrrigationService) findRainSensorID(zoneID uint, scheduleID *uint) (*uint, error) {
	if scheduleID != nil {
		var schedule models.IrrigationSchedule
		if err := database.DB.Select("rain_sensor_id").First(&schedule, *scheduleID).Error; err != nil {
			return nil, err
		}
		if schedule.RainSensorID != nil {
			return schedule.RainSensorID, nil
		}
	}

	var device models.Device
	err := database.DB.Select("id").
		Where("zone_id = ? AND type = ?", zoneID, models.DeviceTypeRainSensor).
		Order("id").
		First(&device).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &device.ID, nil
}

// CompleteIrrigation 完成一次灌溉执行。
// 只处理进行中记录；按结束时间计算实际时长和用水量；重复完成返回 ErrLogNotInProgress 不会再次累计。
// 失败完成时告警与执行记录在同一事务中提交，任一步失败整体回滚。
func (s *IrrigationService) CompleteIrrigation(logID uint, success bool, endTime time.Time, errorMsg *string) (*models.IrrigationLog, error) {
	if endTime.IsZero() {
		endTime = time.Now()
	}

	log, err := s.GetLogByID(logID)
	if err != nil {
		return nil, err
	}

	// 非法参数校验：直接返回，不改动执行记录或告警
	if !success && (errorMsg == nil || *errorMsg == "") {
		return nil, ErrCompletionErrorMsgNeeded
	}
	if endTime.Before(log.StartTime) {
		return nil, ErrEndTimeBeforeStart
	}
	if log.Status != models.ExecutionStatusInProgress {
		return nil, ErrLogNotInProgress
	}

	duration := int(endTime.Sub(log.StartTime).Seconds())
	updates := map[string]interface{}{
		"end_time": endTime,
		"duration": duration,
	}
	if success {
		updates["status"] = models.ExecutionStatusSuccess
		updates["water_usage"] = float64(duration) * WaterUsagePerSecond
	} else {
		updates["status"] = models.ExecutionStatusFailed
		updates["error_message"] = *errorMsg
	}

	tx := database.DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	// 条件更新保证只有进行中记录会被完成，并发/重复完成不会再次累计
	result := tx.Model(&models.IrrigationLog{}).
		Where("id = ? AND status = ?", logID, models.ExecutionStatusInProgress).
		Updates(updates)
	if result.Error != nil {
		tx.Rollback()
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		return nil, ErrLogNotInProgress
	}

	if !success {
		alert := &models.Alert{
			Type:    models.AlertTypeIrrigationFailed,
			Level:   models.AlertLevelCritical,
			Title:   "灌溉执行失败",
			Message: fmt.Sprintf("区域 %d 灌溉执行失败: %s", ptrUintValue(log.ZoneID), *errorMsg),
			Status:  models.AlertStatusNew,
		}
		if err := tx.Create(alert).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// 更新区域熔断器状态（Redis 故障不影响主流程）
	if log.ZoneID != nil {
		s.recordCompletionResult(*log.ZoneID, success)
	}

	return s.GetLogByID(logID)
}

func ptrUintValue(v *uint) uint {
	if v == nil {
		return 0
	}
	return *v
}

// recordCompletionResult 成功清零连续失败计数并解除熔断；失败累计并在达到阈值后熔断
func (s *IrrigationService) recordCompletionResult(zoneID uint, success bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if redispkg.Client == nil {
		return
	}

	if success {
		if err := redispkg.Client.Del(ctx, failureCountRedisKey(zoneID), breakerRedisKey(zoneID)).Err(); err != nil {
			logger.Warn("Failed to reset circuit breaker", zap.Uint("zone_id", zoneID), zap.Error(err))
		}
		return
	}

	failures, err := redispkg.Client.Incr(ctx, failureCountRedisKey(zoneID)).Result()
	if err != nil {
		logger.Warn("Failed to increase failure counter", zap.Uint("zone_id", zoneID), zap.Error(err))
		return
	}
	if failures >= BreakerFailureThreshold {
		if err := redispkg.Client.Set(ctx, breakerRedisKey(zoneID), "open", BreakerCooldown).Err(); err != nil {
			logger.Warn("Failed to open circuit breaker", zap.Uint("zone_id", zoneID), zap.Error(err))
			return
		}
		logger.Warn("Zone circuit breaker opened",
			zap.Uint("zone_id", zoneID),
			zap.Int64("consecutive_failures", failures),
		)
	}
}

func (s *IrrigationService) isBreakerOpen(zoneID uint) bool {
	if redispkg.Client == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	exists, err := redispkg.Client.Exists(ctx, breakerRedisKey(zoneID)).Result()
	if err != nil {
		logger.Warn("Failed to check circuit breaker", zap.Uint("zone_id", zoneID), zap.Error(err))
		return false
	}
	return exists > 0
}

// BreakerStatus 区域熔断状态
type BreakerStatus struct {
	Open                bool  `json:"open"`
	ConsecutiveFailures int64 `json:"consecutive_failures"`
	FailureThreshold    int   `json:"failure_threshold"`
	RemainingCooldown   int64 `json:"remaining_cooldown_seconds"`
}

func (s *IrrigationService) GetBreakerStatus(zoneID uint) *BreakerStatus {
	status := &BreakerStatus{FailureThreshold: BreakerFailureThreshold}
	if redispkg.Client == nil {
		return status
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ttl, err := redispkg.Client.TTL(ctx, breakerRedisKey(zoneID)).Result()
	if err == nil && ttl > 0 {
		status.Open = true
		status.RemainingCooldown = int64(ttl.Seconds())
	}

	failures, err := redispkg.Client.Get(ctx, failureCountRedisKey(zoneID)).Int64()
	if err == nil {
		status.ConsecutiveFailures = failures
	}

	return status
}

func (s *IrrigationService) getByIdempotencyKey(key string) (*models.IrrigationLog, error) {
	var log models.IrrigationLog
	err := database.DB.Where("idempotency_key = ?", key).First(&log).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (s *IrrigationService) GetLogByID(id uint) (*models.IrrigationLog, error) {
	var log models.IrrigationLog
	if err := database.DB.First(&log, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrLogNotFound
		}
		return nil, err
	}
	return &log, nil
}

// GetCurrentExecution 返回区域当前进行中的执行记录，无则返回 nil
func (s *IrrigationService) GetCurrentExecution(zoneID uint) (*models.IrrigationLog, error) {
	var log models.IrrigationLog
	err := database.DB.
		Where("zone_id = ? AND status = ?", zoneID, models.ExecutionStatusInProgress).
		Order("start_time DESC").
		First(&log).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (s *IrrigationService) GetIrrigationHistory(zoneID *uint, startTime, endTime time.Time, limit int) ([]models.IrrigationLog, error) {
	var logs []models.IrrigationLog
	query := database.DB

	if zoneID != nil {
		query = query.Where("zone_id = ?", *zoneID)
	}
	if !startTime.IsZero() {
		query = query.Where("start_time >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("start_time <= ?", endTime)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Order("start_time DESC, id DESC").Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

type WaterUsageStats struct {
	TotalUsage      float64 `json:"total_usage"`
	Duration        int64   `json:"duration"`
	IrrigationCount int64   `json:"irrigation_count"`
}

func (s *IrrigationService) GetWaterUsageStats(zoneID *uint, startTime, endTime time.Time) (*WaterUsageStats, error) {
	var stats WaterUsageStats
	query := database.DB.Model(&models.IrrigationLog{}).
		Select("COALESCE(SUM(water_usage), 0) as total_usage, COALESCE(COUNT(*), 0) as irrigation_count").
		Where("status = ?", models.ExecutionStatusSuccess)

	if zoneID != nil {
		query = query.Where("zone_id = ?", *zoneID)
	}
	if !startTime.IsZero() {
		query = query.Where("start_time >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("start_time <= ?", endTime)
	}

	err := query.Scan(&stats).Error
	return &stats, err
}

type ZoneWaterUsage struct {
	ZoneID     uint    `json:"zone_id"`
	ZoneName   string  `json:"zone_name"`
	WaterUsage float64 `json:"water_usage"`
	Percentage float64 `json:"percentage"`
}

func (s *IrrigationService) GetZoneWaterUsage(startTime, endTime time.Time) ([]ZoneWaterUsage, error) {
	var zoneUsages []ZoneWaterUsage

	query := `
		SELECT
			z.id as zone_id,
			z.name as zone_name,
			COALESCE(SUM(il.water_usage), 0) as water_usage
		FROM irrigation_zones z
		LEFT JOIN irrigation_logs il ON z.id = il.zone_id
			AND il.status = 'success'
			AND il.start_time >= ?
			AND il.start_time <= ?
		GROUP BY z.id, z.name
		ORDER BY water_usage DESC
	`

	err := database.DB.Raw(query, startTime, endTime).Scan(&zoneUsages).Error
	if err != nil {
		return nil, err
	}

	var total float64
	for _, zu := range zoneUsages {
		total += zu.WaterUsage
	}

	if total > 0 {
		for i := range zoneUsages {
			zoneUsages[i].Percentage = (zoneUsages[i].WaterUsage / total) * 100
		}
	}

	return zoneUsages, nil
}
