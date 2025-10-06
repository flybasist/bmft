-- ============================================================
-- BMFT Database Schema v0.6.0 - REFACTORED
-- ============================================================
-- Версия: 0.6.0-dev
-- Дата: 2025-10-06
-- 
-- ФИЛОСОФИЯ:
-- 1. Ничего не хардкодим - всё через БД и команды ТГ
-- 2. Упрощённая логика - без магических чисел  
-- 3. VIP система - обход всех лимитов
-- 4. Понятность > сложность
--
-- ИЗМЕНЕНИЯ от v0.5:
-- - Убраны violation codes → явные типы
-- - Лимиты на КАЖДЫЙ тип контента
-- - VIP через отдельную таблицу
-- - Scheduler полностью на БД
-- ============================================================

-- ============================================================
-- CORE: Базовые таблицы
-- ============================================================

CREATE TABLE IF NOT EXISTS chats (
    chat_id BIGINT PRIMARY KEY,
    chat_type VARCHAR(20) NOT NULL,
    title TEXT,
    username TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_chats_active ON chats(is_active);

CREATE TABLE IF NOT EXISTS users (
    user_id BIGINT PRIMARY KEY,
    username TEXT,
    first_name TEXT,
    last_name TEXT,
    is_bot BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS chat_vips (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    granted_by BIGINT,
    granted_at TIMESTAMPTZ DEFAULT NOW(),
    reason TEXT,
    UNIQUE(chat_id, user_id)
);

CREATE INDEX idx_chat_vips_lookup ON chat_vips(chat_id, user_id);

CREATE TABLE IF NOT EXISTS chat_modules (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    module_name VARCHAR(50) NOT NULL,
    is_enabled BOOLEAN DEFAULT FALSE,
    config JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, module_name)
);

CREATE INDEX idx_chat_modules_enabled ON chat_modules(chat_id, is_enabled);

-- ============================================================
-- MESSAGES: Партиционирование по месяцам
-- ============================================================

CREATE TABLE IF NOT EXISTS messages (
    id BIGSERIAL,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    content_type VARCHAR(20) NOT NULL,
    text TEXT,
    caption TEXT,
    file_id TEXT,
    was_deleted BOOLEAN DEFAULT FALSE,
    deletion_reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_messages_chat_user ON messages(chat_id, user_id, created_at DESC);
CREATE INDEX idx_messages_content_type ON messages(chat_id, content_type, created_at DESC);

CREATE TABLE messages_2025_10 PARTITION OF messages FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');
CREATE TABLE messages_2025_11 PARTITION OF messages FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
CREATE TABLE messages_2025_12 PARTITION OF messages FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

-- ============================================================
-- MODULE: LIMITER
-- ============================================================

CREATE TABLE IF NOT EXISTS content_limits (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    user_id BIGINT,
    limit_text INTEGER DEFAULT 0,
    limit_photo INTEGER DEFAULT 0,
    limit_video INTEGER DEFAULT 0,
    limit_sticker INTEGER DEFAULT 0,
    limit_animation INTEGER DEFAULT 0,
    limit_voice INTEGER DEFAULT 0,
    limit_video_note INTEGER DEFAULT 0,
    limit_audio INTEGER DEFAULT 0,
    limit_document INTEGER DEFAULT 0,
    limit_location INTEGER DEFAULT 0,
    limit_contact INTEGER DEFAULT 0,
    limit_banned_words INTEGER DEFAULT 0,
    warning_threshold INTEGER DEFAULT 2,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, COALESCE(user_id, -1))
);

CREATE INDEX idx_content_limits_chat ON content_limits(chat_id);

CREATE TABLE IF NOT EXISTS content_counters (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    counter_date DATE NOT NULL DEFAULT CURRENT_DATE,
    count_text INTEGER DEFAULT 0,
    count_photo INTEGER DEFAULT 0,
    count_video INTEGER DEFAULT 0,
    count_sticker INTEGER DEFAULT 0,
    count_animation INTEGER DEFAULT 0,
    count_voice INTEGER DEFAULT 0,
    count_video_note INTEGER DEFAULT 0,
    count_audio INTEGER DEFAULT 0,
    count_document INTEGER DEFAULT 0,
    count_location INTEGER DEFAULT 0,
    count_contact INTEGER DEFAULT 0,
    count_banned_words INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, user_id, counter_date)
);

CREATE INDEX idx_content_counters_lookup ON content_counters(chat_id, user_id, counter_date);

-- ============================================================
-- MODULE: REACTIONS
-- ============================================================

