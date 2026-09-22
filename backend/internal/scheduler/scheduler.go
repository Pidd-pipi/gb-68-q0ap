package scheduler

import (
	"errors"
	"fmt"
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

	// 数据库 TIME 类型可能返回 "15:04:05"，统一归一化到 "15:04" 再比较
	startTime := normalizeClock(schedule.StartTime)
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

func normalizeClock(s string) string {
	for _, layout := range []string{"15:04:05", "15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("15:04")
		}
	}
	return s
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

	// 幂等键精确到分钟：调度器每分钟巡检，同一分钟内重复触发返回原记录
	idempotencyKey := fmt.Sprintf("schedule-%d-%s", schedule.ID, time.Now().Format("20060102T1504"))

	// 降雨保护在执行服务内统一处理：达到阈值只落跳过记录，不启动阀门
	log, deduplicated, err := s.irrigationService.StartIrrigation(&schedule.ID, schedule.ZoneID, triggerType, idempotencyKey)
	if err != nil {
		if errors.Is(err, services.ErrZoneBusy) || errors.Is(err, services.ErrBreakerOpen) {
			logger.Info("Irrigation start rejected",
				zap.Uint("schedule_id", schedule.ID),
				zap.Error(err),
			)
			return
		}
		logger.Error("Failed to start irrigation", zap.Error(err))
		s.alertService.CreateIrrigationFailedAlert(schedule.ZoneID, "启动灌溉失败: "+err.Error())
		return
	}

	if deduplicated {
		logger.Info("Irrigation already started for this key",
			zap.Uint("schedule_id", schedule.ID),
			zap.Uint("log_id", log.ID),
		)
		return
	}

	if log.Status == models.ExecutionStatusSkipped {
		logger.Info("Irrigation skipped",
			zap.Uint("schedule_id", schedule.ID),
			zap.Uint("log_id", log.ID),
		)
		return
	}

	if schedule.Duration > 0 {
		time.Sleep(time.Duration(schedule.Duration) * time.Second)

		if _, err := s.irrigationService.CompleteIrrigation(log.ID, true, time.Now(), nil); err != nil {
			logger.Error("Failed to complete irrigation", zap.Uint("log_id", log.ID), zap.Error(err))
			return
		}
		logger.Info("Irrigation completed", zap.Uint("log_id", log.ID))
	} else {
		errMsg := "灌溉计划时长配置无效"
		if _, err := s.irrigationService.CompleteIrrigation(log.ID, false, time.Now(), &errMsg); err != nil {
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
