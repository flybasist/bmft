package main

import (
	"strconv"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/modules/limiter"
	"github.com/flybasist/bmft/internal/modules/reactions"
	"github.com/flybasist/bmft/internal/modules/scheduler"
	"github.com/flybasist/bmft/internal/wizard"
)

// Имя единого inline-меню. Все экраны (help, statistics, limiter и т.д.)
// используют одну сессию с разными значениями state.Step.
const menuName = "menu"

// Экраны (значения state.Step).
const (
	screenMain      = "main"      // /help — корневое меню
	screenStats     = "stats"     // /statistics
	screenLimiter   = "limiter"   // /limiter
	screenReactions = "reactions" // /reactions
	screenScheduler = "scheduler" // /scheduler
	screenWelcome   = "welcome"   // приветствие
)

// Экраны результатов (показывают данные + кнопка «Назад»).
const (
	screenStatsMyWeek = "stats_myweek"
	screenStatsChatSt = "stats_chat"
	screenStatsTopCh  = "stats_top"

	screenLimMyStats  = "lim_mystats"
	screenLimGetLimit = "lim_getlimit"
	screenLimVIPs     = "lim_vips"

	screenReactList      = "react_list"
	screenReactBans      = "react_bans"
	screenReactProfanity = "react_prof"

	screenSchedList = "sched_list"
)

// Кнопки навигации между разделами.
var (
	btnNavStats     = tele.Btn{Unique: "m_nav_stats", Text: "📊 Статистика"}
	btnNavLimiter   = tele.Btn{Unique: "m_nav_limit", Text: "🚦 Лимиты"}
	btnNavReactions = tele.Btn{Unique: "m_nav_react", Text: "🎯 Реакции"}
	btnNavScheduler = tele.Btn{Unique: "m_nav_sched", Text: "⏰ Планировщик"}
)

// Кнопки действий Statistics.
var (
	btnStatsMyWeek = tele.Btn{Unique: "m_st_myweek", Text: "📈 Моя неделя"}
	btnStatsChatSt = tele.Btn{Unique: "m_st_chat", Text: "🔒 Статистика чата"}
	btnStatsTopCh  = tele.Btn{Unique: "m_st_top", Text: "🔒 Топ активных"}
)

// Кнопки действий Limiter.
var (
	btnLimMyStats  = tele.Btn{Unique: "m_lm_my", Text: "📊 Мои лимиты"}
	btnLimGetLimit = tele.Btn{Unique: "m_lm_chat", Text: "🔒 Лимиты чата"}
	btnLimVIPs     = tele.Btn{Unique: "m_lm_vip", Text: "🔒 VIP-список"}
)

// Кнопки действий Reactions.
var (
	btnReactList      = tele.Btn{Unique: "m_re_list", Text: "🔒 Реакции"}
	btnReactBans      = tele.Btn{Unique: "m_re_bans", Text: "🔒 Фильтры"}
	btnReactProfanity = tele.Btn{Unique: "m_re_prof", Text: "🔒 Мат-фильтр"}
)

// Кнопки действий Scheduler.
var (
	btnSchedList = tele.Btn{Unique: "m_sc_list", Text: "🔒 Список задач"}
)

// Кнопка навигации Welcome.
var btnNavWelcome = tele.Btn{Unique: "m_nav_welc", Text: "👋 Приветствие"}

