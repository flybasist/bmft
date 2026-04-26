#!/bin/bash
# ============================================================================
# prepare-debian-host.sh — Подготовка Debian хоста для запуска BMFT бота
# ============================================================================
# Создаёт необходимые директории с правильными правами доступа
# Запускать на Debian хосте перед docker-compose up
# ============================================================================

set -e

echo "Preparing directories for BMFT bot..."

# Создаём структуру папок если её нет
mkdir -p data/logs
mkdir -p data/postgres
mkdir -p migrations

# Устанавливаем права доступа (UID/GID 1000 = пользователь bmft в контейнере)
chown -R 1000:1000 data/logs
chmod -R 755 data/logs

echo "Directories prepared:"
ls -la data/

echo ""
echo "Ready to start:"
echo "  docker-compose -f docker-compose.env.yaml up -d"
echo "  docker-compose -f docker-compose.bot.yaml up -d"
