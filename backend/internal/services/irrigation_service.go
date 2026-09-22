package services

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"irrigation/internal/models"
	"irrigation/pkg/database"
)

// 安全闭环业务错误
var (
	ErrZoneNotFound      = errors.New("irrigation zone not found")
	ErrZoneBusy          = errors.New("zone already has an in-progress irrigation")
	ErrCircuitOpen       = errors.New("circuit breaker is open for this zone")
	ErrLogNotFound       = errors.New("irrigation log not found")
	ErrNotInProgress     = errors.New("irrigation log is not in progress")
	ErrInvalidCompletion = errors.New("invalid completion parameters")
)

// 用水量估算系数：升/秒，与调度器原有估算保持一致
const waterUsagePerSecond = 0.1

// PostgreSQL 唯一约束冲突错误码
const pgUniqueViolation = "23505"

type IrrigationService struct {
	sensorService *SensorService
	configService *ConfigService
}

func NewIrrigationService() *IrrigationService {
	return &IrrigationService{
		sensorService: NewSensorService(),
		configService: NewConfigService(),
	}
}

type StartIrrigationRequest struct {
	ZoneID         uint
	ScheduleID     *uint
	TriggerType    models.TriggerType
	IdempotencyKey string
	RainSensorID   *uint // 计划指定的雨量传感器；为空则自动查找区域雨量传感器
}

type StartIrrigationResult struct {
	Log       *models.IrrigationLog
	Duplicate bool // 命中幂等键，返回原执行记录
	Skipped   bool // 降雨量达到阈值，未启动阀门
}

func (s *IrrigationService) StartIrrigation(req StartIrrigationRequest) (*StartIrrigationResult, error) {
	// 1. 幂等键命中：同一键重复调用直接返回原执行记录
	if req.IdempotencyKey != "" {
		if existing, err := s.findByIdempotencyKey(req.IdempotencyKey); err != nil {
			return nil, err
		} else if existing != nil {
			return &StartIrrigationResult{Log: existing, Duplicate: true}, nil
		}
	}

	// 2. 区域存在性与熔断状态
	zone, err := s.getZone(req.ZoneID)
	if err != nil {
		return nil, err
	}
	if zone.CircuitOpenUntil != nil && zone.CircuitOpenUntil.After(time.Now()) {
		return nil, fmt.Errorf("%w until %s", ErrCircuitOpen, zone.CircuitOpenUntil.Format(time.RFC3339))
	}

	// 3. 启动前降雨量检查：达到阈值不启动阀门，只落一条跳过记录并说明原因
	rainfall, checked, err := s.recentRainfall(req.ZoneID, req.RainSensorID)
	if err != nil {
		return nil, err
	}
	threshold := s.rainfallSkipThreshold()
	if checked && rainfall >= threshold {
		reason := fmt.Sprintf("启动前%d小时降雨量 %.2fmm 达到 %.2fmm 阈值，跳过灌溉",
			s.rainfallCheckWindowHours(), rainfall, threshold)
		log, err := s.insertLog(req, models.ExecutionStatusSkipped, &reason)
		if err != nil {
			if existing := s.recoverDuplicate(err, req.IdempotencyKey); existing != nil {
				return &StartIrrigationResult{Log: existing, Duplicate: true}, nil
			}
			return nil, err
		}
		return &StartIrrigationResult{Log: log, Skipped: true}, nil
	}

	// 4. 落进行中记录；区域级部分唯一索引保证手动与计划启动只有一个成功
	log, err := s.insertLog(req, models.ExecutionStatusInProgress, nil)
	if err != nil {
		if existing := s.recoverDuplicate(err, req.IdempotencyKey); existing != nil {
			return &StartIrrigationResult{Log: existing, Duplicate: true}, nil
		}
		if isUniqueViolation(err, "idx_irrigation_logs_zone_in_progress") {
			return nil, ErrZoneBusy
		}
		return nil, err
	}

	return &StartIrrigationResult{Log: log}, nil
}

func (s *IrrigationService) insertLog(req StartIrrigationRequest, status models.ExecutionStatus, message *string) (*models.IrrigationLog, error) {
	now := time.Now()
	zoneID := req.ZoneID
	log := &models.IrrigationLog{
		ScheduleID:  req.ScheduleID,
		ZoneID:      &zoneID,
		TriggerType: req.TriggerType,
		StartTime:   now,
		Status:      status,
	}
	if req.IdempotencyKey != "" {
		log.IdempotencyKey = &req.IdempotencyKey
	}
	if status == models.ExecutionStatusSkipped {
		log.EndTime = &now
		duration := 0
		log.Duration = &duration
		log.ErrorMessage = message
	}

	if err := database.DB.Create(log).Error; err != nil {
		return nil, err
	}
	return log, nil
}

