package services

import (
	"irrigation/internal/models"
	"irrigation/pkg/database"
)

type ConfigService struct{}

func NewConfigService() *ConfigService {
	return &ConfigService{}
}

func (s *ConfigService) GetConfig(key string) (*models.SystemConfig, error) {
	var config models.SystemConfig
	if err := database.DB.Where("key = ?", key).First(&config).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *ConfigService) GetAllConfigs() ([]models.SystemConfig, error) {
	var configs []models.SystemConfig
	if err := database.DB.Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}

func (s *ConfigService) SetConfig(key, value, description string) error {
	var config models.SystemConfig
	err := database.DB.Where("key = ?", key).First(&config).Error
	
	if err != nil {
		config = models.SystemConfig{
			Key:         key,
			Value:       value,
			Description: description,
		}
		return database.DB.Create(&config).Error
	}

	config.Value = value
	if description != "" {
		config.Description = description
	}
	return database.DB.Save(&config).Error
}

func (s *ConfigService) GetValue(key, defaultValue string) string {
	config, err := s.GetConfig(key)
	if err != nil {
		return defaultValue
	}
	return config.Value
}

func (s *ConfigService) DeleteConfig(key string) error {
	return database.DB.Where("key = ?", key).Delete(&models.SystemConfig{}).Error
}
