-- Миграция 001: Базовая оптимизированная схема для модульного бота
-- Дата: 2025-10-04
-- Автор: FlyBasist

-- ============================================================
-- CORE TABLES (общие для всех модулей)
-- ============================================================

-- Таблица чатов (метаинформация)
CREATE TABLE IF NOT EXISTS chats (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT UNIQUE NOT NULL,
    chat_type VARCHAR(20) NOT NULL, -- 'private', 'group', 'supergroup', 'channel'
    title TEXT,
    username TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_chats_chat_id ON chats(chat_id);
CREATE INDEX idx_chats_active ON chats(is_active, chat_id);

-- Таблица пользователей (кэш информации о пользователях)
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT UNIQUE NOT NULL,
    username TEXT,
    first_name TEXT,
    last_name TEXT,
    is_bot BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_users_user_id ON users(user_id);

-- Таблица администраторов чатов (для управления модулями и настройками)
CREATE TABLE IF NOT EXISTS chat_admins (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    can_manage_modules BOOLEAN DEFAULT true,   -- может включать/выключать модули
    can_manage_limits BOOLEAN DEFAULT true,    -- может настраивать лимиты
    can_manage_reactions BOOLEAN DEFAULT true, -- может настраивать реакции
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, user_id)
);

CREATE INDEX idx_chat_admins_chat ON chat_admins(chat_id);

-- ============================================================
-- MODULE CONFIGURATION (включение/выключение модулей per chat)
-- ============================================================

CREATE TABLE IF NOT EXISTS chat_modules (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    module_name VARCHAR(50) NOT NULL, -- 'limiter', 'reactions', 'antispam', 'statistics', etc.
    is_enabled BOOLEAN DEFAULT false,
    config JSONB DEFAULT '{}', -- специфичная конфигурация модуля
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, module_name)
);

CREATE INDEX idx_chat_modules_chat ON chat_modules(chat_id, is_enabled);
CREATE INDEX idx_chat_modules_enabled ON chat_modules(module_name, is_enabled);

-- ============================================================
-- MESSAGES TABLE (единая для всех чатов, партиционирование по дате)
-- ============================================================

-- Родительская таблица (партиционирование по created_at)
CREATE TABLE IF NOT EXISTS messages (
    id BIGSERIAL,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    content_type VARCHAR(20) NOT NULL, -- 'text', 'photo', 'video', 'sticker', etc.
    text TEXT,
    caption TEXT,
    file_id TEXT, -- для фото/видео/стикеров
    metadata JSONB DEFAULT '{}', -- дополнительные данные (reply_to, forward_from, etc.)
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Индексы на родительской таблице (будут наследоваться партициями)
CREATE INDEX idx_messages_chat_user ON messages(chat_id, user_id, created_at DESC);
CREATE INDEX idx_messages_chat_content ON messages(chat_id, content_type, created_at DESC);
CREATE INDEX idx_messages_unique ON messages(chat_id, message_id, created_at);

-- Создаём партиции на 3 месяца вперед (автоматизация через pg_partman или cron)
CREATE TABLE messages_2025_10 PARTITION OF messages
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');
CREATE TABLE messages_2025_11 PARTITION OF messages
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
CREATE TABLE messages_2025_12 PARTITION OF messages
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

-- ============================================================
-- MODULE: LIMITER (лимиты на контент)
-- ============================================================

CREATE TABLE IF NOT EXISTS limiter_config (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    user_id BIGINT DEFAULT NULL, -- NULL = настройки для всех пользователей (allmembers)
    content_type VARCHAR(20) NOT NULL, -- 'photo', 'video', 'sticker', 'text', etc.
    daily_limit INT NOT NULL, -- -1 = запрет, 0 = без лимита, N = лимит
    warning_threshold INT DEFAULT 2, -- за сколько сообщений до лимита предупреждать
    is_vip BOOLEAN DEFAULT false, -- VIP игнорирует лимиты
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, COALESCE(user_id, -1), content_type) -- -1 для NULL user_id
);

CREATE INDEX idx_limiter_chat ON limiter_config(chat_id, content_type);
CREATE INDEX idx_limiter_user ON limiter_config(chat_id, user_id) WHERE user_id IS NOT NULL;

-- Счётчики (кэш для быстрого доступа, обновляется триггерами или периодически)
CREATE TABLE IF NOT EXISTS limiter_counters (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    content_type VARCHAR(20) NOT NULL,
    counter_date DATE NOT NULL DEFAULT CURRENT_DATE,
    count INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, user_id, content_type, counter_date)
);

CREATE INDEX idx_limiter_counters_lookup ON limiter_counters(chat_id, user_id, counter_date);

-- ============================================================
-- MODULE: REACTIONS (реакции на ключевые слова)
-- ============================================================