// registerMenuNavigation регистрирует команды /help, /statistics, /limiter,
// /reactions, /scheduler и callback'и навигации между экранами.
func registerMenuNavigation(bot *tele.Bot, mgr *wizard.Manager, modules *Modules, logger *zap.Logger) {
	// Команды входа — каждая открывает меню на своём экране.
	entries := []struct {
		command string
		screen  string
	}{
		{"/help", screenMain},
		{"/statistics", screenStats},
		{"/limiter", screenLimiter},
		{"/reactions", screenReactions},
		{"/scheduler", screenScheduler},
	}
	for _, e := range entries {
		entry := e
		bot.Handle(entry.command, func(c tele.Context) error {
			logger.Info("handling menu command",
				zap.String("command", entry.command),
				zap.Int64("chat_id", c.Chat().ID),
				zap.Int64("user_id", c.Sender().ID))
			return openMenuScreen(c, mgr, modules, entry.screen)
		})
	}

	// Callback'и навигации между экранами.
	navButtons := []struct {
		btn    *tele.Btn
		screen string
	}{
		{&btnNavStats, screenStats},
		{&btnNavLimiter, screenLimiter},
		{&btnNavReactions, screenReactions},
		{&btnNavScheduler, screenScheduler},
		{&btnNavWelcome, screenWelcome},
	}
	for _, nb := range navButtons {
		nav := nb
		bot.Handle(nav.btn, func(c tele.Context) error {
			_ = c.Respond()
			state, err := mgr.GuardMenu(c, menuName)
			if err != nil {
				return nil
			}
			state.Step = nav.screen
			return renderScreen(mgr, modules, state)
		})
	}

	// Кнопка «Назад» — возвращает на главный экран.
	btnBack := wizard.BackButton()
	bot.Handle(&btnBack, func(c tele.Context) error {
		_ = c.Respond()
		state, err := mgr.GuardMenu(c, menuName)
		if err != nil {
			return nil
		}
		// Если на экране результата — возвращаемся в меню модуля.
		// Если в меню модуля — возвращаемся в главное меню.
		switch state.Step {
		case screenStatsMyWeek, screenStatsChatSt, screenStatsTopCh:
			state.Step = screenStats
		case screenLimMyStats, screenLimGetLimit, screenLimVIPs:
			state.Step = screenLimiter
		case screenReactList, screenReactBans, screenReactProfanity:
			state.Step = screenReactions
		case screenSchedList:
			state.Step = screenScheduler
		default:
			state.Step = screenMain
		}
		return renderScreen(mgr, modules, state)
	})

	// ── Кнопки действий Statistics ─────────────────────────────────────────

	// «Моя неделя» — доступна всем.
	bot.Handle(&btnStatsMyWeek, func(c tele.Context) error {
		_ = c.Respond()
		state, err := mgr.GuardMenu(c, menuName)
		if err != nil {
			return nil
		}
		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)
		userID := state.Key.UserID

		text, renderErr := modules.Statistics.RenderMyWeek(chatID, threadID, userID)
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ У вас пока нет сообщений за последнюю неделю."
		}

		state.Step = screenStatsMyWeek
		return mgr.EditMenu(state, text, backCloseMarkup())
	})

	// «Статистика чата» — только для админов.
	bot.Handle(&btnStatsChatSt, func(c tele.Context) error {
		_ = c.Respond()
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			return nil
		}
		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)

		text, renderErr := modules.Statistics.RenderChatStats(chatID, threadID)
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ Пока нет сообщений за сегодня."
		}

		state.Step = screenStatsChatSt
		return mgr.EditMenu(state, text, backCloseMarkup())
	})

	// «Топ активных» — только для админов.
	bot.Handle(&btnStatsTopCh, func(c tele.Context) error {
		_ = c.Respond()
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			return nil
		}
		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)

		text, renderErr := modules.Statistics.RenderTopChat(chatID, threadID, &tele.Chat{ID: chatID})
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ Пока нет сообщений за сегодня."
		}

		state.Step = screenStatsTopCh
		return mgr.EditMenu(state, text, backCloseMarkup())
	})

	// ── Кнопки действий Limiter ────────────────────────────────────────────

	// «Мои лимиты» — доступна всем.
	bot.Handle(&btnLimMyStats, func(c tele.Context) error {
		_ = c.Respond()
		state, err := mgr.GuardMenu(c, menuName)
		if err != nil {
			return nil
		}
		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)
		userID := state.Key.UserID

		text, _, renderErr := modules.Limiter.RenderMyStats(chatID, threadID, userID)
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		}

		state.Step = screenLimMyStats
		return mgr.EditMenu(state, text, backCloseMarkup())
	})

	// «Лимиты чата» — только для админов.
	bot.Handle(&btnLimGetLimit, func(c tele.Context) error {
		_ = c.Respond()
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			return nil
		}
		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)
		userID := state.Key.UserID

		text, renderErr := modules.Limiter.RenderGetLimit(chatID, threadID, userID)
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		}

		state.Step = screenLimGetLimit
		return mgr.EditMenu(state, text, backCloseMarkup())
	})

	// «VIP-список» — только для админов. Показывает VIP + кнопки 🗑 для удаления.
	bot.Handle(&btnLimVIPs, func(c tele.Context) error {
		_ = c.Respond()
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			return nil
		}
		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)

		text, userIDs, renderErr := modules.Limiter.RenderListVIPs(chatID, threadID, &tele.Chat{ID: chatID})
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ VIP-пользователей нет.\n\n💡 Выдать VIP: /setvip (ответом на сообщение)"
		}

		m := &tele.ReplyMarkup{}
		var rows []tele.Row
		vipBtns := limiter.BuildVIPButtons(userIDs)
		for _, btn := range vipBtns {
			rows = append(rows, m.Row(btn))
		}
		rows = append(rows, m.Row(wizard.BackButton(), wizard.CloseButton()))
		m.Inline(rows...)

		state.Step = screenLimVIPs
		return mgr.EditMenu(state, text, m)
	})

	// Кнопка 🗑 — удаление VIP по userID.
	var btnRemoveVIP = tele.Btn{Unique: limiter.UniqueRemoveVIP}
	bot.Handle(&btnRemoveVIP, func(c tele.Context) error {
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			_ = c.Respond()
			return nil
		}

		vipUserID, parseErr := strconv.ParseInt(c.Data(), 10, 64)
		if parseErr != nil {
			_ = c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка данных"})
			return nil
		}

		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)

		if revokeErr := modules.Limiter.RevokeVIPByID(chatID, threadID, vipUserID, state.Key.UserID); revokeErr != nil {
			_ = c.Respond(&tele.CallbackResponse{Text: "❌ Не удалось снять VIP"})
			return nil
		}

		_ = c.Respond(&tele.CallbackResponse{Text: "✅ VIP снят"})

		// Перерисовать список VIP.
		text, userIDs, renderErr := modules.Limiter.RenderListVIPs(chatID, threadID, &tele.Chat{ID: chatID})
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ VIP-пользователей нет.\n\n💡 Выдать VIP: /setvip (ответом на сообщение)"
		}

		m := &tele.ReplyMarkup{}
		var rows []tele.Row
		vipBtns := limiter.BuildVIPButtons(userIDs)
		for _, btn := range vipBtns {
			rows = append(rows, m.Row(btn))
		}
		rows = append(rows, m.Row(wizard.BackButton(), wizard.CloseButton()))
		m.Inline(rows...)

		return mgr.EditMenu(state, text, m)
	})

	// ── Кнопки действий Reactions ──────────────────────────────────────────

	// «Реакции» — только для админов.
	bot.Handle(&btnReactList, func(c tele.Context) error {
		_ = c.Respond()
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			return nil
		}
		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)

		text, ids, renderErr := modules.Reactions.RenderListReactions(chatID, threadID, maxMenuButtons)
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ Реакции не настроены.\n\n💡 Добавить: /addreaction"
		}

		mk := &tele.ReplyMarkup{}
		var rows []tele.Row
		btns := reactions.BuildReactionButtons(ids)
		for i := 0; i < len(btns); i += 2 {
			if i+1 < len(btns) {
				rows = append(rows, mk.Row(btns[i], btns[i+1]))
			} else {
				rows = append(rows, mk.Row(btns[i]))
			}
		}
		rows = append(rows, mk.Row(wizard.BackButton(), wizard.CloseButton()))
		mk.Inline(rows...)

		state.Step = screenReactList
		return mgr.EditMenu(state, text, mk)
	})

	// «Фильтры» — только для админов.
	bot.Handle(&btnReactBans, func(c tele.Context) error {
		_ = c.Respond()
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			return nil
		}
		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)

		text, ids, renderErr := modules.Reactions.RenderListBans(chatID, threadID, maxMenuButtons)
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ Фильтры не настроены.\n\n💡 Добавить: /addban"
		}

		mk := &tele.ReplyMarkup{}
		var rows []tele.Row
		btns := reactions.BuildBanButtons(ids)
		for i := 0; i < len(btns); i += 2 {
			if i+1 < len(btns) {
				rows = append(rows, mk.Row(btns[i], btns[i+1]))
			} else {
				rows = append(rows, mk.Row(btns[i]))
			}
		}
		rows = append(rows, mk.Row(wizard.BackButton(), wizard.CloseButton()))
		mk.Inline(rows...)

		state.Step = screenReactBans
		return mgr.EditMenu(state, text, mk)
	})

	// «Мат-фильтр» — только для админов.
	bot.Handle(&btnReactProfanity, func(c tele.Context) error {
		_ = c.Respond()
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			return nil
		}
		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)

		text, renderErr := modules.Reactions.RenderProfanityStatus(chatID, threadID)
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ Фильтр мата не настроен.\n\n💡 Включить: /setprofanity"
		}

		state.Step = screenReactProfanity
		return mgr.EditMenu(state, text, backCloseMarkup())
	})

	// Кнопка 🗑 — удаление реакции по ID.
	var btnMenuDelReaction = tele.Btn{Unique: reactions.UniqueMenuDelReaction}
	bot.Handle(&btnMenuDelReaction, func(c tele.Context) error {
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			_ = c.Respond()
			return nil
		}

		reactionID, parseErr := strconv.ParseInt(c.Data(), 10, 64)
		if parseErr != nil {
			_ = c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка данных"})
			return nil
		}

		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)

		if delErr := modules.Reactions.DeleteReactionByID(chatID, reactionID, state.Key.UserID); delErr != nil {
			_ = c.Respond(&tele.CallbackResponse{Text: "❌ Не удалось удалить"})
			return nil
		}

		_ = c.Respond(&tele.CallbackResponse{Text: "✅ Реакция удалена"})

		// Перерисовать список.
		text, ids, renderErr := modules.Reactions.RenderListReactions(chatID, threadID, maxMenuButtons)
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ Реакции не настроены.\n\n💡 Добавить: /addreaction"
		}

		mk := &tele.ReplyMarkup{}
		var rows []tele.Row
		btns := reactions.BuildReactionButtons(ids)
		for i := 0; i < len(btns); i += 2 {
			if i+1 < len(btns) {
				rows = append(rows, mk.Row(btns[i], btns[i+1]))
			} else {
				rows = append(rows, mk.Row(btns[i]))
			}
		}
		rows = append(rows, mk.Row(wizard.BackButton(), wizard.CloseButton()))
		mk.Inline(rows...)

		return mgr.EditMenu(state, text, mk)
	})

	// Кнопка 🗑 — удаление фильтра по ID.
	var btnMenuDelBan = tele.Btn{Unique: reactions.UniqueMenuDelBan}
	bot.Handle(&btnMenuDelBan, func(c tele.Context) error {
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			_ = c.Respond()
			return nil
		}

		banID, parseErr := strconv.ParseInt(c.Data(), 10, 64)
		if parseErr != nil {
			_ = c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка данных"})
			return nil
		}

		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)

		if delErr := modules.Reactions.DeleteBanByID(chatID, banID, state.Key.UserID); delErr != nil {
			_ = c.Respond(&tele.CallbackResponse{Text: "❌ Не удалось удалить"})
			return nil
		}

		_ = c.Respond(&tele.CallbackResponse{Text: "✅ Фильтр удалён"})

		// Перерисовать список.
		text, ids, renderErr := modules.Reactions.RenderListBans(chatID, threadID, maxMenuButtons)
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ Фильтры не настроены.\n\n💡 Добавить: /addban"
		}

		mk := &tele.ReplyMarkup{}
		var rows []tele.Row
		btns := reactions.BuildBanButtons(ids)
		for i := 0; i < len(btns); i += 2 {
			if i+1 < len(btns) {
				rows = append(rows, mk.Row(btns[i], btns[i+1]))
			} else {
				rows = append(rows, mk.Row(btns[i]))
			}
		}
		rows = append(rows, mk.Row(wizard.BackButton(), wizard.CloseButton()))
		mk.Inline(rows...)

		return mgr.EditMenu(state, text, mk)
	})

	// ── Кнопки действий Scheduler ──────────────────────────────────────────

	// «Список задач» — только для админов.
	bot.Handle(&btnSchedList, func(c tele.Context) error {
		_ = c.Respond()
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			return nil
		}
		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)

		text, ids, renderErr := modules.Scheduler.RenderListTasks(chatID, threadID)
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ Задачи не настроены.\n\n💡 Добавить: /addtask"
		}

		mk := &tele.ReplyMarkup{}
		var rows []tele.Row
		taskBtns := scheduler.BuildTaskButtons(ids)
		// Каждые 2 кнопки (▶ и 🗑) — одна строка.
		for i := 0; i < len(taskBtns); i += 2 {
			if i+1 < len(taskBtns) {
				rows = append(rows, mk.Row(taskBtns[i], taskBtns[i+1]))
			} else {
				rows = append(rows, mk.Row(taskBtns[i]))
			}
		}
		rows = append(rows, mk.Row(wizard.BackButton(), wizard.CloseButton()))
		mk.Inline(rows...)

		state.Step = screenSchedList
		return mgr.EditMenu(state, text, mk)
	})

	// Кнопка 🗑 — удаление задачи по ID.
	var btnMenuDelTask = tele.Btn{Unique: scheduler.UniqueMenuDelTask}
	bot.Handle(&btnMenuDelTask, func(c tele.Context) error {
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			_ = c.Respond()
			return nil
		}

		taskID, parseErr := strconv.ParseInt(c.Data(), 10, 64)
		if parseErr != nil {
			_ = c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка данных"})
			return nil
		}

		chatID := state.Key.ChatID
		threadID := getThreadIDFromState(state)

		if delErr := modules.Scheduler.DeleteTaskByID(chatID, taskID, state.Key.UserID); delErr != nil {
			_ = c.Respond(&tele.CallbackResponse{Text: "❌ " + delErr.Error()})
			return nil
		}

		_ = c.Respond(&tele.CallbackResponse{Text: "✅ Задача удалена"})

		// Перерисовать список.
		text, ids, renderErr := modules.Scheduler.RenderListTasks(chatID, threadID)
		if renderErr != nil {
			text = "❌ " + renderErr.Error()
		} else if text == "" {
			text = "ℹ️ Задачи не настроены.\n\n💡 Добавить: /addtask"
		}

		mk := &tele.ReplyMarkup{}
		var rows []tele.Row
		taskBtns := scheduler.BuildTaskButtons(ids)
		for i := 0; i < len(taskBtns); i += 2 {
			if i+1 < len(taskBtns) {
				rows = append(rows, mk.Row(taskBtns[i], taskBtns[i+1]))
			} else {
				rows = append(rows, mk.Row(taskBtns[i]))
			}
		}
		rows = append(rows, mk.Row(wizard.BackButton(), wizard.CloseButton()))
		mk.Inline(rows...)

		return mgr.EditMenu(state, text, mk)
	})

	// Кнопка ▶ — запуск задачи по ID.
	var btnMenuRunTask = tele.Btn{Unique: scheduler.UniqueMenuRunTask}
	bot.Handle(&btnMenuRunTask, func(c tele.Context) error {
		state, err := mgr.GuardMenuAdmin(c, menuName)
		if err != nil {
			_ = c.Respond()
			return nil
		}

		taskID, parseErr := strconv.ParseInt(c.Data(), 10, 64)
		if parseErr != nil {
			_ = c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка данных"})
			return nil
		}

		chatID := state.Key.ChatID
		taskName, runErr := modules.Scheduler.RunTaskByID(chatID, taskID, state.Key.UserID)
		if runErr != nil {
			_ = c.Respond(&tele.CallbackResponse{Text: "❌ " + runErr.Error()})
			return nil
		}

		return c.Respond(&tele.CallbackResponse{Text: "▶ " + taskName + " запущена"})
	})
}

