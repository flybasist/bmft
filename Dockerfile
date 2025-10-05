# ============================================================================
# Multi-stage Dockerfile для BMFT бота
# ============================================================================
# Stage 1: Builder - компиляция Go бинарника
# Stage 2: Runtime  - минимальный Alpine образ с только необходимым
# ============================================================================

# ============================================================================
# Stage 1: Builder
# ============================================================================
FROM golang:1.25.1-alpine AS builder

# Метаданные
LABEL maintainer="flybasist"
LABEL description="BMFT Bot Builder Stage"

# Рабочая директория внутри контейнера
WORKDIR /build

# Установка зависимостей для сборки (если нужны C-библиотеки)
RUN apk add --no-cache git ca-certificates tzdata

# Копируем go.mod и go.sum для кеширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь исходный код
COPY . .

# Сборка статического бинарника (CGO_ENABLED=0 для полной статичности)
# -ldflags="-s -w" уменьшает размер бинарника (убирает debug info)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -a -installsuffix cgo \
    -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty) -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o bot cmd/bot/main.go

# ============================================================================
# Stage 2: Runtime
# ============================================================================
FROM alpine:latest

# Метаданные
LABEL maintainer="flybasist"
LABEL description="BMFT Bot - Modular Telegram Bot Framework"
LABEL version="0.6.0"

# Установка CA сертификатов и timezone data (для TLS и правильного времени)
RUN apk --no-cache add ca-certificates tzdata

# Создаём непривилегированного пользователя для безопасности
RUN addgroup -g 1000 bmft && \
    adduser -D -u 1000 -G bmft bmft

# Рабочая директория
WORKDIR /app

# Копируем бинарник из builder stage
COPY --from=builder --chown=bmft:bmft /build/bot /app/bot

# Устанавливаем timezone (опционально, можно переопределить через env)
ENV TZ=Asia/Almaty

# Переключаемся на непривилегированного пользователя
USER bmft

# Healthcheck для Kubernetes/Docker Compose
# Проверяем, что бот отвечает на metrics endpoint (:9090/healthz)
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:9090/healthz || exit 1

# Expose порт для метрик
EXPOSE 9090

# Запуск бота
ENTRYPOINT ["/app/bot"]
