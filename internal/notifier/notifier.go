package notifier

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog/log"

	"github.com/axonigma/rent-watcher/internal/model"
	"github.com/axonigma/rent-watcher/internal/version"
)

type Notifier interface {
	Send(ctx context.Context, text string) error
	Start(ctx context.Context, commands CommandHandler) error
}

type CommandHandler interface {
	VersionText(context.Context) string
	ListText(context.Context) (string, error)
}

type Telegram struct {
	bot          *tgbotapi.BotAPI
	allowedUsers map[int64]bool
}

func NewTelegram(token string, allowedUserIDs []int64) (*Telegram, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	allowedMap := make(map[int64]bool, len(allowedUserIDs))
	for _, id := range allowedUserIDs {
		allowedMap[id] = true
	}
	log.Info().
		Str("bot_username", bot.Self.UserName).
		Int64("bot_id", bot.Self.ID).
		Int("allowed_user_count", len(allowedMap)).
		Msg("telegram bot authenticated")
	return &Telegram{bot: bot, allowedUsers: allowedMap}, nil
}

func (t *Telegram) Send(ctx context.Context, text string) error {
	if len(t.allowedUsers) == 0 {
		log.Warn().Msg("telegram notification skipped: no allowed user ids configured")
		return nil
	}
	for userID := range t.allowedUsers {
		// For direct bot conversations, the private chat ID matches the user ID.
		chatID := userID
		msg := tgbotapi.NewMessage(chatID, text)
		log.Debug().Int64("chat_id", chatID).Msg("sending telegram notification")
		if _, err := t.bot.Send(msg); err != nil {
			return err
		}
	}
	return nil
}

func (t *Telegram) Start(ctx context.Context, commands CommandHandler) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	log.Info().
		Str("bot_username", t.bot.Self.UserName).
		Int("timeout_seconds", u.Timeout).
		Int("allowed_user_count", len(t.allowedUsers)).
		Msg("starting telegram long polling")
	updates := t.bot.GetUpdatesChan(u)
	log.Info().
		Str("bot_username", t.bot.Self.UserName).
		Msg("telegram long polling active")
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("telegram listener stopping")
			return nil
		case update, ok := <-updates:
			if !ok {
				log.Warn().Msg("telegram updates channel closed")
				return nil
			}
			if update.Message == nil {
				log.Debug().Int("update_id", update.UpdateID).Msg("ignoring telegram update without message")
				continue
			}
			fields := strings.Fields(update.Message.Text)
			if len(fields) == 0 {
				log.Debug().
					Int("update_id", update.UpdateID).
					Int64("chat_id", update.Message.Chat.ID).
					Msg("ignoring telegram message without text command")
				continue
			}
			if update.Message.From == nil {
				log.Warn().
					Int("update_id", update.UpdateID).
					Int64("chat_id", update.Message.Chat.ID).
					Str("text", update.Message.Text).
					Msg("ignoring telegram message without sender")
				continue
			}
			chatID := update.Message.Chat.ID
			userID := update.Message.From.ID
			log.Info().
				Int("update_id", update.UpdateID).
				Int64("chat_id", chatID).
				Int64("user_id", userID).
				Str("username", update.Message.From.UserName).
				Str("chat_type", update.Message.Chat.Type).
				Str("text", update.Message.Text).
				Msg("incoming telegram chat")
			if len(t.allowedUsers) > 0 && !t.allowedUsers[userID] {
				log.Warn().
					Int64("chat_id", chatID).
					Int64("user_id", userID).
					Msg("ignoring telegram message from unauthorized user")
				continue
			}
			var text string
			switch fields[0] {
			case "/version":
				text = commands.VersionText(ctx)
			case "/list":
				resp, err := commands.ListText(ctx)
				if err != nil {
					text = fmt.Sprintf("error: %v", err)
				} else {
					text = resp
				}
			default:
				log.Debug().
					Int64("chat_id", chatID).
					Int64("user_id", userID).
					Str("command", fields[0]).
					Msg("ignoring unsupported telegram command")
				continue
			}
			msg := tgbotapi.NewMessage(chatID, text)
			log.Info().
				Int64("chat_id", chatID).
				Int64("user_id", userID).
				Str("command", fields[0]).
				Msg("sending telegram command response")
			if _, err := t.bot.Send(msg); err != nil {
				log.Error().Err(err).Int64("chat_id", chatID).Msg("telegram send command response")
			}
		}
	}
}

type Noop struct{}

func (Noop) Send(context.Context, string) error { return nil }
func (Noop) Start(context.Context, CommandHandler) error {
	return nil
}

func FormatHeartbeat(status model.HeartbeatStatus) string {
	lastCycle := "never"
	if status.LastCompletedCycle != nil {
		lastCycle = status.LastCompletedCycle.Format(time.RFC3339)
	}
	lines := []string{
		fmt.Sprintf("heartbeat %s", version.String),
		fmt.Sprintf("watch_pages=%d", status.WatchPages),
		fmt.Sprintf("available=%d unavailable=%d total=%d", status.Counts.Available, status.Counts.Unavailable, status.Counts.Total),
		fmt.Sprintf("last_completed_cycle=%s", lastCycle),
	}
	if len(status.LatestFailedWatches) > 0 {
		lines = append(lines, "recent_failures:")
		for _, failed := range status.LatestFailedWatches {
			lines = append(lines, fmt.Sprintf("- [%s] %s :: %s", failed.SiteKey, failed.URL, failed.ErrorText))
		}
	}
	return strings.Join(lines, "\n")
}

func FormatEvent(event model.ListingEvent) string {
	switch event.EventType {
	case model.EventTypePriceChanged:
		return fmt.Sprintf("price changed\n%s\n%s -> %s", event.ListingURL, event.OldValue, event.NewValue)
	case model.EventTypeAvailabilityAvailable:
		if event.OldValue == "" {
			return fmt.Sprintf("new listing available\n%s", event.ListingURL)
		}
		return fmt.Sprintf("listing available again\n%s", event.ListingURL)
	case model.EventTypeAvailabilityGone:
		return fmt.Sprintf("listing unavailable\n%s", event.ListingURL)
	default:
		return fmt.Sprintf("listing event %s\n%s", event.EventType, event.ListingURL)
	}
}

type Buffer struct {
	mu   sync.Mutex
	Sent []string
}

func (b *Buffer) Send(_ context.Context, text string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Sent = append(b.Sent, text)
	return nil
}

func (b *Buffer) Start(context.Context, CommandHandler) error { return nil }
