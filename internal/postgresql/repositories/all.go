package repositories

import (
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ============================================================================
// ChatRepository - управление чатами
// ============================================================================

// ChatRepository управляет операциями с таблицей chats.
// Русский комментарий: Репозиторий для работы с чатами.
// Автоматически создаёт запись при первом сообщении, деактивирует удалённые чаты.
type ChatRepository struct {
	db *sql.DB
}

// NewChatRepository создаёт новый инстанс репозитория чатов.
func NewChatRepository(db *sql.DB) *ChatRepository {
	return &ChatRepository{db: db}
}

// GetOrCreate получает существующий чат или создаёт новую запись.
// Русский комментарий: Вызывается при каждом сообщении для гарантии, что чат есть в БД.
func (r *ChatRepository) GetOrCreate(chatID int64, chatType, title, username string) error {
	query := `
		INSERT INTO chats (chat_id, chat_type, title, username, is_active)
		VALUES ($1, $2, $3, $4, true)
		ON CONFLICT (chat_id) DO UPDATE
		SET
			chat_type = EXCLUDED.chat_type,
			title = EXCLUDED.title,
			username = EXCLUDED.username,
			is_active = true,
			updated_at = NOW()
	`
	_, err := r.db.Exec(query, chatID, chatType, title, username)
	if err != nil {
		return fmt.Errorf("failed to get or create chat: %w", err)
	}
	return nil
}

// ============================================================================
// EventRepository - логирование событий
// ============================================================================

// EventRepository управляет записью событий в таблицу event_log.
// Русский комментарий: Репозиторий для audit trail — все действия модулей логируются здесь.
type EventRepository struct {
	db *sql.DB
}

// NewEventRepository создаёт новый инстанс репозитория событий.
func NewEventRepository(db *sql.DB) *EventRepository {
	return &EventRepository{db: db}
}

// Log записывает событие в event_log.
// Русский комментарий: Каждое действие модуля (лимит превышен, реакция сработала, etc.)
// логируется для последующего анализа и отладки.
func (r *EventRepository) Log(chatID, userID int64, moduleName, eventType, details string) error {
	query := `
		INSERT INTO event_log (chat_id, user_id, module_name, event_type, details)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.Exec(query, chatID, userID, moduleName, eventType, details)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}
	return nil
}

// ============================================================================
// ModuleRepository - управление модулями
// ============================================================================

// ModuleRepository управляет операциями с таблицей chat_modules.
// Русский комментарий: Репозиторий для управления модулями в чатах.
// Проверяет включен ли модуль, включает/выключает, читает/пишет JSONB конфиг.
type ModuleRepository struct {
	db *sql.DB
}

// NewModuleRepository создаёт новый инстанс репозитория модулей.
func NewModuleRepository(db *sql.DB) *ModuleRepository {
	return &ModuleRepository{db: db}
}

// IsEnabled проверяет включен ли модуль для данного чата.
func (r *ModuleRepository) IsEnabled(chatID int64, moduleName string) (bool, error) {
	var isEnabled bool
	query := `SELECT is_enabled FROM chat_modules WHERE chat_id = $1 AND module_name = $2`
	err := r.db.QueryRow(query, chatID, moduleName).Scan(&isEnabled)
	if err == sql.ErrNoRows {
		return false, nil // Модуль не зарегистрирован для чата = выключен
	}
	if err != nil {
		return false, fmt.Errorf("failed to check module enabled: %w", err)
	}
	return isEnabled, nil
}

// Enable включает модуль для чата (создаёт запись или обновляет is_enabled = true).
func (r *ModuleRepository) Enable(chatID int64, moduleName string) error {
	query := `
		INSERT INTO chat_modules (chat_id, module_name, is_enabled)
		VALUES ($1, $2, true)
		ON CONFLICT (chat_id, module_name) DO UPDATE
		SET is_enabled = true, updated_at = NOW()
	`
	_, err := r.db.Exec(query, chatID, moduleName)
	if err != nil {
		return fmt.Errorf("failed to enable module: %w", err)
	}
	return nil
}

// Disable выключает модуль для чата (is_enabled = false).
func (r *ModuleRepository) Disable(chatID int64, moduleName string) error {
	query := `UPDATE chat_modules SET is_enabled = false, updated_at = NOW() WHERE chat_id = $1 AND module_name = $2`
	_, err := r.db.Exec(query, chatID, moduleName)
	if err != nil {
		return fmt.Errorf("failed to disable module: %w", err)
	}
	return nil
}

// ============================================================================
// SettingsRepository - глобальные настройки
// ============================================================================

// SettingsRepository управляет глобальными настройками бота
type SettingsRepository struct {
	db *sql.DB
}

// NewSettingsRepository создаёт новый репозиторий настроек
func NewSettingsRepository(db *sql.DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

// GetVersion получает версию бота из БД
func (r *SettingsRepository) GetVersion() (string, error) {
	var version string
	err := r.db.QueryRow(`
		SELECT bot_version FROM bot_settings WHERE id = 1
	`).Scan(&version)

	if err == sql.ErrNoRows {
		return "unknown", nil
	}
	if err != nil {
		return "", fmt.Errorf("get version: %w", err)
	}

	return version, nil
}

// ============================================================================
// UserMessageRepository - пользователи и сообщения
// ============================================================================

// UserMessageRepository реализует сохранение пользователей и сообщений
// в таблицы users и messages.
type UserMessageRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewUserMessageRepository(db *sql.DB, logger *zap.Logger) *UserMessageRepository {
	return &UserMessageRepository{db: db, logger: logger}
}

// UpsertUser сохраняет или обновляет пользователя в таблице users.
func (r *UserMessageRepository) UpsertUser(userID int64, username, firstName string) error {
	query := `
		INSERT INTO users (user_id, username, first_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			username = EXCLUDED.username,
			first_name = EXCLUDED.first_name
	`
	_, err := r.db.Exec(query, userID, username, firstName)
	if err != nil {
		r.logger.Error("failed to upsert user", zap.Int64("user_id", userID), zap.Error(err))
		return err
	}
	return nil
}

// InsertMessage сохраняет сообщение в таблице messages.
func (r *UserMessageRepository) InsertMessage(chatID, userID int64, messageID int, contentType string) error {
	query := `
		INSERT INTO messages (chat_id, user_id, message_id, content_type)
		VALUES ($1, $2, $3, $4)
	`
	_, err := r.db.Exec(query, chatID, userID, messageID, contentType)
	if err != nil {
		r.logger.Error("failed to insert message", zap.Int64("user_id", userID), zap.Error(err))
		return err
	}
	return nil
}

// ============================================================================
// VIPRepository - управление VIP пользователями
// ============================================================================

// VIPRepository управляет VIP пользователями
type VIPRepository struct {
	db *sql.DB
}

// NewVIPRepository создаёт новый репозиторий VIP
func NewVIPRepository(db *sql.DB) *VIPRepository {
	return &VIPRepository{
		db: db,
	}
}

// IsVIP проверяет является ли пользователь VIP в данном чате
func (r *VIPRepository) IsVIP(chatID, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM chat_vips 
			WHERE chat_id = $1 AND user_id = $2
		)
	`, chatID, userID).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("check VIP status: %w", err)
	}

	return exists, nil
}

