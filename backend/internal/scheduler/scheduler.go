package scheduler

import (
	"time"

	"go.uber.org/zap"

	"irrigation/internal/models"
	"irrigation/internal/services"
	"irrigation/pkg/logger"
)

type IrrigationScheduler struct {
	scheduleService  *services.ScheduleService
	irrigationService *services.IrrigationService
	sensorService   *services.SensorService
	deviceService  *services.DeviceService
	alertService   *services.AlertService
}

func NewIrrigationScheduler() *IrrigationScheduler {
	return &IrrigationScheduler{
		scheduleService:  services.NewScheduleService(),
		irrigationService: services.NewIrrigationService(),
		sensorService:   services.NewSensorService(),
		deviceService:  services.NewDeviceService(),
		alertService:   services.NewAlertService(),
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

	nowTime := now.Format("15:04")
	if schedule.StartTime == nowTime {
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

	if schedule.RainSensorID != nil {
		rainfall, err := s.sensorService.CheckRecentRainfall(*schedule.RainSensorID, 2*time.Hour)
		if err == nil && rainfall > 5.0 {
			logger.Info("Skipping irrigation due to recent rainfall", zap.Float64("rainfall", rainfall))
			return
		}
	}

	var triggerType models.TriggerType
	if schedule.Type == models.ScheduleTypeTimed {
		triggerType = models.TriggerTypeTimed
	} else {
		triggerType = models.TriggerTypeConditional
	}

	log, err := s.irrigationService.StartIrrigation(&schedule.ID, schedule.ZoneID, triggerType)
	if err != nil {
		logger.Error("Failed to start irrigation", zap.Error(err))
		s.alertService.CreateIrrigationFailedAlert(schedule.ZoneID, "启动灌溉失败: "+err.Error())
		return
	}

	duration := time.Duration(schedule.Duration) * time.Second
	if schedule.Duration > 0 {
		time.Sleep(duration)

		waterUsage := float64(schedule.Duration) * 0.1
		s.irrigationService.CompleteIrrigation(log.ID, true, &waterUsage, nil)
		logger.Info("Irrigation completed", zap.Uint("log_id", log.ID))
	} else {
		s.irrigationService.CompleteIrrigation(log.ID, false, nil, nil)
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