CREATE TABLE IF NOT EXISTS reactions_config (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    user_id BIGINT DEFAULT NULL, -- NULL = для всех пользователей
    content_type VARCHAR(20) NOT NULL, -- 'text', 'photo', 'video', etc.
    trigger_type VARCHAR(20) NOT NULL, -- 'regex', 'exact', 'contains'
    trigger_pattern TEXT NOT NULL, -- regex или текст для поиска
    reaction_type VARCHAR(20) NOT NULL, -- 'text', 'sticker', 'delete', 'mute'
    reaction_data TEXT, -- текст ответа или file_id стикера
    violation_code INT DEFAULT 0, -- код нарушения для статистики
    cooldown_minutes INT DEFAULT 10, -- антифлуд: сколько минут между реакциями
    is_enabled BOOLEAN DEFAULT true,
    is_vip BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_reactions_chat ON reactions_config(chat_id, content_type, is_enabled);

-- Логи реакций (для антифлуда и статистики)
CREATE TABLE IF NOT EXISTS reactions_log (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    reaction_id BIGINT NOT NULL REFERENCES reactions_config(id) ON DELETE CASCADE,
    message_id BIGINT NOT NULL,
    triggered_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_reactions_log_cooldown ON reactions_log(chat_id, reaction_id, triggered_at DESC);

-- ============================================================
-- MODULE: ANTISPAM (антиспам фильтры)
-- ============================================================

CREATE TABLE IF NOT EXISTS antispam_config (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    rule_name VARCHAR(50) NOT NULL,
    rule_type VARCHAR(20) NOT NULL, -- 'flood', 'duplicate', 'links', 'mentions'
    threshold_value INT, -- порог срабатывания (например, 5 сообщений в минуту)
    action VARCHAR(20) NOT NULL, -- 'warn', 'delete', 'mute', 'ban'
    is_enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_antispam_chat ON antispam_config(chat_id, is_enabled);

-- ============================================================
-- MODULE: STATISTICS (кэшированная статистика)
-- ============================================================

CREATE TABLE IF NOT EXISTS statistics_daily (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    stat_date DATE NOT NULL DEFAULT CURRENT_DATE,
    content_type VARCHAR(20) NOT NULL,
    message_count INT DEFAULT 0,
    violation_count INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, user_id, stat_date, content_type)
);

CREATE INDEX idx_statistics_lookup ON statistics_daily(chat_id, user_id, stat_date);

-- ============================================================
-- MODULE: SCHEDULER (задачи по расписанию)
-- ============================================================

CREATE TABLE IF NOT EXISTS scheduler_tasks (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    task_name VARCHAR(100) NOT NULL,
    task_type VARCHAR(20) NOT NULL, -- 'sticker', 'text', 'poll', 'custom'
    cron_expression VARCHAR(100) NOT NULL, -- '0 9 * * *' = каждый день в 9:00
    task_data JSONB NOT NULL, -- данные для выполнения (file_id стикера, текст, etc.)
    is_enabled BOOLEAN DEFAULT true,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, task_name)
);

CREATE INDEX idx_scheduler_next_run ON scheduler_tasks(is_enabled, next_run_at);

-- ============================================================
-- SYSTEM TABLES (мета-информация)
-- ============================================================

-- Логи событий (для отладки и аудита)
CREATE TABLE IF NOT EXISTS event_log (
    id BIGSERIAL PRIMARY KEY,
    event_type VARCHAR(50) NOT NULL, -- 'message_deleted', 'user_muted', 'limit_exceeded', etc.
    chat_id BIGINT,
    user_id BIGINT,
    module_name VARCHAR(50),
    event_data JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_event_log_time ON event_log(created_at DESC);
CREATE INDEX idx_event_log_chat ON event_log(chat_id, created_at DESC);

-- Настройки бота (глобальные)
CREATE TABLE IF NOT EXISTS bot_settings (
    id SERIAL PRIMARY KEY,
    key VARCHAR(100) UNIQUE NOT NULL,
    value TEXT,
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- TRIGGERS & FUNCTIONS
-- ============================================================

-- Функция для обновления updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Триггеры для updated_at на всех таблицах
CREATE TRIGGER update_chats_updated_at BEFORE UPDATE ON chats
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_chat_admins_updated_at BEFORE UPDATE ON chat_admins
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_chat_modules_updated_at BEFORE UPDATE ON chat_modules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_limiter_config_updated_at BEFORE UPDATE ON limiter_config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_reactions_config_updated_at BEFORE UPDATE ON reactions_config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- VIEWS (для удобства запросов)
-- ============================================================

-- Вью: активные модули по чатам
CREATE OR REPLACE VIEW v_active_modules AS
SELECT 
    c.chat_id,
    c.title AS chat_title,
    cm.module_name,
    cm.config,
    cm.updated_at
FROM chats c
JOIN chat_modules cm ON c.chat_id = cm.chat_id
WHERE c.is_active = true AND cm.is_enabled = true;

-- Вью: суточная статистика по чатам
CREATE OR REPLACE VIEW v_daily_chat_stats AS
SELECT 
    chat_id,
    stat_date,
    content_type,
    SUM(message_count) as total_messages,
    SUM(violation_count) as total_violations
FROM statistics_daily
GROUP BY chat_id, stat_date, content_type
ORDER BY stat_date DESC, chat_id;

-- ============================================================
-- SEED DATA (начальные данные)
-- ============================================================

-- Вставляем доступные модули
INSERT INTO bot_settings (key, value, description) VALUES
    ('available_modules', 'limiter,reactions,antispam,statistics,scheduler', 'Список доступных модулей через запятую'),
    ('bot_version', '1.0.0', 'Версия бота'),
    ('default_timezone', 'UTC', 'Часовой пояс по умолчанию')
ON CONFLICT (key) DO NOTHING;

-- ============================================================
-- COMMENTS (документация схемы)
-- ============================================================

COMMENT ON TABLE chats IS 'Метаинформация о чатах где работает бот';
COMMENT ON TABLE chat_modules IS 'Включение/выключение модулей для каждого чата';
COMMENT ON TABLE limiter_config IS 'Настройки лимитов на контент (модуль limiter)';
COMMENT ON TABLE reactions_config IS 'Настройки реакций на ключевые слова (модуль reactions)';
COMMENT ON TABLE scheduler_tasks IS 'Задачи по расписанию (модуль scheduler)';
COMMENT ON TABLE event_log IS 'Лог всех событий для аудита и отладки';

COMMENT ON COLUMN messages.metadata IS 'JSONB поле для хранения reply_to_message_id, forward_from, entities и другой мета-информации';
COMMENT ON COLUMN chat_modules.config IS 'JSONB конфигурация специфичная для модуля. Например: {"max_warnings": 3, "ban_duration": 86400}';