// GrantVIP выдаёт VIP статус пользователю
func (r *VIPRepository) GrantVIP(chatID, userID, grantedBy int64, reason string) error {
	_, err := r.db.Exec(`
		INSERT INTO chat_vips (chat_id, user_id, granted_by, reason)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (chat_id, user_id) DO UPDATE
		SET granted_by = EXCLUDED.granted_by,
		    reason = EXCLUDED.reason,
		    granted_at = NOW()
	`, chatID, userID, grantedBy, reason)

	if err != nil {
		return fmt.Errorf("grant VIP: %w", err)
	}

	return nil
}

// RevokeVIP забирает VIP статус
func (r *VIPRepository) RevokeVIP(chatID, userID int64) error {
	result, err := r.db.Exec(`
		DELETE FROM chat_vips
		WHERE chat_id = $1 AND user_id = $2
	`, chatID, userID)

	if err != nil {
		return fmt.Errorf("revoke VIP: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user is not VIP")
	}

	return nil
}

// ListVIPs возвращает список всех VIP пользователей в чате
func (r *VIPRepository) ListVIPs(chatID int64) ([]VIPInfo, error) {
	rows, err := r.db.Query(`
		SELECT 
			cv.user_id,
			COALESCE(u.username, ''),
			COALESCE(u.first_name, ''),
			cv.granted_at,
			COALESCE(cv.reason, '')
		FROM chat_vips cv
		LEFT JOIN users u ON cv.user_id = u.user_id
		WHERE cv.chat_id = $1
		ORDER BY cv.granted_at DESC
	`, chatID)

	if err != nil {
		return nil, fmt.Errorf("list VIPs: %w", err)
	}
	defer rows.Close()

	var vips []VIPInfo
	for rows.Next() {
		var vip VIPInfo
		err := rows.Scan(
			&vip.UserID,
			&vip.Username,
			&vip.FirstName,
			&vip.GrantedAt,
			&vip.Reason,
		)
		if err != nil {
			continue // Пропускаем невалидные записи
		}
		vips = append(vips, vip)
	}

	return vips, nil
}