// openMenuScreen создаёт новую сессию меню и рисует указанный экран.
func openMenuScreen(c tele.Context, mgr *wizard.Manager, modules *Modules, screen string) error {
	initialData := map[string]any{}
	if sender := c.Sender(); sender != nil {
		if sender.Username != "" {
			initialData["owner"] = "@" + sender.Username
		} else {
			initialData["owner"] = sender.FirstName
		}
	}
	return mgr.StartMenu(c, menuName, initialData, func(state *wizard.State) error {
		state.Step = screen
		text, markup := buildScreen(state)
		msg, err := c.Bot().Send(c.Chat(), text, &tele.SendOptions{
			ParseMode:   tele.ModeHTML,
			ReplyMarkup: markup,
		})
		if err != nil {
			return err
		}
		mgr.SetMessage(state, msg)
		return nil
	})
}

// renderScreen редактирует текущее сообщение меню на новый экран.
func renderScreen(mgr *wizard.Manager, modules *Modules, state *wizard.State) error {
	text, markup := buildScreen(state)
	return mgr.EditMenu(state, text, markup)
}

// buildScreen возвращает текст и кнопки для текущего экрана.
func buildScreen(state *wizard.State) (string, *tele.ReplyMarkup) {
	switch state.Step {
	case screenStats:
		return buildStatsScreen(state)
	case screenLimiter:
		return buildLimiterScreen(state)
	case screenReactions:
		return buildReactionsScreen(state)
	case screenScheduler:
		return buildSchedulerScreen(state)
	case screenWelcome:
		return buildWelcomeScreen(state)
	default:
		return buildMainScreen(state)
	}
}

