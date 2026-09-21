package services

import (
	"errors"

	"irrigation/internal/models"
	"irrigation/pkg/database"
)

type ZoneService struct{}

func NewZoneService() *ZoneService {
	return &ZoneService{}
}

func (s *ZoneService) CreateZone(zone *models.IrrigationZone) error {
	return database.DB.Create(zone).Error
}

func (s *ZoneService) GetZoneByID(id uint) (*models.IrrigationZone, error) {
	var zone models.IrrigationZone
	if err := database.DB.Preload("Devices").First(&zone, id).Error; err != nil {
		return nil, err
	}
	return &zone, nil
}

func (s *ZoneService) ListZones() ([]models.IrrigationZone, error) {
	var zones []models.IrrigationZone
	if err := database.DB.Preload("Devices").Find(&zones).Error; err != nil {
		return nil, err
	}
	return zones, nil
}

func (s *ZoneService) UpdateZone(id uint, updates map[string]interface{}) error {
	result := database.DB.Model(&models.IrrigationZone{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("zone not found")
	}
	return nil
}

func (s *ZoneService) DeleteZone(id uint) error {
	result := database.DB.Delete(&models.IrrigationZone{}, id)
	if result.RowsAffected == 0 {
		return errors.New("zone not found")
	}
	return result.Error
}
