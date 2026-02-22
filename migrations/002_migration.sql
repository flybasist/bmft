-- ============================================================================
-- BMFT Migration: v1.0 → v1.1
-- ============================================================================
-- Применяется автоматически при обновлении с v1.0 (schema_migrations version=1).
-- При свежей установке эта миграция пропускается (001 уже создаёт актуальную схему).
-- ============================================================================

-- 1. scheduled_tasks.action_data: JSONB → TEXT
-- Go-код записывает простые строки (file_id, текст), не JSON.
ALTER TABLE scheduled_tasks ALTER COLUMN action_data TYPE TEXT USING action_data::TEXT;
ALTER TABLE scheduled_tasks ALTER COLUMN action_data SET DEFAULT '';

-- 2. keyword_reactions: добавляем колонку action для фильтров
-- NULL = обычная реакция (ответ), NOT NULL = фильтр (delete/warn/delete_warn)
ALTER TABLE keyword_reactions ADD COLUMN IF NOT EXISTS action VARCHAR(20) DEFAULT NULL;

-- 3. Переносим данные из banned_words в keyword_reactions (если таблица существует)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'banned_words') THEN
        INSERT INTO keyword_reactions (chat_id, thread_id, pattern, is_regex, response_type, response_content, description, action, is_active, created_at, updated_at)
        SELECT chat_id, thread_id, pattern, is_regex, 'none', '', 'Migrated from banned_words', action, is_active, created_at, updated_at
        FROM banned_words;
    END IF;
END $$;

-- 4. Удаляем таблицу banned_words (данные перенесены в keyword_reactions)
DROP TABLE IF EXISTS banned_words;

-- 5. Удаляем таблицу users (никогда не использовалась в Go-коде)
DROP TABLE IF EXISTS users;

-- 6. Удаляем materialized view daily_content_stats (не использовался в Go-коде)
DROP MATERIALIZED VIEW IF EXISTS daily_content_stats;

-- 7. Удаляем функцию refresh_stats_views (зависела от удалённых объектов)
DROP FUNCTION IF EXISTS refresh_stats_views();

-- Запись версии миграции
INSERT INTO schema_migrations (version, description)
VALUES (2, 'v1.0 to v1.1: consolidate textfilter+profanity into reactions, drop dead objects')
ON CONFLICT (version) DO NOTHING;