// ── Главный экран (/help) ──────────────────────────────────────────────────

func buildMainScreen(state *wizard.State) (string, *tele.ReplyMarkup) {
	text := "📖 <b>BMFT — главное меню" + ownerSuffix(state) + "</b>\n\nВыберите раздел:"

	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(btnNavStats, btnNavLimiter),
		m.Row(btnNavReactions, btnNavScheduler),
		m.Row(btnNavWelcome),
		m.Row(wizard.CloseButton()),
	)
	return text, m
}

// ── Статистика ─────────────────────────────────────────────────────────────
func buildStatsScreen(state *wizard.State) (string, *tele.ReplyMarkup) {
	text := "📊 <b>Статистика" + ownerSuffix(state) + "</b>\n\nВыберите действие:"

	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(btnStatsMyWeek),
		m.Row(btnStatsChatSt, btnStatsTopCh),
		m.Row(wizard.BackButton(), wizard.CloseButton()),
	)
	return text, m
}

// ── Лимиты ─────────────────────────────────────────────────────────────────

func buildLimiterScreen(state *wizard.State) (string, *tele.ReplyMarkup) {
	text := "🚦 <b>Лимиты" + ownerSuffix(state) + "</b>\n\n" +
		"Выберите действие:\n\n" +
		"<i>⚙️ Настройка (команды):\n" +
		"/setlimit — установить лимит\n" +
		"💡 Ответьте на сообщение пользователя для персонального лимита\n" +
		"/setvip — выдать VIP (ответом на сообщение)</i>"

	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(btnLimMyStats),
		m.Row(btnLimGetLimit, btnLimVIPs),
		m.Row(wizard.BackButton(), wizard.CloseButton()),
	)
	return text, m
}