// VIPInfo содержит информацию о VIP пользователе
type VIPInfo struct {
	UserID    int64
	Username  string
	FirstName string
	GrantedAt string
	Reason    string
}

// ============================================================================
// ContentLimitsRepository - лимиты на контент
// ============================================================================

// ContentLimitsRepository управляет лимитами на контент
type ContentLimitsRepository struct {
	db *sql.DB
}

// NewContentLimitsRepository создаёт новый репозиторий лимитов
func NewContentLimitsRepository(db *sql.DB) *ContentLimitsRepository {
	return &ContentLimitsRepository{
		db: db,
	}
}

// ContentLimits представляет лимиты для чата/пользователя
type ContentLimits struct {
	ChatID           int64
	UserID           *int64 // nil = настройки для всех (allmembers)
	LimitText        int
	LimitPhoto       int
	LimitVideo       int
	LimitSticker     int
	LimitAnimation   int
	LimitVoice       int
	LimitVideoNote   int
	LimitAudio       int
	LimitDocument    int
	LimitLocation    int
	LimitContact     int
	LimitBannedWords int
	WarningThreshold int
}

// GetLimits получает лимиты для пользователя (или allmembers если не указан)
func (r *ContentLimitsRepository) GetLimits(chatID int64, userID *int64) (*ContentLimits, error) {
	var limits ContentLimits

	// Сначала ищем лимит для конкретного пользователя
	queryUser := `
		SELECT 
			chat_id, user_id,
			limit_text, limit_photo, limit_video, limit_sticker,
			limit_animation, limit_voice, limit_video_note, limit_audio,
			limit_document, limit_location, limit_contact, limit_banned_words,
			warning_threshold
		FROM content_limits
		WHERE chat_id = $1 AND user_id = $2
		LIMIT 1
	`

	err := r.db.QueryRow(queryUser, chatID, userID).Scan(
		&limits.ChatID, &limits.UserID,
		&limits.LimitText, &limits.LimitPhoto, &limits.LimitVideo, &limits.LimitSticker,
		&limits.LimitAnimation, &limits.LimitVoice, &limits.LimitVideoNote, &limits.LimitAudio,
		&limits.LimitDocument, &limits.LimitLocation, &limits.LimitContact, &limits.LimitBannedWords,
		&limits.WarningThreshold,
	)

	if err == sql.ErrNoRows {
		// Если нет лимита для пользователя, ищем лимит для всех (user_id = NULL)
		queryAll := `
			SELECT 
				chat_id, user_id,
				limit_text, limit_photo, limit_video, limit_sticker,
				limit_animation, limit_voice, limit_video_note, limit_audio,
				limit_document, limit_location, limit_contact, limit_banned_words,
				warning_threshold
			FROM content_limits
			WHERE chat_id = $1 AND user_id IS NULL
			LIMIT 1
		`
		errAll := r.db.QueryRow(queryAll, chatID).Scan(
			&limits.ChatID, &limits.UserID,
			&limits.LimitText, &limits.LimitPhoto, &limits.LimitVideo, &limits.LimitSticker,
			&limits.LimitAnimation, &limits.LimitVoice, &limits.LimitVideoNote, &limits.LimitAudio,
			&limits.LimitDocument, &limits.LimitLocation, &limits.LimitContact, &limits.LimitBannedWords,
			&limits.WarningThreshold,
		)
		if errAll == sql.ErrNoRows {
			// Нет лимитов вообще — возвращаем дефолтные (всё разрешено)
			return &ContentLimits{
				ChatID:           chatID,
				UserID:           userID,
				WarningThreshold: 2,
			}, nil
		}
		if errAll != nil {
			return nil, fmt.Errorf("get limits (all): %w", errAll)
		}
		return &limits, nil
	}

	if err != nil {
		return nil, fmt.Errorf("get limits: %w", err)
	}

	return &limits, nil
}

