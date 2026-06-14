-- Описание: Добавление колонки limit_via в таблицу content_limits для лимитирования инлайн-ботов

ALTER TABLE content_limits ADD COLUMN IF NOT EXISTS limit_via INTEGER DEFAULT 0;

-- Обновление версии бота
UPDATE bot_settings SET bot_version = '1.3';

-- Запись версии миграции
INSERT INTO schema_migrations (version, description) 
VALUES (6, 'Add limit_via to content_limits')
ON CONFLICT (version) DO NOTHING;
