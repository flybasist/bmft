-- ============================================================================
-- BMFT Migration: v1.1.1 → v1.2 (welcome message per-chat settings)
-- ============================================================================
-- Добавляет per-chat настройки приветственного сообщения в таблицу chats:
--   welcome_enabled      — выключатель приветствий для нового пользователя
--   welcome_ttl_seconds  — TTL приветствия до авто-удаления (0 = не удалять)
-- ============================================================================

ALTER TABLE chats
    ADD COLUMN IF NOT EXISTS welcome_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS welcome_ttl_seconds INTEGER NOT NULL DEFAULT 60;

UPDATE bot_settings SET bot_version = '1.2' WHERE id = 1;

INSERT INTO schema_migrations (version, description)
VALUES (4, 'v1.1.1 to v1.2: per-chat welcome message settings (enabled, ttl)')
ON CONFLICT (version) DO NOTHING;