// ── Реакции ────────────────────────────────────────────────────────────────

func buildReactionsScreen(state *wizard.State) (string, *tele.ReplyMarkup) {
	text := "🎯 <b>Реакции и фильтры" + ownerSuffix(state) + "</b>\n\n" +
		"Выберите раздел:\n\n" +
		"<i>⚙️ Настройка (команды):\n" +
		"/addreaction — добавить автоответ\n" +
		"💡 Для стикера/гифки/фото: ответьте на медиа командой /addreaction\n" +
		"/addban — добавить запрещённое слово\n" +
		"/setprofanity — включить мат-фильтр\n" +
		"/removeprofanity — выключить мат-фильтр</i>"

	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(btnReactList, btnReactBans),
		m.Row(btnReactProfanity),
		m.Row(wizard.BackButton(), wizard.CloseButton()),
	)
	return text, m
}

// ── Планировщик ────────────────────────────────────────────────────────────

func buildSchedulerScreen(state *wizard.State) (string, *tele.ReplyMarkup) {
	text := "⏰ <b>Планировщик" + ownerSuffix(state) + "</b>\n\n" +
		"Выберите действие:\n\n" +
		"<i>⚙️ Настройка (команды):\n" +
		"/addtask — добавить задачу\n" +
		"💡 Для стикера/фото/видео: ответьте на медиа командой /addtask</i>"

	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(btnSchedList),
		m.Row(wizard.BackButton(), wizard.CloseButton()),
	)
	return text, m
}

