package scheduler

import (
	"errors"
	"strconv"
	"time"

	"go.uber.org/zap"

	"irrigation/internal/models"
	"irrigation/internal/services"
	"irrigation/pkg/logger"
)

type IrrigationScheduler struct {
	scheduleService   *services.ScheduleService
	irrigationService *services.IrrigationService
	sensorService     *services.SensorService
	deviceService     *services.DeviceService
	alertService      *services.AlertService
}

func NewIrrigationScheduler() *IrrigationScheduler {
	return &IrrigationScheduler{
		scheduleService:   services.NewScheduleService(),
		irrigationService: services.NewIrrigationService(),
		sensorService:     services.NewSensorService(),
		deviceService:     services.NewDeviceService(),
		alertService:      services.NewAlertService(),
	}
}

func (s *IrrigationScheduler) Start() {
	logger.Info("Starting irrigation scheduler started")

	go s.runScheduleCheck()
	go s.runDeviceHealthCheck()
}

func (s *IrrigationScheduler) runScheduleCheck() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.checkAndExecuteSchedules()
	}
}

func (s *IrrigationScheduler) checkAndExecuteSchedules() {
	schedules, err := s.scheduleService.ListActiveSchedules()
	if err != nil {
		logger.Error("Failed to get active schedules", zap.Error(err))
		return
	}

	for _, schedule := range schedules {
		s.executeScheduleIfNeeded(schedule)
	}
}

func (s *IrrigationScheduler) executeScheduleIfNeeded(schedule models.IrrigationSchedule) {
	now := time.Now()

	if schedule.Type == models.ScheduleTypeTimed {
		if shouldExecuteTimedSchedule(schedule, now) {
			go s.executeIrrigation(schedule)
		}
	} else if schedule.Type == models.ScheduleTypeConditional {
		if shouldExecuteConditionalSchedule(schedule) {
			go s.executeIrrigation(schedule)
		}
	}
}

func shouldExecuteTimedSchedule(schedule models.IrrigationSchedule, now time.Time) bool {
	if schedule.StartTime == "" {
		return false
	}

	// 数据库 TIME 类型可能带秒（"02:19:00"），统一截断到分钟比较
	startTime := schedule.StartTime
	if len(startTime) > 5 {
		startTime = startTime[:5]
	}

	nowTime := now.Format("15:04")
	if startTime == nowTime {
		switch schedule.RepeatMode {
		case models.RepeatModeOnce:
			return true
		case models.RepeatModeDaily:
			return true
		case models.RepeatModeWeekly:
			weekday := int(now.Weekday())
			for _, d := range schedule.RepeatDays {
				if d == weekday {
					return true
				}
			}
		case models.RepeatModeMonthly:
			if len(schedule.RepeatDays) > 0 {
				day := now.Day()
				for _, d := range schedule.RepeatDays {
					if d == day {
						return true
					}
				}
			}
		}
	}
	return false
}

func shouldExecuteConditionalSchedule(schedule models.IrrigationSchedule) bool {
	if schedule.ZoneID == nil || schedule.HumidityThreshold == nil {
		return false
	}

	avgHumidity, err := services.NewSensorService().GetAverageHumidity(*schedule.ZoneID, 1*time.Hour)
	if err != nil || avgHumidity == nil {
		return false
	}

	return *avgHumidity < *schedule.HumidityThreshold
}

func (s *IrrigationScheduler) executeIrrigation(schedule models.IrrigationSchedule) {
	logger.Info("Executing irrigation schedule", zap.Uint("schedule_id", schedule.ID))

	var triggerType models.TriggerType
	if schedule.Type == models.ScheduleTypeTimed {
		triggerType = models.TriggerTypeTimed
	} else {
		triggerType = models.TriggerTypeConditional
	}

	zoneID := uint(0)
	if schedule.ZoneID != nil {
		zoneID = *schedule.ZoneID
	}

	// 幂等键按计划+分钟生成，防止调度周期内重复触发；
	// 雨量检查、区域互斥、熔断由灌溉服务统一处理
	result, err := s.irrigationService.StartIrrigation(services.StartIrrigationRequest{
		ZoneID:         zoneID,
		ScheduleID:     &schedule.ID,
		TriggerType:    triggerType,
		IdempotencyKey: "schedule-" + strconv.FormatUint(uint64(schedule.ID), 10) + "-" + time.Now().Format("200601021504"),
		RainSensorID:   schedule.RainSensorID,
	})
	if err != nil {
		switch {
		case errors.Is(err, services.ErrZoneBusy):
			logger.Info("Skipping schedule, zone busy", zap.Uint("schedule_id", schedule.ID))
		case errors.Is(err, services.ErrCircuitOpen):
			logger.Warn("Skipping schedule, circuit breaker open", zap.Uint("schedule_id", schedule.ID))
		case errors.Is(err, services.ErrZoneNotFound):
			logger.Error("Schedule zone not found", zap.Uint("schedule_id", schedule.ID))
		default:
			logger.Error("Failed to start irrigation", zap.Error(err))
			s.alertService.CreateIrrigationFailedAlert(schedule.ZoneID, "启动灌溉失败: "+err.Error())
		}
		return
	}

	if result.Duplicate {
		logger.Info("Schedule already triggered in this window", zap.Uint("schedule_id", schedule.ID))
		return
	}
	if result.Skipped {
		logger.Info("Irrigation skipped due to recent rainfall",
			zap.Uint("schedule_id", schedule.ID),
			zap.String("reason", *result.Log.ErrorMessage))
		return
	}

	log := result.Log
	duration := time.Duration(schedule.Duration) * time.Second
	if schedule.Duration > 0 {
		time.Sleep(duration)

		if _, err := s.irrigationService.CompleteIrrigation(log.ID, services.CompleteIrrigationRequest{Success: true}); err != nil {
			logger.Error("Failed to complete irrigation", zap.Uint("log_id", log.ID), zap.Error(err))
			return
		}
		logger.Info("Irrigation completed", zap.Uint("log_id", log.ID))
	} else {
		errMsg := "schedule duration is not positive"
		if _, err := s.irrigationService.CompleteIrrigation(log.ID, services.CompleteIrrigationRequest{Success: false, ErrorMsg: &errMsg}); err != nil {
			logger.Error("Failed to complete irrigation", zap.Uint("log_id", log.ID), zap.Error(err))
		}
	}
}

func (s *IrrigationScheduler) runDeviceHealthCheck() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.checkDeviceHealth()
	}
}

func (s *IrrigationScheduler) checkDeviceHealth() {
	timeout := 5 * time.Minute
	devices, err := s.deviceService.CheckOfflineDevices(timeout)
	if err != nil {
		logger.Error("Failed to check offline devices", zap.Error(err))
		return
	}

	for _, device := range devices {
		if device.Status == models.DeviceStatusOnline {
			s.deviceService.MarkDeviceOffline(device.ID)
			s.alertService.CreateDeviceOfflineAlert(device.ID, device.Name)
			logger.Warn("Device marked as offline", zap.Uint("device_id", device.ID), zap.String("device_name", device.Name))
		}
	}
}
