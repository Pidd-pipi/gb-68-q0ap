-- 智能灌溉管理系统数据库初始化脚本

-- 灌溉区域表
CREATE TABLE IF NOT EXISTS irrigation_zones (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- 设备类型枚举
CREATE TYPE device_type AS ENUM ('valve', 'pump', 'soil_sensor', 'rain_sensor', 'temp_sensor');
CREATE TYPE device_status AS ENUM ('online', 'offline', 'error');

-- 设备表
CREATE TABLE IF NOT EXISTS devices (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    type device_type NOT NULL,
    serial_number VARCHAR(100) UNIQUE NOT NULL,
    zone_id INTEGER REFERENCES irrigation_zones(id),
    status device_status DEFAULT 'offline',
    last_heartbeat TIMESTAMP,
    config JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- 传感器数据表（时序表）
CREATE TABLE IF NOT EXISTS sensor_data (
    id BIGSERIAL,
    device_id INTEGER NOT NULL REFERENCES devices(id),
    data_type VARCHAR(50) NOT NULL,
    value DECIMAL(10, 2) NOT NULL,
    unit VARCHAR(20),
    timestamp TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id, timestamp)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_sensor_data_device_time ON sensor_data(device_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_sensor_data_time ON sensor_data(timestamp);

-- 灌溉计划类型枚举
CREATE TYPE schedule_type AS ENUM ('timed', 'conditional');
CREATE TYPE schedule_status AS ENUM ('active', 'inactive');
CREATE TYPE repeat_mode AS ENUM ('once', 'daily', 'weekly', 'monthly');

-- 灌溉计划表
CREATE TABLE IF NOT EXISTS irrigation_schedules (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    type schedule_type NOT NULL,
    zone_id INTEGER REFERENCES irrigation_zones(id),
    status schedule_status DEFAULT 'inactive',
    start_time TIME,
    duration INTEGER,
    repeat_mode repeat_mode DEFAULT 'once',
    repeat_days INTEGER[],
    humidity_threshold DECIMAL(5, 2),
    rain_sensor_id INTEGER REFERENCES devices(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- 触发方式枚举
CREATE TYPE trigger_type AS ENUM ('manual', 'timed', 'conditional');
CREATE TYPE execution_status AS ENUM ('success', 'failed', 'in_progress', 'skipped');

-- 灌溉执行记录表
CREATE TABLE IF NOT EXISTS irrigation_logs (
    id BIGSERIAL PRIMARY KEY,
    schedule_id INTEGER REFERENCES irrigation_schedules(id),
    zone_id INTEGER REFERENCES irrigation_zones(id),
    trigger_type trigger_type NOT NULL,
    idempotency_key VARCHAR(100),
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP,
    duration INTEGER,
    water_usage DECIMAL(10, 2),
    status execution_status NOT NULL,
    error_message TEXT,
    skip_reason TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_irrigation_logs_zone_time ON irrigation_logs(zone_id, start_time);
CREATE INDEX IF NOT EXISTS idx_irrigation_logs_time ON irrigation_logs(start_time);
-- 幂等键唯一：同一键重复调用返回原执行记录
CREATE UNIQUE INDEX IF NOT EXISTS idx_irrigation_logs_idempotency_key ON irrigation_logs(idempotency_key) WHERE idempotency_key IS NOT NULL;
-- 同一区域同一时间只允许一条进行中的执行记录（并发安全）
CREATE UNIQUE INDEX IF NOT EXISTS idx_irrigation_logs_zone_in_progress ON irrigation_logs(zone_id) WHERE status = 'in_progress';

-- 告警类型枚举
CREATE TYPE alert_type AS ENUM ('device_offline', 'sensor_abnormal', 'irrigation_failed');
CREATE TYPE alert_level AS ENUM ('info', 'warning', 'critical');
CREATE TYPE alert_status AS ENUM ('new', 'acknowledged', 'resolved');

-- 告警表
CREATE TABLE IF NOT EXISTS alerts (
    id BIGSERIAL PRIMARY KEY,
    type alert_type NOT NULL,
    level alert_level NOT NULL,
    title VARCHAR(200) NOT NULL,
    message TEXT,
    device_id INTEGER REFERENCES devices(id),
    status alert_status DEFAULT 'new',
    acknowledged_at TIMESTAMP,
    resolved_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    email VARCHAR(100) UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 系统配置表
CREATE TABLE IF NOT EXISTS system_configs (
    id SERIAL PRIMARY KEY,
    key VARCHAR(100) UNIQUE NOT NULL,
    value TEXT,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 插入默认管理员用户 (密码: admin123)
INSERT INTO users (username, password_hash, email)
VALUES ('admin', '$2a$10$NgUq3xV947g6DDcvoYZ6R..ei9LpOJuMxeL6bgRwnK/qz4i/ZTq0i', 'admin@example.com')
ON CONFLICT (username) DO NOTHING;

-- 插入默认系统配置
INSERT INTO system_configs (key, value, description) VALUES 
('water_saving_mode', 'false', '节水模式'),
('preferred_irrigation_start', '06:00', '偏好灌溉开始时间'),
('preferred_irrigation_end', '08:00', '偏好灌溉结束时间'),
('device_heartbeat_timeout', '300', '设备心跳超时时间（秒）')
ON CONFLICT (key) DO NOTHING;
