package notifier

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

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
	return &Telegram{bot: bot, allowedUsers: allowedMap}, nil
}

func (t *Telegram) Send(ctx context.Context, text string) error {
	for userID := range t.allowedUsers {
		// For direct bot conversations, the private chat ID matches the user ID.
		chatID := userID
		msg := tgbotapi.NewMessage(chatID, text)
		if _, err := t.bot.Send(msg); err != nil {
			return err
		}
	}
	return nil
}

func (t *Telegram) Start(ctx context.Context, commands CommandHandler) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := t.bot.GetUpdatesChan(u)
	for {
		select {
		case <-ctx.Done():
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if update.Message == nil {
				continue
			}
			fields := strings.Fields(update.Message.Text)
			if len(fields) == 0 {
				continue
			}
			if update.Message.From == nil {
				continue
			}
			chatID := update.Message.Chat.ID
			userID := update.Message.From.ID
			if len(t.allowedUsers) > 0 && !t.allowedUsers[userID] {
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
				continue
			}
			msg := tgbotapi.NewMessage(chatID, text)
			if _, err := t.bot.Send(msg); err != nil {
				log.Printf("telegram send command response: %v", err)
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