// ── Приветствие ────────────────────────────────────────────────────────────

func buildWelcomeScreen(state *wizard.State) (string, *tele.ReplyMarkup) {
	text := "👋 <b>Приветствие" + ownerSuffix(state) + `</b>

Автоприветствие новых участников чата.

<i>⚙️ Настройка (🔒 только для админов):
/welcome — включить/выключить, настроить TTL (wizard с кнопками)
/welcome` + " &lt;текст&gt;" + ` — задать текст приветствия</i>`

	m := &tele.ReplyMarkup{}
	m.Inline(
		m.Row(wizard.BackButton(), wizard.CloseButton()),
	)
	return text, m
}

// ownerSuffix возвращает " (@user)" или "" — для подписи меню.
func ownerSuffix(state *wizard.State) string {
	if owner, ok := state.Data["owner"]; ok {
		if s, ok2 := owner.(string); ok2 && s != "" {
			return " · " + s
		}
	}
	return ""
}

// ── Вспомогательные функции ────────────────────────────────────────────────

// backCloseMarkup — стандартная клавиатура для экранов результатов.
func backCloseMarkup() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	m.Inline(m.Row(wizard.BackButton(), wizard.CloseButton()))
	return m
}

// getThreadIDFromState извлекает threadID из state.Data.
func getThreadIDFromState(state *wizard.State) int {
	v, ok := state.Data[wizard.DataThreadID]
	if !ok {
		return 0
	}
	switch tid := v.(type) {
	case int:
		return tid
	case int64:
		return int(tid)
	case float64:
		return int(tid)
	}
	return 0
}

// maxMenuButtons — максимальное кол-во inline-кнопок 🗑 в меню.
const maxMenuButtons = 20
