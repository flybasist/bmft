package reactions

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// ReactionsModule реализует модуль автоматических реакций на сообщения
// Русский комментарий: Модуль для реакций на ключевые слова по regex паттернам
// Аналог Python бота: rts_bot/checkmessage.py + rts_bot/reaction.py
type ReactionsModule struct {
	db         *sql.DB
	logger     *zap.Logger
	moduleRepo *repositories.ModuleRepository
	eventRepo  *repositories.EventRepository
	adminUsers []int64
}

// ReactionConfig хранит настройки одной реакции
type ReactionConfig struct {
	ID              int64
	ChatID          int64
	ContentType     string // "text", "sticker", "photo", etc.
	TriggerType     string // "regex", "exact", "contains"
	TriggerPattern  string // regex или текст для поиска
	ReactionType    string // "text", "sticker", "delete", "mute"
	ReactionData    string // текст ответа или file_id стикера
	ViolationCode   int    // код нарушения для статистики
	CooldownMinutes int    // антифлуд: сколько минут между реакциями
	IsEnabled       bool
	IsVIP           bool // VIP пользователи игнорируют cooldown
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// New создаёт новый инстанс модуля реакций
func New(db *sql.DB, moduleRepo *repositories.ModuleRepository, eventRepo *repositories.EventRepository, logger *zap.Logger) *ReactionsModule {
	return &ReactionsModule{
		db:         db,
		logger:     logger,
		moduleRepo: moduleRepo,
		eventRepo:  eventRepo,
		adminUsers: []int64{},
	}
}

// Name возвращает имя модуля
func (m *ReactionsModule) Name() string {
	return "reactions"
}

// Init инициализирует модуль
func (m *ReactionsModule) Init(deps core.ModuleDependencies) error {
	m.logger.Info("reactions module initialized")
	return nil
}

// Commands возвращает список команд модуля (публичных команд нет)
func (m *ReactionsModule) Commands() []core.BotCommand {
	return []core.BotCommand{} // Все команды reactions — админские, см. RegisterAdminCommands
}

// Enabled проверяет включен ли модуль для чата
func (m *ReactionsModule) Enabled(chatID int64) (bool, error) {
	return m.moduleRepo.IsEnabled(chatID, m.Name())
}

// OnMessage обрабатывает входящее сообщение
// Русский комментарий: Проверяет сообщение на совпадение с regex паттернами
// Аналог Python: checkmessage.regextext()
func (m *ReactionsModule) OnMessage(ctx *core.MessageContext) error {
	// Пропускаем команды бота
	if strings.HasPrefix(ctx.Message.Text, "/") {
		return nil
	}

	// Получаем конфигурацию реакций для этого чата
	reactions, err := m.getReactions(ctx.Chat.ID)
	if err != nil {
		m.logger.Error("failed to get reactions config",
			zap.Int64("chat_id", ctx.Chat.ID),
			zap.Error(err),
		)
		return err
	}

	// Проверяем каждую реакцию
	for _, reaction := range reactions {
		if !reaction.IsEnabled {
			continue
		}

		// Получаем текст для проверки (text или caption)
		textToCheck := m.getTextFromMessage(ctx.Message)
		if textToCheck == "" {
			continue
		}

		// Проверяем паттерн
		matched, err := m.checkPattern(textToCheck, reaction)
		if err != nil {
			m.logger.Error("failed to check pattern",
				zap.String("pattern", reaction.TriggerPattern),
				zap.Error(err),
			)
			continue
		}

		if !matched {
			continue
		}

		// Проверяем cooldown (антифлуд)
		if m.shouldSkipDueToCooldown(ctx, reaction) {
			continue
		}

		// Выполняем реакцию
		if err := m.executeReaction(ctx, reaction); err != nil {
			m.logger.Error("failed to execute reaction",
				zap.Int64("reaction_id", reaction.ID),
				zap.Error(err),
			)
			continue
		}

		// Логируем событие
		_ = m.eventRepo.Log(
			ctx.Chat.ID,
			ctx.Message.Sender.ID,
			m.Name(),
			"reaction_triggered",
			fmt.Sprintf("Reaction #%d triggered by pattern: %s", reaction.ID, reaction.TriggerPattern),
		)

		// Записываем в reactions_log
		if err := m.logReaction(ctx.Chat.ID, ctx.Message.Sender.ID, reaction.ID); err != nil {
			m.logger.Warn("failed to log reaction", zap.Error(err))
		}

		// Одна реакция на сообщение
		break
	}

	return nil
}

// Shutdown корректно завершает работу модуля
func (m *ReactionsModule) Shutdown() error {
	m.logger.Info("reactions module shutdown")
	return nil
}

// getReactions получает все активные реакции для чата
func (m *ReactionsModule) getReactions(chatID int64) ([]ReactionConfig, error) {
	query := `
		SELECT id, chat_id, content_type, trigger_type, trigger_pattern,
		       reaction_type, reaction_data, violation_code, cooldown_minutes,
		       is_enabled, is_vip, created_at, updated_at
		FROM reactions_config
		WHERE chat_id = $1 AND is_enabled = true
		ORDER BY id ASC
	`

	rows, err := m.db.Query(query, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to query reactions: %w", err)
	}
	defer rows.Close()

	var reactions []ReactionConfig
	for rows.Next() {
		var r ReactionConfig
		err := rows.Scan(
			&r.ID, &r.ChatID, &r.ContentType, &r.TriggerType, &r.TriggerPattern,
			&r.ReactionType, &r.ReactionData, &r.ViolationCode, &r.CooldownMinutes,
			&r.IsEnabled, &r.IsVIP, &r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan reaction: %w", err)
		}
		reactions = append(reactions, r)
	}

	return reactions, rows.Err()
}

// getTextFromMessage извлекает текст из сообщения (text или caption)
func (m *ReactionsModule) getTextFromMessage(msg *tele.Message) string {
	if msg.Text != "" {
		return strings.ToLower(msg.Text)
	}
	if msg.Caption != "" {
		return strings.ToLower(msg.Caption)
	}
	return ""
}

// checkPattern проверяет совпадение текста с паттерном
// Русский комментарий: Аналог Python re.search(pattern, text.lower())
func (m *ReactionsModule) checkPattern(text string, reaction ReactionConfig) (bool, error) {
	switch reaction.TriggerType {
	case "regex":
		regex, err := regexp.Compile(reaction.TriggerPattern)
		if err != nil {
			return false, fmt.Errorf("invalid regex pattern: %w", err)
		}
		return regex.MatchString(text), nil

	case "exact":
		return text == strings.ToLower(reaction.TriggerPattern), nil

	case "contains":
		return strings.Contains(text, strings.ToLower(reaction.TriggerPattern)), nil

	default:
		return false, fmt.Errorf("unknown trigger type: %s", reaction.TriggerType)
	}
}

// shouldSkipDueToCooldown проверяет нужно ли пропустить реакцию из-за cooldown
// Русский комментарий: Аналог Python db.basecounttext(delta="deltahour_message")
func (m *ReactionsModule) shouldSkipDueToCooldown(ctx *core.MessageContext, reaction ReactionConfig) bool {
	// VIP пользователи игнорируют cooldown
	if reaction.IsVIP {
		return false
	}

	// Если cooldown = 0, не проверяем
	if reaction.CooldownMinutes == 0 {
		return false
	}

	// Проверяем было ли срабатывание этой реакции недавно
	query := `
		SELECT COUNT(*)
		FROM reactions_log
		WHERE chat_id = $1 
		  AND reaction_id = $2
		  AND created_at > NOW() - INTERVAL '1 minute' * $3
	`

	var count int
	err := m.db.QueryRow(query, ctx.Chat.ID, reaction.ID, reaction.CooldownMinutes).Scan(&count)
	if err != nil {
		m.logger.Warn("failed to check cooldown", zap.Error(err))
		return false
	}

	return count > 0
}

// executeReaction выполняет реакцию (отправка текста/стикера/удаление)
// Русский комментарий: Аналог Python modesend() + deletemessage()
func (m *ReactionsModule) executeReaction(ctx *core.MessageContext, reaction ReactionConfig) error {
	switch reaction.ReactionType {
	case "text":
		_, err := ctx.Bot.Send(ctx.Chat, reaction.ReactionData)
		return err

	case "sticker":
		sticker := &tele.Sticker{File: tele.File{FileID: reaction.ReactionData}}
		_, err := ctx.Bot.Send(ctx.Chat, sticker)
		return err

	case "delete":
		return ctx.Bot.Delete(ctx.Message)

	default:
		return fmt.Errorf("unknown reaction type: %s", reaction.ReactionType)
	}
}

// logReaction записывает срабатывание реакции в reactions_log
func (m *ReactionsModule) logReaction(chatID, userID, reactionID int64) error {
	query := `
		INSERT INTO reactions_log (chat_id, user_id, reaction_id, triggered_at)
		VALUES ($1, $2, $3, NOW())
	`
	_, err := m.db.Exec(query, chatID, userID, reactionID)
	if err != nil {
		return fmt.Errorf("failed to log reaction: %w", err)
	}
	return nil
}

// RegisterCommands регистрирует публичные команды модуля
func (m *ReactionsModule) RegisterCommands(bot *tele.Bot) {
	// У reactions модуля нет публичных команд
	// Все команды admin-only и регистрируются в RegisterAdminCommands
}

// RegisterAdminCommands регистрирует админские команды модуля
func (m *ReactionsModule) RegisterAdminCommands(bot *tele.Bot) {
	bot.Handle("/addreaction", m.handleAddReaction)
	bot.Handle("/listreactions", m.handleListReactions)
	bot.Handle("/delreaction", m.handleDeleteReaction)
	bot.Handle("/testreaction", m.handleTestReaction)
}

// handleAddReaction добавляет новую реакцию
// Формат: /addreaction <contentType> <triggerType> <pattern> <reactionType> <data> [cooldown]
// Пример: /addreaction text regex (?i)привет text "Здравствуй!" 10
func (m *ReactionsModule) handleAddReaction(c tele.Context) error {
	// Проверка прав админа
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Команда доступна только администраторам")
	}

	// Проверка что команда в группе
	if c.Chat().Type != tele.ChatGroup && c.Chat().Type != tele.ChatSuperGroup {
		return c.Send("❌ Команда работает только в группах")
	}

	args := strings.Fields(c.Text())
	if len(args) < 6 {
		return c.Send(
			"📖 *Использование:*\n"+
				"`/addreaction <contentType> <triggerType> <pattern> <reactionType> <data> [cooldown]`\n\n"+
				"*contentType:* text, photo, video, document, sticker, voice\n"+
				"*triggerType:* regex, exact, contains\n"+
				"*pattern:* regex выражение или текст для поиска\n"+
				"*reactionType:* text, sticker, delete\n"+
				"*data:* текст ответа или file_id стикера (для delete пусто)\n"+
				"*cooldown:* минуты между реакциями (по умолчанию 10)\n\n"+
				"*Примеры:*\n"+
				"`/addreaction text regex (?i)привет text \"Здравствуй!\" 10`\n"+
				"`/addreaction text contains спам delete \"\" 5`\n"+
				"`/addreaction photo exact test sticker CAACAgIAAxkBAAIC... 0`",
			&tele.SendOptions{ParseMode: tele.ModeMarkdown},
		)
	}

	contentType := args[1]
	triggerType := args[2]
	pattern := args[3]
	reactionType := args[4]
	reactionData := args[5]
	cooldown := 10 // По умолчанию 10 минут

	// Парсим cooldown если указан
	if len(args) >= 7 {
		var err error
		cooldown, err = strconv.Atoi(args[6])
		if err != nil || cooldown < 0 {
			return c.Send("❌ Неверный cooldown (должно быть целое число >= 0)")
		}
	}

	// Валидация contentType
	validContentTypes := map[string]bool{
		"text": true, "photo": true, "video": true,
		"document": true, "sticker": true, "voice": true,
	}
	if !validContentTypes[contentType] {
		return c.Send("❌ Неверный contentType. Допустимые: text, photo, video, document, sticker, voice")
	}

	// Валидация triggerType
	validTriggerTypes := map[string]bool{"regex": true, "exact": true, "contains": true}
	if !validTriggerTypes[triggerType] {
		return c.Send("❌ Неверный triggerType. Допустимые: regex, exact, contains")
	}

	// Валидация reactionType
	validReactionTypes := map[string]bool{"text": true, "sticker": true, "delete": true}
	if !validReactionTypes[reactionType] {
		return c.Send("❌ Неверный reactionType. Допустимые: text, sticker, delete")
	}

	// Проверка regex если triggerType = regex
	if triggerType == "regex" {
		if _, err := regexp.Compile(pattern); err != nil {
			return c.Send(fmt.Sprintf("❌ Неверный regex паттерн: %v", err))
		}
	}

	// Добавляем реакцию в БД
	query := `
		INSERT INTO reactions_config 
		(chat_id, content_type, trigger_type, trigger_pattern, reaction_type, reaction_data, cooldown_minutes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`
	var reactionID int64
	err := m.db.QueryRow(query, c.Chat().ID, contentType, triggerType, pattern, reactionType, reactionData, cooldown).Scan(&reactionID)
	if err != nil {
		m.logger.Error("failed to add reaction",
			zap.Int64("chat_id", c.Chat().ID),
			zap.Error(err),
		)
		return c.Send("❌ Не удалось добавить реакцию")
	}

	// Логируем событие
	_ = m.eventRepo.Log(c.Chat().ID, c.Sender().ID, "reactions", "add_reaction",
		fmt.Sprintf("Added reaction #%d: %s/%s/%s", reactionID, contentType, triggerType, pattern))

	return c.Send(
		fmt.Sprintf("✅ Реакция добавлена!\n\n*ID:* `%d`\n*Content:* %s\n*Trigger:* %s\n*Pattern:* `%s`\n*Reaction:* %s\n*Cooldown:* %d мин",
			reactionID, contentType, triggerType, pattern, reactionType, cooldown),
		&tele.SendOptions{ParseMode: tele.ModeMarkdown},
	)
}