CREATE TABLE IF NOT EXISTS keyword_reactions (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    user_id BIGINT,
    content_types TEXT[] NOT NULL,
    trigger_pattern TEXT NOT NULL,
    case_sensitive BOOLEAN DEFAULT FALSE,
    reaction_type VARCHAR(20) NOT NULL,
    reaction_data TEXT NOT NULL,
    cooldown_minutes INTEGER DEFAULT 60,
    last_triggered_at TIMESTAMPTZ,
    description TEXT,
    is_enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_keyword_reactions_chat ON keyword_reactions(chat_id, is_enabled);

CREATE TABLE IF NOT EXISTS banned_words (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    pattern TEXT NOT NULL,
    pattern_type VARCHAR(20) DEFAULT 'regex',
    case_sensitive BOOLEAN DEFAULT FALSE,
    action VARCHAR(20) NOT NULL,
    warning_message TEXT,
    description TEXT,
    is_enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_banned_words_chat ON banned_words(chat_id, is_enabled);

CREATE TABLE IF NOT EXISTS reaction_triggers (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    trigger_type VARCHAR(20) NOT NULL,
    trigger_id BIGINT,
    matched_text TEXT,
    action_taken VARCHAR(20),
    reaction_sent TEXT,
    triggered_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_reaction_triggers_cooldown ON reaction_triggers(trigger_type, trigger_id, triggered_at DESC);

-- ============================================================
-- MODULE: SCHEDULER
-- ============================================================

CREATE TABLE IF NOT EXISTS scheduled_tasks (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    task_name VARCHAR(100) NOT NULL,
    description TEXT,
    cron_expression VARCHAR(100) NOT NULL,
    timezone VARCHAR(50) DEFAULT 'UTC',
    action_type VARCHAR(20) NOT NULL,
    action_data JSONB NOT NULL,
    is_enabled BOOLEAN DEFAULT TRUE,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    run_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, task_name)
);

CREATE INDEX idx_scheduled_tasks_next_run ON scheduled_tasks(is_enabled, next_run_at) WHERE is_enabled = TRUE;

-- ============================================================
-- MODULE: STATISTICS
-- ============================================================

CREATE TABLE IF NOT EXISTS statistics_daily (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    stat_date DATE NOT NULL DEFAULT CURRENT_DATE,
    messages_text INTEGER DEFAULT 0,
    messages_photo INTEGER DEFAULT 0,
    messages_video INTEGER DEFAULT 0,
    messages_sticker INTEGER DEFAULT 0,
    messages_animation INTEGER DEFAULT 0,
    messages_voice INTEGER DEFAULT 0,
    messages_video_note INTEGER DEFAULT 0,
    messages_audio INTEGER DEFAULT 0,
    messages_document INTEGER DEFAULT 0,
    messages_location INTEGER DEFAULT 0,
    messages_contact INTEGER DEFAULT 0,
    violations_banned_words INTEGER DEFAULT 0,
    violations_deleted INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, user_id, stat_date)
);

CREATE INDEX idx_statistics_lookup ON statistics_daily(chat_id, user_id, stat_date DESC);

-- ============================================================
-- SYSTEM
-- ============================================================

CREATE TABLE IF NOT EXISTS event_log (
    id BIGSERIAL PRIMARY KEY,
    event_type VARCHAR(50) NOT NULL,
    chat_id BIGINT,
    user_id BIGINT,
    module_name VARCHAR(50),
    event_data JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_event_log_time ON event_log(created_at DESC);

CREATE TABLE IF NOT EXISTS bot_settings (
    key VARCHAR(100) PRIMARY KEY,
    value TEXT,
    description TEXT,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- TRIGGERS
-- ============================================================

CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $func$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$func$ LANGUAGE plpgsql;

CREATE TRIGGER trg_chats_updated_at BEFORE UPDATE ON chats FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_chat_modules_updated_at BEFORE UPDATE ON chat_modules FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_content_limits_updated_at BEFORE UPDATE ON content_limits FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- ============================================================
-- VIEWS
-- ============================================================

CREATE OR REPLACE VIEW v_active_modules AS
SELECT c.chat_id, c.title, cm.module_name, cm.is_enabled, cm.config
FROM chats c
JOIN chat_modules cm ON c.chat_id = cm.chat_id
WHERE c.is_active = TRUE;

CREATE OR REPLACE VIEW v_chat_vips AS
SELECT cv.chat_id, c.title AS chat_title, cv.user_id, u.username, u.first_name, cv.granted_at, cv.reason
FROM chat_vips cv
JOIN chats c ON cv.chat_id = c.chat_id
LEFT JOIN users u ON cv.user_id = u.user_id;

-- ============================================================
-- SEED DATA
-- ============================================================

INSERT INTO bot_settings (key, value, description) VALUES
    ('bot_version', '0.6.0-dev', 'Версия бота'),
    ('default_timezone', 'UTC', 'Часовой пояс по умолчанию'),
    ('available_modules', 'limiter,reactions,statistics,scheduler', 'Список доступных модулей')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

-- ============================================================
-- COMMENTS
-- ============================================================

COMMENT ON TABLE chat_vips IS 'VIP пользователи - обходят ВСЕ лимиты';
COMMENT ON TABLE content_limits IS 'Лимиты: -1=запрет, 0=без лимита, N=макс в день';
COMMENT ON TABLE keyword_reactions IS 'Автореакции на ключевые слова (regex)';
COMMENT ON TABLE banned_words IS 'Запрещённые слова с действиями';
COMMENT ON TABLE scheduled_tasks IS 'Задачи по расписанию (cron)';
