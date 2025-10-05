package core

import (
	"fmt"

	tele "gopkg.in/telebot.v3"
)

// WelcomeModule реализует приветственные сообщения и информацию о боте
// Русский комментарий: Модуль для welcome messages и /version команды
// Аналог Python: reaction.py::newchat(), reaction.py::newmember(), reaction.py::reactionversion()
type WelcomeModule struct {
	version string
}

// NewWelcomeModule создаёт новый инстанс модуля
func NewWelcomeModule(version string) *WelcomeModule {
	return &WelcomeModule{
		version: version,
	}
}

// HandleNewChatMember обрабатывает добавление нового участника в чат
// Русский комментарий: Приветствие нового пользователя
// Аналог Python: reaction.py::newmember()
func (m *WelcomeModule) HandleNewChatMember(c tele.Context) error {
	newMember := c.Message().UserJoined

	// Пропускаем если бот сам присоединился
	if newMember.ID == c.Bot().Me.ID {
		return nil
	}

	username := newMember.Username
	if username == "" {
		username = newMember.FirstName
	}

	answer := fmt.Sprintf(
		"Привет, @%s! Добро пожаловать в наш чат! "+
			"Капча для новых пользователей в разработке, "+
			"поэтому если ты спамер то удались сам пожалуйста",
		username,
	)

	return c.Send(answer)
}

// HandleBotAddedToChat обрабатывает добавление бота в новый чат
// Русский комментарий: Сообщение когда бот добавлен в чат
// Аналог Python: reaction.py::newchat()
func (m *WelcomeModule) HandleBotAddedToChat(c tele.Context) error {
	newMember := c.Message().UserJoined

	// Проверяем что это бот
	if newMember.ID != c.Bot().Me.ID {
		return nil
	}

	answer := "Всем привет! Я ваш новый бот! " +
		"Пока все индивидуальные настройки под чат задаются через @FlyBasist " +
		"но потом меня можно будет настраивать владельцу чата самостоятельно"

	return c.Send(answer)
}

// HandleVersion обрабатывает команду /version
// Русский комментарий: Показывает версию бота и информацию
// Аналог Python: reaction.py::reactionversion()
func (m *WelcomeModule) HandleVersion(c tele.Context) error {
	answer := fmt.Sprintf(
		"Текущая версия - %s\n"+
			"По всем вопросам писать автору бота - @FlyBasist\n"+
			"Индивидуальная реакция стикером не чаще одного раза в десять минут\n"+
			"Разработка бота требует ресурсов, поддержи разработку донатом!",
		m.version,
	)

	return c.Send(answer)
}

// RegisterCommands регистрирует команды модуля
func (m *WelcomeModule) RegisterCommands(bot *tele.Bot) {
	// /version — информация о версии бота
	bot.Handle("/version", m.HandleVersion)

	// new_chat_members — приветствие новых пользователей
	bot.Handle(tele.OnUserJoined, func(c tele.Context) error {
		// Сначала проверяем добавление бота
		if err := m.HandleBotAddedToChat(c); err != nil {
			return err
		}
		// Затем приветствуем обычных пользователей
		return m.HandleNewChatMember(c)
	})
}
