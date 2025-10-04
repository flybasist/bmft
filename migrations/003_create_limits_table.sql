-- Migration: 003_create_limits_table
-- Description: Создание таблицы для хранения лимитов пользователей
-- Date: 2025-10-04

-- Таблица лимитов пользователей
CREATE TABLE IF NOT EXISTS user_limits (
    user_id BIGINT PRIMARY KEY,
    username VARCHAR(255),
    
    -- Лимиты
    daily_limit INT NOT NULL DEFAULT 10,
    monthly_limit INT NOT NULL DEFAULT 300,
    
    -- Использование
    daily_used INT NOT NULL DEFAULT 0,
    monthly_used INT NOT NULL DEFAULT 0,
    
    -- Время последнего сброса
    last_reset_daily TIMESTAMP NOT NULL DEFAULT NOW(),
    last_reset_monthly TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- Метаданные
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Индексы для оптимизации запросов сброса
CREATE INDEX IF NOT EXISTS idx_user_limits_daily_reset 
    ON user_limits(last_reset_daily);

CREATE INDEX IF NOT EXISTS idx_user_limits_monthly_reset 
    ON user_limits(last_reset_monthly);

-- Комментарии к таблице
COMMENT ON TABLE user_limits IS 'Лимиты пользователей на запросы к AI';
COMMENT ON COLUMN user_limits.user_id IS 'Telegram User ID';
COMMENT ON COLUMN user_limits.username IS 'Telegram username для логирования';
COMMENT ON COLUMN user_limits.daily_limit IS 'Максимальное количество запросов в день';
COMMENT ON COLUMN user_limits.monthly_limit IS 'Максимальное количество запросов в месяц';
COMMENT ON COLUMN user_limits.daily_used IS 'Использовано запросов сегодня';
COMMENT ON COLUMN user_limits.monthly_used IS 'Использовано запросов в этом месяце';
COMMENT ON COLUMN user_limits.last_reset_daily IS 'Время последнего дневного сброса';
COMMENT ON COLUMN user_limits.last_reset_monthly IS 'Время последнего месячного сброса';