// GetLimitForContentType получает лимит для конкретного типа контента
func (r *ContentLimitsRepository) GetLimitForContentType(chatID int64, userID *int64, contentType string) (int, error) {
	limits, err := r.GetLimits(chatID, userID)
	if err != nil {
		return 0, err
	}

	// Мапим тип контента на поле
	switch contentType {
	case "text":
		return limits.LimitText, nil
	case "photo":
		return limits.LimitPhoto, nil
	case "video":
		return limits.LimitVideo, nil
	case "sticker":
		return limits.LimitSticker, nil
	case "animation":
		return limits.LimitAnimation, nil
	case "voice":
		return limits.LimitVoice, nil
	case "video_note":
		return limits.LimitVideoNote, nil
	case "audio":
		return limits.LimitAudio, nil
	case "document":
		return limits.LimitDocument, nil
	case "location":
		return limits.LimitLocation, nil
	case "contact":
		return limits.LimitContact, nil
	default:
		return 0, nil // нет лимита
	}
}

// SetLimit устанавливает лимит для типа контента
func (r *ContentLimitsRepository) SetLimit(chatID int64, userID *int64, contentType string, limit int) error {
	// Определяем какое поле обновлять
	var columnName string
	switch contentType {
	case "text":
		columnName = "limit_text"
	case "photo":
		columnName = "limit_photo"
	case "video":
		columnName = "limit_video"
	case "sticker":
		columnName = "limit_sticker"
	case "animation":
		columnName = "limit_animation"
	case "voice":
		columnName = "limit_voice"
	case "video_note":
		columnName = "limit_video_note"
	case "audio":
		columnName = "limit_audio"
	case "document":
		columnName = "limit_document"
	case "location":
		columnName = "limit_location"
	case "contact":
		columnName = "limit_contact"
	case "banned_words":
		columnName = "limit_banned_words"
	default:
		return fmt.Errorf("unknown content type: %s", contentType)
	}

	// Upsert лимита
	query := fmt.Sprintf(`
		INSERT INTO content_limits (chat_id, user_id, %s)
		VALUES ($1, $2, $3)
		ON CONFLICT (chat_id, COALESCE(user_id, -1))
		DO UPDATE SET %s = EXCLUDED.%s, updated_at = NOW()
	`, columnName, columnName, columnName)

	_, err := r.db.Exec(query, chatID, userID, limit)
	if err != nil {
		return fmt.Errorf("set limit: %w", err)
	}

	return nil
}

