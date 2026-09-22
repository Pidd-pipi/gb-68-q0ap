package models

import (
	"time"

	"gorm.io/gorm"
)

type DeviceType string

const (
	DeviceTypeValve      DeviceType = "valve"
	DeviceTypePump       DeviceType = "pump"
	DeviceTypeSoilSensor DeviceType = "soil_sensor"
	DeviceTypeRainSensor DeviceType = "rain_sensor"
	DeviceTypeTempSensor DeviceType = "temp_sensor"
)

type DeviceStatus string

const (
	DeviceStatusOnline  DeviceStatus = "online"
	DeviceStatusOffline DeviceStatus = "offline"
	DeviceStatusError   DeviceStatus = "error"
)

type IrrigationZone struct {
	ID                  uint           `json:"id" gorm:"primaryKey"`
	Name                string         `json:"name" gorm:"size:100;not null"`
	Description         string         `json:"description" gorm:"type:text"`
	ConsecutiveFailures int            `json:"consecutive_failures" gorm:"default:0"`
	CircuitOpenUntil    *time.Time     `json:"circuit_open_until"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `json:"-" gorm:"index"`
	Devices             []Device       `json:"devices,omitempty" gorm:"foreignKey:ZoneID"`
}

type Device struct {
	ID            uint                   `json:"id" gorm:"primaryKey"`
	Name          string                 `json:"name" gorm:"size:100;not null"`
	Type          DeviceType             `json:"type" gorm:"type:device_type;not null"`
	SerialNumber  string                 `json:"serial_number" gorm:"size:100;uniqueIndex;not null"`
	ZoneID        *uint                  `json:"zone_id"`
	Status        DeviceStatus           `json:"status" gorm:"type:device_status;default:'offline'"`
	LastHeartbeat *time.Time             `json:"last_heartbeat"`
	Config        map[string]interface{} `json:"config" gorm:"type:jsonb"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	DeletedAt     gorm.DeletedAt         `json:"-" gorm:"index"`
}

type SensorData struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	DeviceID  uint      `json:"device_id" gorm:"not null"`
	DataType  string    `json:"data_type" gorm:"size:50;not null"`
	Value     float64   `json:"value" gorm:"type:decimal(10,2);not null"`
	Unit      string    `json:"unit" gorm:"size:20"`
	Timestamp time.Time `json:"timestamp" gorm:"not null"`
	CreatedAt time.Time `json:"created_at"`
}

type ScheduleType string

const (
	ScheduleTypeTimed       ScheduleType = "timed"
	ScheduleTypeConditional ScheduleType = "conditional"
)

type ScheduleStatus string

const (
	ScheduleStatusActive   ScheduleStatus = "active"
	ScheduleStatusInactive ScheduleStatus = "inactive"
)

type RepeatMode string

const (
	RepeatModeOnce    RepeatMode = "once"
	RepeatModeDaily   RepeatMode = "daily"
	RepeatModeWeekly  RepeatMode = "weekly"
	RepeatModeMonthly RepeatMode = "monthly"
)

type IrrigationSchedule struct {
	ID                uint           `json:"id" gorm:"primaryKey"`
	Name              string         `json:"name" gorm:"size:100;not null"`
	Type              ScheduleType   `json:"type" gorm:"type:schedule_type;not null"`
	ZoneID            *uint          `json:"zone_id"`
	Status            ScheduleStatus `json:"status" gorm:"type:schedule_status;default:'inactive'"`
	StartTime         string         `json:"start_time" gorm:"type:time"`
	Duration          int            `json:"duration"`
	RepeatMode        RepeatMode     `json:"repeat_mode" gorm:"type:repeat_mode;default:'once'"`
	RepeatDays        []int          `json:"repeat_days" gorm:"type:integer[]"`
	HumidityThreshold *float64       `json:"humidity_threshold" gorm:"type:decimal(5,2)"`
	RainSensorID      *uint          `json:"rain_sensor_id"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"index"`
}

type TriggerType string

const (
	TriggerTypeManual      TriggerType = "manual"
	TriggerTypeTimed       TriggerType = "timed"
	TriggerTypeConditional TriggerType = "conditional"
)

type ExecutionStatus string

const (
	ExecutionStatusSuccess    ExecutionStatus = "success"
	ExecutionStatusFailed     ExecutionStatus = "failed"
	ExecutionStatusInProgress ExecutionStatus = "in_progress"
	ExecutionStatusSkipped    ExecutionStatus = "skipped"
)

type IrrigationLog struct {
	ID             uint            `json:"id" gorm:"primaryKey"`
	ScheduleID     *uint           `json:"schedule_id"`
	ZoneID         *uint           `json:"zone_id"`
	TriggerType    TriggerType     `json:"trigger_type" gorm:"type:trigger_type;not null"`
	StartTime      time.Time       `json:"start_time" gorm:"not null"`
	EndTime        *time.Time      `json:"end_time"`
	Duration       *int            `json:"duration"`
	WaterUsage     *float64        `json:"water_usage" gorm:"type:decimal(10,2)"`
	Status         ExecutionStatus `json:"status" gorm:"type:execution_status;not null"`
	ErrorMessage   *string         `json:"error_message" gorm:"type:text"`
	IdempotencyKey *string         `json:"idempotency_key" gorm:"size:100"`
	CreatedAt      time.Time       `json:"created_at"`
}

type AlertType string

const (
	AlertTypeDeviceOffline    AlertType = "device_offline"
	AlertTypeSensorAbnormal   AlertType = "sensor_abnormal"
	AlertTypeIrrigationFailed AlertType = "irrigation_failed"
)

type AlertLevel string

const (
	AlertLevelInfo     AlertLevel = "info"
	AlertLevelWarning  AlertLevel = "warning"
	AlertLevelCritical AlertLevel = "critical"
)

type AlertStatus string

const (
	AlertStatusNew          AlertStatus = "new"
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	AlertStatusResolved     AlertStatus = "resolved"
)

type Alert struct {
	ID             uint        `json:"id" gorm:"primaryKey"`
	Type           AlertType   `json:"type" gorm:"type:alert_type;not null"`
	Level          AlertLevel  `json:"level" gorm:"type:alert_level;not null"`
	Title          string      `json:"title" gorm:"size:200;not null"`
	Message        string      `json:"message" gorm:"type:text"`
	DeviceID       *uint       `json:"device_id"`
	Status         AlertStatus `json:"status" gorm:"type:alert_status;default:'new'"`
	AcknowledgedAt *time.Time  `json:"acknowledged_at"`
	ResolvedAt     *time.Time  `json:"resolved_at"`
	CreatedAt      time.Time   `json:"created_at"`
}

type User struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Username     string    `json:"username" gorm:"size:50;uniqueIndex;not null"`
	PasswordHash string    `json:"-" gorm:"size:255;not null"`
	Email        string    `json:"email" gorm:"size:100;uniqueIndex"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type SystemConfig struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Key         string    `json:"key" gorm:"size:100;uniqueIndex;not null"`
	Value       string    `json:"value" gorm:"type:text"`
	Description string    `json:"description" gorm:"type:text"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
