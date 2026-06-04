-- migrations/005_migration.sql
-- Описание: Обновление версии бота до 1.2.1

-- 1. Обновляем версию в bot_settings
UPDATE bot_settings SET bot_version = '1.2.1' WHERE id = 1;

-- 2. Записываем версию миграции
INSERT INTO schema_migrations (version, description) 
VALUES (5, 'Update bot version to 1.2.1')
ON CONFLICT (version) DO NOTHING;