// GetCounter получает счётчик контента за сегодня
func (r *ContentLimitsRepository) GetCounter(chatID, userID int64, contentType string) (int, error) {
	today := time.Now().Format("2006-01-02")

	var columnName string
	switch contentType {
	case "text":
		columnName = "count_text"
	case "photo":
		columnName = "count_photo"
	case "video":
		columnName = "count_video"
	case "sticker":
		columnName = "count_sticker"
	case "animation":
		columnName = "count_animation"
	case "voice":
		columnName = "count_voice"
	case "video_note":
		columnName = "count_video_note"
	case "audio":
		columnName = "count_audio"
	case "document":
		columnName = "count_document"
	case "location":
		columnName = "count_location"
	case "contact":
		columnName = "count_contact"
	case "banned_words":
		columnName = "count_banned_words"
	default:
		return 0, nil
	}

	query := fmt.Sprintf(`
		SELECT %s FROM content_counters
		WHERE chat_id = $1 AND user_id = $2 AND counter_date = $3
	`, columnName)

	var count int
	err := r.db.QueryRow(query, chatID, userID, today).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get counter: %w", err)
	}

	return count, nil
}

// IncrementCounter увеличивает счётчик контента
func (r *ContentLimitsRepository) IncrementCounter(chatID, userID int64, contentType string) error {
	today := time.Now().Format("2006-01-02")

	var columnName string
	switch contentType {
	case "text":
		columnName = "count_text"
	case "photo":
		columnName = "count_photo"
	case "video":
		columnName = "count_video"
	case "sticker":
		columnName = "count_sticker"
	case "animation":
		columnName = "count_animation"
	case "voice":
		columnName = "count_voice"
	case "video_note":
		columnName = "count_video_note"
	case "audio":
		columnName = "count_audio"
	case "document":
		columnName = "count_document"
	case "location":
		columnName = "count_location"
	case "contact":
		columnName = "count_contact"
	case "banned_words":
		columnName = "count_banned_words"
	default:
		return nil
	}

	query := fmt.Sprintf(`
		INSERT INTO content_counters (chat_id, user_id, counter_date, %s)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (chat_id, user_id, counter_date)
		DO UPDATE SET %s = content_counters.%s + 1, updated_at = NOW()
	`, columnName, columnName, columnName)

	_, err := r.db.Exec(query, chatID, userID, today)
	if err != nil {
		return fmt.Errorf("increment counter: %w", err)
	}

	return nil
}

// ============================================================================
// StatisticsRepository - статистика
// ============================================================================

// StatisticsRepository управляет статистикой пользователей и чатов.
// Использует таблицу content_counters для агрегации данных по типам контента.
// В v0.6.0 убрали дублирующую таблицу statistics_daily - вся статистика через content_counters.
type StatisticsRepository struct {
	db *sql.DB
}

// NewStatisticsRepository создаёт новый экземпляр репозитория статистики.
func NewStatisticsRepository(db *sql.DB) *StatisticsRepository {
	return &StatisticsRepository{
		db: db,
	}
}

// UserDailyStats представляет статистику пользователя за день.
type UserDailyStats struct {
	ChatID       int64
	UserID       int64
	Username     string
	Date         time.Time
	TextCount    int
	PhotoCount   int
	VideoCount   int
	StickerCount int
	VoiceCount   int
	OtherCount   int
	TotalCount   int
}

// ChatDailyStats представляет статистику чата за день.
type ChatDailyStats struct {
	ChatID       int64
	Date         time.Time
	TextCount    int
	PhotoCount   int
	VideoCount   int
	StickerCount int
	VoiceCount   int
	OtherCount   int
	TotalCount   int
	UserCount    int
}

// TopUser представляет пользователя в топе активности.
type TopUser struct {
	UserID       int64
	Username     string
	FirstName    string
	MessageCount int
	Rank         int
}