// handleListReactions показывает список всех реакций для чата
func (m *ReactionsModule) handleListReactions(c tele.Context) error {
	// Проверка прав админа
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Команда доступна только администраторам")
	}

	// Проверка что команда в группе
	if c.Chat().Type != tele.ChatGroup && c.Chat().Type != tele.ChatSuperGroup {
		return c.Send("❌ Команда работает только в группах")
	}

	query := `
		SELECT id, content_type, trigger_type, trigger_pattern, reaction_type, 
		       reaction_data, cooldown_minutes, is_enabled
		FROM reactions_config
		WHERE chat_id = $1
		ORDER BY id
	`
	rows, err := m.db.Query(query, c.Chat().ID)
	if err != nil {
		m.logger.Error("failed to list reactions",
			zap.Int64("chat_id", c.Chat().ID),
			zap.Error(err),
		)
		return c.Send("❌ Не удалось получить список реакций")
	}
	defer rows.Close()

	var reactions []string
	count := 0
	for rows.Next() {
		var r ReactionConfig
		if err := rows.Scan(&r.ID, &r.ContentType, &r.TriggerType, &r.TriggerPattern,
			&r.ReactionType, &r.ReactionData, &r.CooldownMinutes, &r.IsEnabled); err != nil {
			m.logger.Error("failed to scan reaction", zap.Error(err))
			continue
		}

		status := "✅"
		if !r.IsEnabled {
			status = "❌"
		}

		dataPreview := r.ReactionData
		if len(dataPreview) > 30 {
			dataPreview = dataPreview[:30] + "..."
		}

		reactions = append(reactions, fmt.Sprintf(
			"%s *#%d* | %s/%s | `%s` → %s (%dm)",
			status, r.ID, r.ContentType, r.TriggerType, r.TriggerPattern, r.ReactionType, r.CooldownMinutes,
		))
		count++
	}

	if count == 0 {
		return c.Send("📋 Реакций пока нет. Используйте `/addreaction` для добавления.",
			&tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	text := fmt.Sprintf("📋 *Реакции чата (%d):*\n\n%s\n\n💡 Для удаления: `/delreaction <id>`",
		count, strings.Join(reactions, "\n"))

	return c.Send(text, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// handleDeleteReaction удаляет реакцию по ID
func (m *ReactionsModule) handleDeleteReaction(c tele.Context) error {
	// Проверка прав админа
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Команда доступна только администраторам")
	}

	// Проверка что команда в группе
	if c.Chat().Type != tele.ChatGroup && c.Chat().Type != tele.ChatSuperGroup {
		return c.Send("❌ Команда работает только в группах")
	}

	args := strings.Fields(c.Text())
	if len(args) != 2 {
		return c.Send("📖 *Использование:*\n`/delreaction <id>`\n\n*Пример:*\n`/delreaction 5`",
			&tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	reactionID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return c.Send("❌ Неверный ID реакции")
	}

	// Проверяем что реакция принадлежит этому чату
	query := `DELETE FROM reactions_config WHERE id = $1 AND chat_id = $2`
	result, err := m.db.Exec(query, reactionID, c.Chat().ID)
	if err != nil {
		m.logger.Error("failed to delete reaction",
			zap.Int64("reaction_id", reactionID),
			zap.Int64("chat_id", c.Chat().ID),
			zap.Error(err),
		)
		return c.Send("❌ Не удалось удалить реакцию")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return c.Send("❌ Реакция не найдена или не принадлежит этому чату")
	}

	// Логируем событие
	_ = m.eventRepo.Log(c.Chat().ID, c.Sender().ID, "reactions", "delete_reaction",
		fmt.Sprintf("Deleted reaction #%d", reactionID))

	return c.Send(fmt.Sprintf("✅ Реакция #%d удалена", reactionID))
}

// handleTestReaction тестирует regex паттерн на тексте
func (m *ReactionsModule) handleTestReaction(c tele.Context) error {
	// Проверка прав админа
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Команда доступна только администраторам")
	}

	args := strings.SplitN(c.Text(), " ", 3)
	if len(args) != 3 {
		return c.Send(
			"📖 *Использование:*\n"+
				"`/testreaction <pattern> <text>`\n\n"+
				"*Примеры:*\n"+
				"`/testreaction (?i)привет Привет мир`\n"+
				"`/testreaction спам это спамное сообщение`",
			&tele.SendOptions{ParseMode: tele.ModeMarkdown},
		)
	}

	pattern := args[1]
	text := args[2]

	// Тестируем как regex
	regexMatch := false
	re, err := regexp.Compile(pattern)
	if err == nil {
		regexMatch = re.MatchString(text)
	}

	// Тестируем как exact
	exactMatch := pattern == text

	// Тестируем как contains
	containsMatch := strings.Contains(strings.ToLower(text), strings.ToLower(pattern))

	result := fmt.Sprintf(
		"🧪 *Тест паттерна:*\n\n"+
			"*Pattern:* `%s`\n"+
			"*Text:* `%s`\n\n"+
			"*Результаты:*\n"+
			"• regex: %s\n"+
			"• exact: %s\n"+
			"• contains: %s",
		pattern, text,
		formatMatch(regexMatch), formatMatch(exactMatch), formatMatch(containsMatch),
	)

	if err != nil {
		result += fmt.Sprintf("\n\n⚠️ Regex ошибка: `%v`", err)
	}

	return c.Send(result, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// formatMatch форматирует результат match
func formatMatch(match bool) string {
	if match {
		return "✅ совпадение"
	}
	return "❌ нет"
}

// isAdmin проверяет является ли пользователь админом
func (m *ReactionsModule) isAdmin(userID int64) bool {
	for _, adminID := range m.adminUsers {
		if adminID == userID {
			return true
		}
	}
	return false
}

// SetAdminUsers устанавливает список админов
func (m *ReactionsModule) SetAdminUsers(adminUsers []int64) {
	m.adminUsers = adminUsers
	m.logger.Info("admin users updated for reactions module", zap.Int("count", len(adminUsers)))
}
