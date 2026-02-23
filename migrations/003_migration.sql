-- ============================================================================
-- BMFT Migration: v1.1 → v1.1.1
-- ============================================================================
-- Anti-Spam & Admin Security hotfix.
-- Обновляет только версию в bot_settings — вся логика в Go-коде.
-- ============================================================================

UPDATE bot_settings SET bot_version = '1.1.1' WHERE id = 1;

-- Запись версии миграции
INSERT INTO schema_migrations (version, description)
VALUES (3, 'v1.1 to v1.1.1: anti-spam and admin security hotfix')
ON CONFLICT (version) DO NOTHING;
