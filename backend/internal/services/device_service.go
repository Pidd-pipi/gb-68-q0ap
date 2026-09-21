package services

import (
	"errors"
	"time"

	"irrigation/internal/models"
	"irrigation/pkg/database"
)

type DeviceService struct{}

func NewDeviceService() *DeviceService {
	return &DeviceService{}
}

func (s *DeviceService) CreateDevice(device *models.Device) error {
	return database.DB.Create(device).Error
}

func (s *DeviceService) GetDeviceByID(id uint) (*models.Device, error) {
	var device models.Device
	if err := database.DB.First(&device, id).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (s *DeviceService) GetDeviceBySerial(serial string) (*models.Device, error) {
	var device models.Device
	if err := database.DB.Where("serial_number = ?", serial).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (s *DeviceService) ListDevices(zoneID *uint, deviceType *string, status *string) ([]models.Device, error) {
	var devices []models.Device
	query := database.DB

	if zoneID != nil {
		query = query.Where("zone_id = ?", *zoneID)
	}
	if deviceType != nil {
		query = query.Where("type = ?", *deviceType)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

func (s *DeviceService) UpdateDevice(id uint, updates map[string]interface{}) error {
	result := database.DB.Model(&models.Device{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("device not found")
	}
	return nil
}

func (s *DeviceService) DeleteDevice(id uint) error {
	result := database.DB.Delete(&models.Device{}, id)
	if result.RowsAffected == 0 {
		return errors.New("device not found")
	}
	return result.Error
}

func (s *DeviceService) UpdateHeartbeat(serial string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"last_heartbeat": now,
		"status":         models.DeviceStatusOnline,
	}
	return database.DB.Model(&models.Device{}).
		Where("serial_number = ?", serial).
		Updates(updates).Error
}

func (s *DeviceService) CheckOfflineDevices(timeout time.Duration) ([]models.Device, error) {
	cutoffTime := time.Now().Add(-timeout)
	var devices []models.Device
	
	err := database.DB.
		Where("status = ? OR last_heartbeat IS NULL OR last_heartbeat < ?",
			models.DeviceStatusOnline, cutoffTime).
		Find(&devices).Error
	
	return devices, err
}

func (s *DeviceService) MarkDeviceOffline(id uint) error {
	return database.DB.Model(&models.Device{}).
		Where("id = ?", id).
		Update("status", models.DeviceStatusOffline).Error
}
