-- ============================================================================
-- BMFT v1.1 — Актуальная схема базы данных
-- ============================================================================
-- При свежей установке создаётся только этот файл.
-- При обновлении с v1.0 — применяется миграция 002_migration.sql.
-- Партиции для messages и event_log создаются автоматически модулем Maintenance.
-- ============================================================================

-- Таблица версий миграций
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- Core tables
-- ============================================================================

CREATE TABLE chats (
    chat_id BIGINT PRIMARY KEY,
    chat_type VARCHAR(20) NOT NULL,
    title TEXT,
    username TEXT,
    is_forum BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_chats_active ON chats(is_active);

CREATE TABLE chat_vips (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    thread_id BIGINT DEFAULT 0,
    user_id BIGINT NOT NULL,
    granted_by BIGINT,
    granted_at TIMESTAMPTZ DEFAULT NOW(),
    reason TEXT,
    UNIQUE(chat_id, thread_id, user_id)
);

CREATE INDEX idx_chat_vips_lookup ON chat_vips(chat_id, thread_id, user_id);

-- Партиционированная таблица сообщений.
-- Партиции создаются автоматически модулем Maintenance при запуске бота.
CREATE TABLE messages (
    id BIGSERIAL,
    chat_id BIGINT NOT NULL,
    thread_id BIGINT DEFAULT 0,
    user_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    content_type VARCHAR(20) NOT NULL,
    text TEXT,
    caption TEXT,
    file_id TEXT,
    chat_name TEXT,
    metadata JSONB DEFAULT '{}',
    was_deleted BOOLEAN DEFAULT FALSE,
    deletion_reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_messages_chat_user ON messages(chat_id, thread_id, user_id, created_at DESC);
CREATE INDEX idx_messages_content_type ON messages(chat_id, thread_id, content_type, created_at DESC);
CREATE INDEX idx_messages_metadata ON messages USING GIN (metadata);
CREATE INDEX idx_messages_chat_name ON messages(chat_name);

-- ============================================================================
-- Limiter Module
-- ============================================================================

CREATE TABLE content_limits (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    thread_id BIGINT DEFAULT 0,
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
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_content_limits_unique ON content_limits(chat_id, thread_id, COALESCE(user_id, -1));
CREATE INDEX idx_content_limits_chat ON content_limits(chat_id, thread_id);

-- ============================================================================
-- Reactions Module (включает фильтры запрещённых слов и автоответы)
-- ============================================================================

CREATE TABLE keyword_reactions (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    thread_id BIGINT DEFAULT 0,
    user_id BIGINT DEFAULT NULL,
    pattern TEXT NOT NULL,
    is_regex BOOLEAN DEFAULT TRUE,
    response_type TEXT DEFAULT 'text',
    response_content TEXT NOT NULL,
    description TEXT,
    trigger_content_type TEXT DEFAULT NULL,
    cooldown INTEGER DEFAULT 3600,
    daily_limit INTEGER DEFAULT 0,
    delete_on_limit BOOLEAN DEFAULT FALSE,
    action VARCHAR(20) DEFAULT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

COMMENT ON COLUMN keyword_reactions.action IS 'NULL = реакция (ответ текстом/стикером), delete/warn/delete_warn = фильтр';

CREATE INDEX idx_keyword_reactions_chat ON keyword_reactions(chat_id, thread_id, is_active);
CREATE INDEX idx_keyword_reactions_user ON keyword_reactions(chat_id, thread_id, user_id) WHERE user_id IS NOT NULL;

-- Счётчик срабатываний реакций (для cooldown)
CREATE TABLE reaction_triggers (
    chat_id BIGINT NOT NULL,
    reaction_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    last_triggered_at TIMESTAMPTZ DEFAULT NOW(),
    trigger_count BIGINT DEFAULT 1,
    PRIMARY KEY (chat_id, reaction_id)
);

CREATE INDEX idx_reaction_triggers_time ON reaction_triggers(last_triggered_at);

-- Дневной счётчик срабатываний (для daily_limit)
CREATE TABLE reaction_daily_counters (
    chat_id BIGINT NOT NULL,
    reaction_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL DEFAULT 0,
    counter_date DATE NOT NULL DEFAULT CURRENT_DATE,
    count INTEGER DEFAULT 0,
    PRIMARY KEY (chat_id, reaction_id, user_id, counter_date)
);

CREATE INDEX idx_reaction_daily_counters_date ON reaction_daily_counters(counter_date);

-- ============================================================================
-- Profanity Filter (глобальный словарь + per-chat настройки)
-- ============================================================================

CREATE TABLE profanity_dictionary (
    id BIGSERIAL PRIMARY KEY,
    pattern TEXT NOT NULL UNIQUE,
    is_regex BOOLEAN DEFAULT FALSE,
    severity VARCHAR(20) DEFAULT 'moderate',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_profanity_pattern ON profanity_dictionary(pattern);
CREATE INDEX idx_profanity_severity ON profanity_dictionary(severity);

CREATE TABLE profanity_settings (
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    thread_id BIGINT DEFAULT 0,
    action VARCHAR(20) DEFAULT 'delete',
    warn_text TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (chat_id, thread_id)
);

CREATE INDEX idx_profanity_settings_chat ON profanity_settings(chat_id, thread_id);

-- ============================================================================
-- Scheduler Module
-- ============================================================================

CREATE TABLE scheduled_tasks (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    thread_id BIGINT DEFAULT 0,
    task_name VARCHAR(100) NOT NULL,
    cron_expression VARCHAR(100) NOT NULL,
    action_type VARCHAR(50) NOT NULL,
    action_data TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN DEFAULT TRUE,
    last_run TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_scheduled_tasks_active ON scheduled_tasks(chat_id, thread_id, is_active);

-- ============================================================================
-- System tables
-- ============================================================================

-- Партиционированный лог событий.
-- Партиции создаются автоматически модулем Maintenance при запуске бота.
CREATE TABLE event_log (
    id BIGSERIAL,
    chat_id BIGINT NOT NULL,
    user_id BIGINT,
    module_name VARCHAR(50) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    details TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_event_log_chat ON event_log(chat_id, created_at DESC);
CREATE INDEX idx_event_log_module ON event_log(module_name, created_at DESC);
CREATE INDEX idx_event_log_metadata ON event_log USING GIN (metadata);

CREATE TABLE bot_settings (
    id SERIAL PRIMARY KEY,
    bot_version TEXT DEFAULT '1.1.1',
    timezone TEXT DEFAULT 'UTC',
    available_modules TEXT[] DEFAULT ARRAY['core', 'limiter', 'statistics', 'reactions', 'scheduler']
);

INSERT INTO bot_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- Record initial schema version
INSERT INTO schema_migrations (version, description)
VALUES (1, 'v1.1 initial schema')
ON CONFLICT (version) DO NOTHING;