// recoverDuplicate 处理并发下幂等键唯一索引冲突：重新查询并返回原执行记录
func (s *IrrigationService) recoverDuplicate(err error, idempotencyKey string) *models.IrrigationLog {
	if idempotencyKey == "" || !isUniqueViolation(err, "idx_irrigation_logs_idempotency_key") {
		return nil
	}
	existing, findErr := s.findByIdempotencyKey(idempotencyKey)
	if findErr != nil {
		return nil
	}
	return existing
}

func (s *IrrigationService) findByIdempotencyKey(key string) (*models.IrrigationLog, error) {
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

func (s *IrrigationService) getZone(zoneID uint) (*models.IrrigationZone, error) {
	var zone models.IrrigationZone
	err := database.DB.First(&zone, zoneID).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrZoneNotFound
	}
	if err != nil {
		return nil, err
	}
	return &zone, nil
}

// recentRainfall 汇总启动前检查窗口内的降雨量；区域无雨量传感器时返回 checked=false
func (s *IrrigationService) recentRainfall(zoneID uint, rainSensorID *uint) (rainfall float64, checked bool, err error) {
	sensorID := rainSensorID
	if sensorID == nil {
		sensorID, err = s.findZoneRainSensor(zoneID)
		if err != nil {
			return 0, false, err
		}
	}
	if sensorID == nil {
		return 0, false, nil
	}

	window := time.Duration(s.rainfallCheckWindowHours()) * time.Hour
	rainfall, err = s.sensorService.CheckRecentRainfall(*sensorID, window)
	if err != nil {
		return 0, false, err
	}
	return rainfall, true, nil
}

func (s *IrrigationService) findZoneRainSensor(zoneID uint) (*uint, error) {
	var device models.Device
	err := database.DB.Where("zone_id = ? AND type = ?", zoneID, models.DeviceTypeRainSensor).First(&device).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &device.ID, nil
}

type CompleteIrrigationRequest struct {
	Success  bool
	EndTime  *time.Time // 为空则取当前时间
	ErrorMsg *string
}

