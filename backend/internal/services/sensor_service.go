package services

import (
	"time"

	"irrigation/internal/models"
	"irrigation/pkg/database"
	"gorm.io/gorm"
)

type SensorService struct{}

func NewSensorService() *SensorService {
	return &SensorService{}
}

func (s *SensorService) CreateSensorData(data *models.SensorData) error {
	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}
	return database.DB.Create(data).Error
}

func (s *SensorService) BatchCreateSensorData(dataList []models.SensorData) error {
	if len(dataList) == 0 {
		return nil
	}
	now := time.Now()
	for i := range dataList {
		if dataList[i].Timestamp.IsZero() {
			dataList[i].Timestamp = now
		}
	}
	return database.DB.Create(&dataList).Error
}

func (s *SensorService) GetSensorHistory(deviceID uint, startTime, endTime time.Time, limit int) ([]models.SensorData, error) {
	var data []models.SensorData
	query := database.DB.Where("device_id = ?", deviceID)

	if !startTime.IsZero() {
		query = query.Where("timestamp >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("timestamp <= ?", endTime)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Order("timestamp DESC").Find(&data).Error; err != nil {
		return nil, err
	}
	return data, nil
}

func (s *SensorService) GetLatestSensorData(deviceID uint, dataType string) (*models.SensorData, error) {
	var data models.SensorData
	query := database.DB.Where("device_id = ?", deviceID)
	if dataType != "" {
		query = query.Where("data_type = ?", dataType)
	}
	err := query.Order("timestamp DESC").First(&data).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &data, err
}

func (s *SensorService) GetAverageHumidity(zoneID uint, duration time.Duration) (*float64, error) {
	var avg *float64
	subQuery := database.DB.Model(&models.Device{}).
		Select("id").
		Where("zone_id = ? AND type = ?", zoneID, models.DeviceTypeSoilSensor)

	err := database.DB.Model(&models.SensorData{}).
		Select("AVG(value)").
		Where("device_id IN (?) AND timestamp >= ?", subQuery, time.Now().Add(-duration)).
		Scan(&avg).Error

	return avg, err
}

func (s *SensorService) CheckRecentRainfall(sensorID uint, duration time.Duration) (float64, error) {
	var total float64
	err := database.DB.Model(&models.SensorData{}).
		Select("COALESCE(SUM(value), 0)").
		Where("device_id = ? AND data_type = ? AND timestamp >= ?",
			sensorID, "rainfall", time.Now().Add(-duration)).
		Scan(&total).Error
	return total, err
}
