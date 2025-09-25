package telegram_bot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/flybasist/bmft/internal/config"
	"github.com/flybasist/bmft/internal/kafkabot"
	"github.com/flybasist/bmft/internal/logx"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Русский комментарий: Пакет обрабатывает взаимодействие с Telegram API и обмен через Kafka.
// Теперь использует контекст для остановки и структурированное логирование.

// Run запускает все компоненты telegram_bot.
func Run(ctx context.Context, cfg *config.Config) {
	log := logx.L().Named("telegram")
	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		log.Fatal("failed to create bot", zap.Error(err))
	}
	log.Info("authorized", zap.String("bot", bot.Self.UserName))

	// Создаём writer для входящих апдейтов (topic telegram-listener)
	listenerWriter := kafkabot.NewWriter("telegram-listener", cfg.KafkaBrokers)
	defer listenerWriter.Close()

	// Создаём reader'ы для send/delete
	senderReader := kafkabot.NewReader("telegram-send", cfg.KafkaBrokers, cfg.KafkaGroupSend)
	deleterReader := kafkabot.NewReader("telegram-delete", cfg.KafkaBrokers, cfg.KafkaGroupDelete)
	defer senderReader.Close()
	defer deleterReader.Close()

	// Канал завершения для UpdateChan (нет прямой поддержки контекста в lib)
	go listenIncoming(ctx, bot, listenerWriter)
	go consumeSender(ctx, bot, senderReader)
	go consumeDeleter(ctx, bot, deleterReader)
}

// listenIncoming слушает входящие сообщения Telegram и пишет в Kafka.
func listenIncoming(ctx context.Context, bot *tgbotapi.BotAPI, writer *kafka.Writer) {
	log := logx.L().Named("telegram.listener")
	updateCfg := tgbotapi.NewUpdate(0)
	updateCfg.Timeout = 60
	updates := bot.GetUpdatesChan(updateCfg)
	for {
		select {
		case <-ctx.Done():
			log.Info("context canceled, stop telegram updates consumption")
			return
		case upd, ok := <-updates:
			if !ok {
				log.Warn("updates channel closed")
				return
			}
			data, err := json.Marshal(upd)
			if err != nil {
				log.Warn("marshal update failed", zap.Error(err))
				continue
			}
			key := fmt.Sprint(upd.UpdateID)
			if err := kafkabot.WriteMessage(ctx, writer, key, data); err != nil {
				log.Error("write to kafka failed", zap.Error(err))
			} else {
				log.Debug("update published", zap.String("key", key))
			}
		}
	}
}

// consumeSender читает сообщения на отправку и выполняет их.
func consumeSender(ctx context.Context, bot *tgbotapi.BotAPI, reader *kafka.Reader) {
	log := logx.L().Named("telegram.sender")
	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Info("context canceled, stop sender consumer")
				return
			}
			log.Warn("fetch send message failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}
		var out SendMessage
		if err := json.Unmarshal(msg.Value, &out); err != nil {
			log.Error("parse outgoing failed", zap.Error(err))
			reader.CommitMessages(ctx, msg) // пропускаем повреждённое
			continue
		}
		switch out.TypeMsg {
		case "text":
			m := tgbotapi.NewMessage(out.ChatID, out.Text)
			if _, err := bot.Send(m); err != nil {
				log.Error("send text failed", zap.Error(err), zap.Int64("chat", out.ChatID))
			} else {
				log.Info("text sent", zap.Int64("chat", out.ChatID))
			}
		case "sticker":
			st := tgbotapi.NewSticker(out.ChatID, tgbotapi.FileID(out.Sticker))
			if _, err := bot.Send(st); err != nil {
				log.Error("send sticker failed", zap.Error(err), zap.Int64("chat", out.ChatID))
			} else {
				log.Info("sticker sent", zap.Int64("chat", out.ChatID))
			}
		default:
			log.Warn("unknown message type", zap.String("type", out.TypeMsg))
		}
		if err := reader.CommitMessages(ctx, msg); err != nil {
			log.Warn("commit failed", zap.Error(err))
		}
	}
}

// consumeDeleter читает команды удаления и выполняет их.
func consumeDeleter(ctx context.Context, bot *tgbotapi.BotAPI, reader *kafka.Reader) {
	log := logx.L().Named("telegram.deleter")
	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Info("context canceled, stop deleter consumer")
				return
			}
			log.Warn("fetch delete message failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}
		var del DeleteMessage
		if err := json.Unmarshal(msg.Value, &del); err != nil {
			log.Error("parse delete failed", zap.Error(err))
			reader.CommitMessages(ctx, msg)
			continue
		}
		req := tgbotapi.NewDeleteMessage(del.ChatID, del.MessageID)
		if _, err := bot.Request(req); err != nil {
			log.Error("delete failed", zap.Error(err), zap.Int64("chat", del.ChatID))
		} else {
			log.Info("message deleted", zap.Int64("chat", del.ChatID), zap.Int("msg_id", del.MessageID))
		}
		if err := reader.CommitMessages(ctx, msg); err != nil {
			log.Warn("commit failed", zap.Error(err))
		}
	}
}