// CompleteIrrigation 只处理进行中记录：按结束时间计算实际时长和用水量，
// 原子条件更新保证重复完成不会再次累计；参数非法时不改动执行记录和告警。
func (s *IrrigationService) CompleteIrrigation(logID uint, req CompleteIrrigationRequest) (*models.IrrigationLog, error) {
	var log models.IrrigationLog
	err := database.DB.First(&log, logID).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrLogNotFound
	}
	if err != nil {
		return nil, err
	}

	// 参数校验先于一切写入，非法参数不得改动执行记录或告警
	now := time.Now()
	endTime := now
	if req.EndTime != nil {
		endTime = *req.EndTime
	}
	if endTime.Before(log.StartTime) {
		return nil, fmt.Errorf("%w: end_time %s is before start_time %s",
			ErrInvalidCompletion, endTime.Format(time.RFC3339), log.StartTime.Format(time.RFC3339))
	}
	if endTime.After(now.Add(time.Minute)) {
		return nil, fmt.Errorf("%w: end_time %s is in the future",
			ErrInvalidCompletion, endTime.Format(time.RFC3339))
	}

	duration := int(endTime.Sub(log.StartTime).Seconds())
	updates := map[string]interface{}{
		"end_time": endTime,
		"duration": duration,
	}
	if req.Success {
		updates["status"] = models.ExecutionStatusSuccess
		waterUsage := float64(duration) * waterUsagePerSecond
		updates["water_usage"] = waterUsage
	} else {
		updates["status"] = models.ExecutionStatusFailed
		if req.ErrorMsg != nil {
			updates["error_message"] = *req.ErrorMsg
		}
	}

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// 只处理进行中记录：条件更新，重复完成 RowsAffected=0 直接冲突返回
		result := tx.Model(&models.IrrigationLog{}).
			Where("id = ? AND status = ?", logID, models.ExecutionStatusInProgress).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotInProgress
		}

		if log.ZoneID == nil {
			return nil
		}
		if req.Success {
			// 成功完成：重置区域连续失败计数并闭合熔断
			return tx.Model(&models.IrrigationZone{}).
				Where("id = ?", *log.ZoneID).
				Updates(map[string]interface{}{
					"consecutive_failures": 0,
					"circuit_open_until":   nil,
				}).Error
		}

		// 失败完成：告警并累计连续失败，达到阈值打开熔断
		alert := &models.Alert{
			Type:    models.AlertTypeIrrigationFailed,
			Level:   models.AlertLevelCritical,
			Title:   "灌溉执行失败",
			Message: failureMessage(log.ID, req.ErrorMsg),
			Status:  models.AlertStatusNew,
		}
		if err := tx.Create(alert).Error; err != nil {
			return err
		}

		threshold := s.circuitBreakerThreshold()
		updateResult := tx.Model(&models.IrrigationZone{}).
			Where("id = ?", *log.ZoneID).
			Update("consecutive_failures", gorm.Expr("consecutive_failures + 1"))
		if updateResult.Error != nil {
			return updateResult.Error
		}

		var zone models.IrrigationZone
		if err := tx.Select("consecutive_failures").First(&zone, *log.ZoneID).Error; err != nil {
			return err
		}
		if zone.ConsecutiveFailures >= threshold {
			cooldown := time.Duration(s.circuitBreakerCooldownMinutes()) * time.Minute
			openUntil := now.Add(cooldown)
			if err := tx.Model(&models.IrrigationZone{}).
				Where("id = ?", *log.ZoneID).
				Update("circuit_open_until", openUntil).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := database.DB.First(&log, logID).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

func failureMessage(logID uint, errorMsg *string) string {
	if errorMsg != nil && *errorMsg != "" {
		return fmt.Sprintf("灌溉执行记录 %d 失败: %s", logID, *errorMsg)
	}
	return fmt.Sprintf("灌溉执行记录 %d 失败", logID)
}

type CircuitBreakerStatus struct {
	State               string     `json:"state"` // open / closed
	ConsecutiveFailures int        `json:"consecutive_failures"`
	OpenUntil           *time.Time `json:"open_until,omitempty"`
}

type ZoneExecutionStatus struct {
	CurrentExecution *models.IrrigationLog  `json:"current_execution"`
	CircuitBreaker   *CircuitBreakerStatus  `json:"circuit_breaker"`
	RecentLogs       []models.IrrigationLog `json:"recent_logs"`
}

// GetZoneExecutionStatus 返回区域当前执行、熔断状态和最近记录
func (s *IrrigationService) GetZoneExecutionStatus(zoneID uint, recentLimit int) (*ZoneExecutionStatus, error) {
	zone, err := s.getZone(zoneID)
	if err != nil {
		return nil, err
	}

	status := &ZoneExecutionStatus{
		CircuitBreaker: &CircuitBreakerStatus{
			State:               "closed",
			ConsecutiveFailures: zone.ConsecutiveFailures,
		},
		RecentLogs: []models.IrrigationLog{},
	}
	if zone.CircuitOpenUntil != nil && zone.CircuitOpenUntil.After(time.Now()) {
		status.CircuitBreaker.State = "open"
		status.CircuitBreaker.OpenUntil = zone.CircuitOpenUntil
	}

	var current models.IrrigationLog
	err = database.DB.Where("zone_id = ? AND status = ?", zoneID, models.ExecutionStatusInProgress).
		Order("start_time DESC").First(&current).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err == nil {
		status.CurrentExecution = &current
	}

	if recentLimit <= 0 {
		recentLimit = 10
	}
	if err := database.DB.Where("zone_id = ?", zoneID).
		Order("start_time DESC").Limit(recentLimit).Find(&status.RecentLogs).Error; err != nil {
		return nil, err
	}

	return status, nil
}

func (s *IrrigationService) rainfallSkipThreshold() float64 {
	return s.configFloat("rainfall_skip_threshold_mm", 5.0)
}

func (s *IrrigationService) rainfallCheckWindowHours() int {
	return s.configInt("rainfall_check_window_hours", 2)
}

func (s *IrrigationService) circuitBreakerThreshold() int {
	return s.configInt("circuit_breaker_threshold", 3)
}

func (s *IrrigationService) circuitBreakerCooldownMinutes() int {
	return s.configInt("circuit_breaker_cooldown_minutes", 30)
}

func (s *IrrigationService) configFloat(key string, defaultValue float64) float64 {
	value, err := strconv.ParseFloat(s.configService.GetValue(key, ""), 64)
	if err != nil {
		return defaultValue
	}
	return value
}

func (s *IrrigationService) configInt(key string, defaultValue int) int {
	value, err := strconv.Atoi(s.configService.GetValue(key, ""))
	if err != nil {
		return defaultValue
	}
	return value
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return constraint == "" || pgErr.ConstraintName == constraint
	}
	return false
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

	if err := query.Order("start_time DESC").Find(&logs).Error; err != nil {
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