// IncrementCounter увеличивает счётчик сообщений для пользователя.
// Использует таблицу content_counters с отдельными полями для каждого типа контента.
// Вызывается при каждом сообщении, использует ON CONFLICT для атомарного инкремента.
func (r *StatisticsRepository) IncrementCounter(chatID, userID int64, contentType string) error {
	// Определяем какое поле инкрементировать
	var column string
	switch contentType {
	case "text":
		column = "count_text"
	case "photo":
		column = "count_photo"
	case "video":
		column = "count_video"
	case "sticker":
		column = "count_sticker"
	case "animation":
		column = "count_animation"
	case "voice":
		column = "count_voice"
	case "video_note":
		column = "count_video_note"
	case "audio":
		column = "count_audio"
	case "document":
		column = "count_document"
	case "location":
		column = "count_location"
	case "contact":
		column = "count_contact"
	default:
		// Неизвестный тип контента - пропускаем
		return nil
	}

	query := fmt.Sprintf(`
		INSERT INTO content_counters (chat_id, user_id, counter_date, %s, updated_at)
		VALUES ($1, $2, CURRENT_DATE, 1, NOW())
		ON CONFLICT (chat_id, user_id, counter_date)
		DO UPDATE SET 
			%s = content_counters.%s + 1,
			updated_at = NOW()
	`, column, column, column)

	_, err := r.db.Exec(query, chatID, userID)
	if err != nil {
		return fmt.Errorf("increment counter: %w", err)
	}

	return nil
}

// GetUserStats возвращает статистику пользователя за указанный день.
// Читает из content_counters, где каждый тип контента хранится в отдельном поле.
func (r *StatisticsRepository) GetUserStats(userID, chatID int64, date time.Time) (*UserDailyStats, error) {
	query := `
		SELECT 
			c.chat_id,
			c.user_id,
			COALESCE(u.username, '') as username,
			c.counter_date,
			c.count_text,
			c.count_photo,
			c.count_video,
			c.count_sticker,
			c.count_voice,
			(c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) as other_count,
			(c.count_text + c.count_photo + c.count_video + c.count_sticker + c.count_voice + 
			 c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) as total_count
		FROM content_counters c
		LEFT JOIN users u ON c.user_id = u.user_id
		WHERE c.user_id = $1 AND c.chat_id = $2 AND c.counter_date = $3
	`

	stats := &UserDailyStats{}
	err := r.db.QueryRow(query, userID, chatID, date).Scan(
		&stats.ChatID,
		&stats.UserID,
		&stats.Username,
		&stats.Date,
		&stats.TextCount,
		&stats.PhotoCount,
		&stats.VideoCount,
		&stats.StickerCount,
		&stats.VoiceCount,
		&stats.OtherCount,
		&stats.TotalCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Нет данных — это ОК
	}
	if err != nil {
		return nil, fmt.Errorf("get user stats: %w", err)
	}

	return stats, nil
}

// GetChatStats возвращает статистику всего чата за указанный день.
// Агрегирует данные по всем пользователям чата из content_counters.
func (r *StatisticsRepository) GetChatStats(chatID int64, date time.Time) (*ChatDailyStats, error) {
	query := `
		SELECT 
			chat_id,
			counter_date,
			SUM(count_text) as text_count,
			SUM(count_photo) as photo_count,
			SUM(count_video) as video_count,
			SUM(count_sticker) as sticker_count,
			SUM(count_voice) as voice_count,
			SUM(count_animation + count_video_note + count_audio + count_document + count_location + count_contact) as other_count,
			SUM(count_text + count_photo + count_video + count_sticker + count_voice + 
			    count_animation + count_video_note + count_audio + count_document + count_location + count_contact) as total_count,
			COUNT(DISTINCT user_id) as user_count
		FROM content_counters
		WHERE chat_id = $1 AND counter_date = $2
		GROUP BY chat_id, counter_date
	`

	stats := &ChatDailyStats{}
	err := r.db.QueryRow(query, chatID, date).Scan(
		&stats.ChatID,
		&stats.Date,
		&stats.TextCount,
		&stats.PhotoCount,
		&stats.VideoCount,
		&stats.StickerCount,
		&stats.VoiceCount,
		&stats.OtherCount,
		&stats.TotalCount,
		&stats.UserCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Нет данных — это ОК
	}
	if err != nil {
		return nil, fmt.Errorf("get chat stats: %w", err)
	}

	return stats, nil
}

