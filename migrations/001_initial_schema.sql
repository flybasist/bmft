-- BMFT v0.8.0 Initial Schema with Topics Support

CREATE TABLE chats (
    chat_id BIGINT PRIMARY KEY,
    chat_type VARCHAR(20) NOT NULL,
    title TEXT,
    username TEXT,
    is_forum BOOLEAN DEFAULT FALSE,  -- TRUE для супергрупп с включенными топиками
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_chats_active ON chats(is_active);

CREATE TABLE users (
    user_id BIGINT PRIMARY KEY,
    username TEXT,
    first_name TEXT,
    last_name TEXT,
    is_bot BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE chat_vips (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    thread_id BIGINT DEFAULT 0,  -- 0 = VIP на весь чат, >0 = VIP только в конкретном топике
    user_id BIGINT NOT NULL,
    granted_by BIGINT,
    granted_at TIMESTAMPTZ DEFAULT NOW(),
    reason TEXT,
    UNIQUE(chat_id, thread_id, user_id)
);

CREATE INDEX idx_chat_vips_lookup ON chat_vips(chat_id, thread_id, user_id);

CREATE TABLE messages (
    id BIGSERIAL,
    chat_id BIGINT NOT NULL,
    thread_id BIGINT DEFAULT 0,  -- 0 = основной чат, >0 = сообщение в топике
    user_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    content_type VARCHAR(20) NOT NULL,
    text TEXT,
    caption TEXT,
    file_id TEXT,
    metadata JSONB DEFAULT '{}',
    was_deleted BOOLEAN DEFAULT FALSE,
    deletion_reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_messages_chat_user ON messages(chat_id, thread_id, user_id, created_at DESC);
CREATE INDEX idx_messages_content_type ON messages(chat_id, thread_id, content_type, created_at DESC);
CREATE INDEX idx_messages_metadata ON messages USING GIN (metadata);

CREATE TABLE messages_2025_10 PARTITION OF messages FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');
CREATE TABLE messages_2025_11 PARTITION OF messages FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
CREATE TABLE messages_2025_12 PARTITION OF messages FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

CREATE TABLE content_limits (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    thread_id BIGINT DEFAULT 0,  -- 0 = лимит для всего чата, >0 = лимит для топика
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

-- Materialized view для статистики контента (заменяет content_counters)
CREATE MATERIALIZED VIEW daily_content_stats AS
SELECT 
    chat_id,
    thread_id,
    user_id,
    DATE(created_at) as stat_date,
    content_type,
    COUNT(*) as message_count,
    COUNT(*) FILTER (WHERE was_deleted = TRUE) as deleted_count
FROM messages
WHERE was_deleted = FALSE
GROUP BY chat_id, thread_id, user_id, DATE(created_at), content_type;

CREATE UNIQUE INDEX idx_daily_content_stats_pk 
    ON daily_content_stats(chat_id, thread_id, user_id, stat_date, content_type);

CREATE INDEX idx_daily_content_stats_lookup 
    ON daily_content_stats(chat_id, thread_id, user_id, stat_date);

CREATE TABLE keyword_reactions (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    thread_id BIGINT DEFAULT 0,  -- 0 = реакция для всего чата, >0 = реакция только для топика
    user_id BIGINT DEFAULT NULL,  -- NULL/0 = для всех пользователей, >0 = только для конкретного user_id (персональная пасхалка)
    pattern TEXT NOT NULL,
    is_regex BOOLEAN DEFAULT TRUE,
    response_type TEXT DEFAULT 'text',
    response_content TEXT NOT NULL,
    description TEXT,
    trigger_content_type TEXT DEFAULT NULL,  -- NULL = любой контент, 'photo' = только фото, 'video' = только видео, 'sticker' = только стикеры, etc.
    cooldown INTEGER DEFAULT 3600,
    daily_limit INTEGER DEFAULT 0,
    delete_on_limit BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_keyword_reactions_chat ON keyword_reactions(chat_id, thread_id, is_active);
CREATE INDEX idx_keyword_reactions_user ON keyword_reactions(chat_id, thread_id, user_id) WHERE user_id IS NOT NULL;

-- Materialized view для статистики реакций (заменяет reaction_triggers и reaction_daily_counters)
CREATE MATERIALIZED VIEW daily_reaction_stats AS
SELECT 
    chat_id,
    thread_id,
    DATE(created_at) as stat_date,
    jsonb_array_elements_text(metadata->'reactions'->'triggered')::INTEGER as reaction_id,
    COUNT(*) as trigger_count
FROM messages
WHERE metadata ? 'reactions' 
  AND metadata->'reactions' ? 'triggered'
  AND was_deleted = FALSE
GROUP BY chat_id, thread_id, DATE(created_at), reaction_id;

CREATE UNIQUE INDEX idx_daily_reaction_stats_pk 
    ON daily_reaction_stats(chat_id, thread_id, stat_date, reaction_id);

CREATE TABLE banned_words (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    thread_id BIGINT DEFAULT 0,  -- 0 = бан-слово для всего чата, >0 = только для топика
    pattern TEXT NOT NULL,
    is_regex BOOLEAN DEFAULT TRUE,
    action VARCHAR(20) NOT NULL CHECK (action IN ('delete', 'warn', 'delete_warn')),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_banned_words_chat ON banned_words(chat_id, thread_id, is_active);

CREATE TABLE scheduled_tasks (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    thread_id BIGINT DEFAULT 0,  -- 0 = отправка в основной чат, >0 = отправка в конкретный топик
    task_name VARCHAR(100) NOT NULL,
    cron_expression VARCHAR(100) NOT NULL,
    action_type VARCHAR(50) NOT NULL,
    action_data JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN DEFAULT TRUE,
    last_run TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_scheduled_tasks_active ON scheduled_tasks(chat_id, thread_id, is_active);

CREATE TABLE event_log (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    user_id BIGINT,
    module_name VARCHAR(50) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    details TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_event_log_chat ON event_log(chat_id, created_at DESC);
CREATE INDEX idx_event_log_module ON event_log(module_name, created_at DESC);
CREATE INDEX idx_event_log_metadata ON event_log USING GIN (metadata);

-- Profanity Filter: глобальный словарь матерных слов
CREATE TABLE profanity_dictionary (
    id BIGSERIAL PRIMARY KEY,
    pattern TEXT NOT NULL UNIQUE,
    is_regex BOOLEAN DEFAULT FALSE,
    severity VARCHAR(20) DEFAULT 'moderate', -- 'mild', 'moderate', 'severe' (задел на будущее)
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_profanity_pattern ON profanity_dictionary(pattern);
CREATE INDEX idx_profanity_severity ON profanity_dictionary(severity);

-- Profanity Filter: настройки фильтра для чата/треда
CREATE TABLE profanity_settings (
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    thread_id BIGINT DEFAULT 0,
    action VARCHAR(20) DEFAULT 'delete', -- 'delete', 'warn', 'delete_warn'
    warn_text TEXT, -- Кастомное предупреждение (опционально)
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (chat_id, thread_id)
);

CREATE INDEX idx_profanity_settings_chat ON profanity_settings(chat_id, thread_id);

CREATE TABLE bot_settings (
    id SERIAL PRIMARY KEY,
    bot_version TEXT DEFAULT '1.0',
    timezone TEXT DEFAULT 'UTC',
    available_modules TEXT[] DEFAULT ARRAY['core', 'limiter', 'statistics', 'reactions', 'scheduler', 'textfilter', 'profanityfilter']
);

INSERT INTO bot_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- Функция для обновления материализованных представлений (вызывается cron-задачей)
CREATE OR REPLACE FUNCTION refresh_stats_views() 
RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY daily_content_stats;
    REFRESH MATERIALIZED VIEW CONCURRENTLY daily_reaction_stats;
END;
$$ LANGUAGE plpgsql;
