-- BMFT v0.6.0 Initial Schema

CREATE TABLE chats (
    chat_id BIGINT PRIMARY KEY,
    chat_type VARCHAR(20) NOT NULL,
    title TEXT,
    username TEXT,
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
    user_id BIGINT NOT NULL,
    granted_by BIGINT,
    granted_at TIMESTAMPTZ DEFAULT NOW(),
    reason TEXT,
    UNIQUE(chat_id, user_id)
);

CREATE INDEX idx_chat_vips_lookup ON chat_vips(chat_id, user_id);

CREATE TABLE chat_modules (
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

CREATE TABLE messages (
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

CREATE TABLE content_limits (
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
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_content_limits_unique ON content_limits(chat_id, COALESCE(user_id, -1));
CREATE INDEX idx_content_limits_chat ON content_limits(chat_id);

CREATE TABLE content_counters (
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

CREATE TABLE keyword_reactions (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    pattern TEXT NOT NULL,
    is_regex BOOLEAN DEFAULT TRUE,
    response_type TEXT DEFAULT 'text',
    response_content TEXT NOT NULL,
    description TEXT,
    cooldown INTEGER DEFAULT 3600,
    daily_limit INTEGER DEFAULT 0,
    delete_on_limit BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_keyword_reactions_chat ON keyword_reactions(chat_id, is_active);

CREATE TABLE reaction_triggers (
    chat_id BIGINT NOT NULL,
    reaction_id INTEGER NOT NULL,
    user_id BIGINT NOT NULL,
    last_triggered_at TIMESTAMPTZ DEFAULT NOW(),
    trigger_count INTEGER DEFAULT 0,
    PRIMARY KEY (chat_id, reaction_id, user_id)
);

CREATE TABLE reaction_daily_counters (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    reaction_id BIGINT NOT NULL,
    counter_date DATE NOT NULL DEFAULT CURRENT_DATE,
    count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(chat_id, reaction_id, counter_date)
);

CREATE INDEX idx_reaction_daily_counters_lookup ON reaction_daily_counters(chat_id, reaction_id, counter_date);

CREATE TABLE banned_words (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    pattern TEXT NOT NULL,
    is_regex BOOLEAN DEFAULT TRUE,
    action VARCHAR(20) NOT NULL CHECK (action IN ('delete', 'warn', 'delete_warn')),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_banned_words_chat ON banned_words(chat_id, is_active);

CREATE TABLE scheduled_tasks (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    task_name VARCHAR(100) NOT NULL,
    cron_expression VARCHAR(100) NOT NULL,
    action_type VARCHAR(50) NOT NULL,
    action_data JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN DEFAULT TRUE,
    last_run TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_scheduled_tasks_active ON scheduled_tasks(chat_id, is_active);

CREATE TABLE bot_settings (
    id SERIAL PRIMARY KEY,
    bot_version TEXT DEFAULT '0.6.0-dev',
    timezone TEXT DEFAULT 'UTC',
    available_modules TEXT[] DEFAULT ARRAY['core', 'limiter', 'statistics', 'reactions', 'scheduler', 'textfilter']
);

INSERT INTO bot_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