// GetTopUsers возвращает топ активных пользователей чата за указанный день.
// Сортирует пользователей по общему количеству сообщений из content_counters.
func (r *StatisticsRepository) GetTopUsers(chatID int64, date time.Time, limit int) ([]TopUser, error) {
	query := `
		SELECT 
			c.user_id,
			COALESCE(u.username, '') as username,
			COALESCE(u.first_name, 'Unknown') as first_name,
			(c.count_text + c.count_photo + c.count_video + c.count_sticker + c.count_voice + 
			 c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) as message_count,
			ROW_NUMBER() OVER (ORDER BY (c.count_text + c.count_photo + c.count_video + c.count_sticker + c.count_voice + 
			                              c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) DESC) as rank
		FROM content_counters c
		LEFT JOIN users u ON c.user_id = u.user_id
		WHERE c.chat_id = $1 AND c.counter_date = $2
		ORDER BY message_count DESC
		LIMIT $3
	`

	rows, err := r.db.Query(query, chatID, date, limit)
	if err != nil {
		return nil, fmt.Errorf("get top users: %w", err)
	}
	defer rows.Close()

	var topUsers []TopUser
	for rows.Next() {
		var user TopUser
		err := rows.Scan(
			&user.UserID,
			&user.Username,
			&user.FirstName,
			&user.MessageCount,
			&user.Rank,
		)
		if err != nil {
			continue
		}
		topUsers = append(topUsers, user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate top users: %w", err)
	}

	return topUsers, nil
}

// GetUserWeeklyStats возвращает статистику пользователя за последние 7 дней.
// Агрегирует данные за неделю для отображения тренда активности.
func (r *StatisticsRepository) GetUserWeeklyStats(userID, chatID int64) (*UserDailyStats, error) {
	weekAgo := time.Now().AddDate(0, 0, -7)

	query := `
		SELECT 
			c.chat_id,
			c.user_id,
			COALESCE(u.username, '') as username,
			NOW()::date as stat_date,
			SUM(c.count_text) as text_count,
			SUM(c.count_photo) as photo_count,
			SUM(c.count_video) as video_count,
			SUM(c.count_sticker) as sticker_count,
			SUM(c.count_voice) as voice_count,
			SUM(c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) as other_count,
			SUM(c.count_text + c.count_photo + c.count_video + c.count_sticker + c.count_voice + 
			    c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) as total_count
		FROM content_counters c
		LEFT JOIN users u ON c.user_id = u.user_id
		WHERE c.user_id = $1 AND c.chat_id = $2 AND c.counter_date >= $3
		GROUP BY c.chat_id, c.user_id, u.username
	`

	stats := &UserDailyStats{}
	err := r.db.QueryRow(query, userID, chatID, weekAgo).Scan(
		&stats.ChatID,
		&stats.UserID,
		&stats.Username,
		&stats.Date,
		&stats.TextCount,
		&stats.PhotoCount,
		&stats.VideoCount,
		&stats.StickerCount,
		&stats.VoiceCount,
		&stats.OtherCount,
		&stats.TotalCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Нет данных — это ОК
	}
	if err != nil {
		return nil, fmt.Errorf("get user weekly stats: %w", err)
	}

	return stats, nil
}

// ============================================================================
// SchedulerRepository - планировщик задач
// ============================================================================

// SchedulerRepository управляет операциями с таблицей scheduled_tasks.
// Русский комментарий: Репозиторий для работы с задачами планировщика.
// Создаёт, читает, удаляет задачи. Отслеживает последний запуск.
type SchedulerRepository struct {
	db *sql.DB
}

// NewSchedulerRepository создаёт новый инстанс репозитория планировщика.
func NewSchedulerRepository(db *sql.DB) *SchedulerRepository {
	return &SchedulerRepository{
		db: db,
	}
}

// ScheduledTask представляет задачу планировщика.
type ScheduledTask struct {
	ID        int64
	ChatID    int64
	TaskName  string
	CronExpr  string
	TaskType  string // sticker, text, photo
	TaskData  string // file_id для sticker, текст для text, file_id для photo
	IsActive  bool
	LastRun   *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateTask создаёт новую задачу планировщика.
func (r *SchedulerRepository) CreateTask(chatID int64, taskName, cronExpr, taskType, taskData string) (int64, error) {
	query := `
		INSERT INTO scheduled_tasks (chat_id, task_name, cron_expression, action_type, action_data, is_active)
		VALUES ($1, $2, $3, $4, $5, true)
		RETURNING id
	`
	var taskID int64
	err := r.db.QueryRow(query, chatID, taskName, cronExpr, taskType, taskData).Scan(&taskID)
	if err != nil {
		return 0, fmt.Errorf("failed to create scheduled task: %w", err)
	}

	return taskID, nil
}

// GetTask получает задачу по ID.
func (r *SchedulerRepository) GetTask(taskID int64) (*ScheduledTask, error) {
	query := `
		SELECT id, chat_id, task_name, cron_expression, action_type, action_data, is_active, last_run, created_at, updated_at
		FROM scheduled_tasks
		WHERE id = $1
	`
	task := &ScheduledTask{}
	err := r.db.QueryRow(query, taskID).Scan(
		&task.ID, &task.ChatID, &task.TaskName, &task.CronExpr,
		&task.TaskType, &task.TaskData, &task.IsActive, &task.LastRun,
		&task.CreatedAt, &task.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	return task, nil
}

// GetChatTasks получает все задачи для чата.
func (r *SchedulerRepository) GetChatTasks(chatID int64) ([]*ScheduledTask, error) {
	query := `
		SELECT id, chat_id, task_name, cron_expression, action_type, action_data, is_active, last_run, created_at, updated_at
		FROM scheduled_tasks
		WHERE chat_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(query, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*ScheduledTask
	for rows.Next() {
		task := &ScheduledTask{}
		err := rows.Scan(
			&task.ID, &task.ChatID, &task.TaskName, &task.CronExpr,
			&task.TaskType, &task.TaskData, &task.IsActive, &task.LastRun,
			&task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			continue
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetActiveTasks получает все активные задачи.
func (r *SchedulerRepository) GetActiveTasks() ([]*ScheduledTask, error) {
	query := `
		SELECT id, chat_id, task_name, cron_expression, action_type, action_data, is_active, last_run, created_at, updated_at
		FROM scheduled_tasks
		WHERE is_active = true
		ORDER BY chat_id, created_at
	`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*ScheduledTask
	for rows.Next() {
		task := &ScheduledTask{}
		err := rows.Scan(
			&task.ID, &task.ChatID, &task.TaskName, &task.CronExpr,
			&task.TaskType, &task.TaskData, &task.IsActive, &task.LastRun,
			&task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			continue
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// UpdateLastRun обновляет время последнего запуска задачи.
func (r *SchedulerRepository) UpdateLastRun(taskID int64, lastRun time.Time) error {
	query := `UPDATE scheduled_tasks SET last_run = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(query, lastRun, taskID)
	if err != nil {
		return fmt.Errorf("failed to update last run: %w", err)
	}
	return nil
}

// DeleteTask удаляет задачу.
func (r *SchedulerRepository) DeleteTask(taskID int64) error {
	query := `DELETE FROM scheduled_tasks WHERE id = $1`
	result, err := r.db.Exec(query, taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("task not found")
	}

	return nil
}
