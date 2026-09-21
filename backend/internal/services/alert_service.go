package services

import (
	"errors"
	"time"

	"irrigation/internal/models"
	"irrigation/pkg/database"
)

type AlertService struct{}

func NewAlertService() *AlertService {
	return &AlertService{}
}

func (s *AlertService) CreateAlert(alert *models.Alert) error {
	return database.DB.Create(alert).Error
}

func (s *AlertService) CreateDeviceOfflineAlert(deviceID uint, deviceName string) error {
	alert := &models.Alert{
		Type:     models.AlertTypeDeviceOffline,
		Level:    models.AlertLevelWarning,
		Title:    "设备离线告警",
		Message:  "设备 " + deviceName + " 已离线",
		DeviceID: &deviceID,
		Status:   models.AlertStatusNew,
	}
	return s.CreateAlert(alert)
}

func (s *AlertService) CreateSensorAbnormalAlert(deviceID uint, deviceName string, msg string) error {
	alert := &models.Alert{
		Type:     models.AlertTypeSensorAbnormal,
		Level:    models.AlertLevelWarning,
		Title:    "传感器异常告警",
		Message:  "设备 " + deviceName + ": " + msg,
		DeviceID: &deviceID,
		Status:   models.AlertStatusNew,
	}
	return s.CreateAlert(alert)
}

func (s *AlertService) CreateIrrigationFailedAlert(zoneID *uint, msg string) error {
	alert := &models.Alert{
		Type:    models.AlertTypeIrrigationFailed,
		Level:   models.AlertLevelCritical,
		Title:   "灌溉执行失败",
		Message: msg,
		Status:  models.AlertStatusNew,
	}
	return s.CreateAlert(alert)
}

func (s *AlertService) GetAlertByID(id uint) (*models.Alert, error) {
	var alert models.Alert
	if err := database.DB.First(&alert, id).Error; err != nil {
		return nil, err
	}
	return &alert, nil
}

func (s *AlertService) ListAlerts(status *string, level *string, limit int) ([]models.Alert, error) {
	var alerts []models.Alert
	query := database.DB

	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if level != nil {
		query = query.Where("level = ?", *level)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Order("created_at DESC").Find(&alerts).Error; err != nil {
		return nil, err
	}
	return alerts, nil
}

func (s *AlertService) AcknowledgeAlert(id uint) error {
	now := time.Now()
	result := database.DB.Model(&models.Alert{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          models.AlertStatusAcknowledged,
			"acknowledged_at": now,
		})
	if result.RowsAffected == 0 {
		return errors.New("alert not found")
	}
	return result.Error
}

func (s *AlertService) ResolveAlert(id uint) error {
	now := time.Now()
	result := database.DB.Model(&models.Alert{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     models.AlertStatusResolved,
			"resolved_at": now,
		})
	if result.RowsAffected == 0 {
		return errors.New("alert not found")
	}
	return result.Error
}

func (s *AlertService) GetUnacknowledgedCount() (int64, error) {
	var count int64
	err := database.DB.Model(&models.Alert{}).
		Where("status = ?", models.AlertStatusNew).
		Count(&count).Error
	return count, err
}
