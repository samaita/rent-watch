package app

import (
	"context"
	"time"

	"github.com/axonigma/rent-watcher/internal/config"
	"github.com/axonigma/rent-watcher/internal/notifier"
	"github.com/axonigma/rent-watcher/internal/scheduler"
	"github.com/axonigma/rent-watcher/internal/seed"
	"github.com/axonigma/rent-watcher/internal/service"
	"github.com/axonigma/rent-watcher/internal/store"
	"github.com/rs/zerolog/log"
)

type App struct {
	cfg       config.Config
	store     *store.Store
	scheduler *scheduler.Scheduler
	service   *service.Service
	notifier  notifier.Notifier
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, err
	}
	time.Local = loc

	st, err := store.Open(cfg.SQLitePath)
	if err != nil {
		return nil, err
	}
	if err := st.Init(ctx); err != nil {
		st.Close()
		return nil, err
	}
	watches, err := seed.Load(cfg.SeedPath)
	if err != nil {
		st.Close()
		return nil, err
	}
	if err := st.SeedWatchPages(ctx, watches); err != nil {
		st.Close()
		return nil, err
	}

	var n notifier.Notifier = notifier.Noop{}
	if cfg.TelegramBotToken != "" {
		log.Info().
			Int("allowed_user_count", len(cfg.TelegramUserIDs)).
			Msg("telegram integration enabled")
		tg, err := notifier.NewTelegram(cfg.TelegramBotToken, cfg.TelegramUserIDs)
		if err != nil {
			st.Close()
			return nil, err
		}
		n = tg
	} else {
		log.Info().Msg("telegram integration disabled: TELEGRAM_BOT_TOKEN not set")
	}
	svc := service.New(st)
	return &App{
		cfg:       cfg,
		store:     st,
		scheduler: scheduler.New(st, n, cfg),
		service:   svc,
		notifier:  n,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.store.Close()
	a.scheduler.Run(ctx)
	log.Info().Msg("scheduler started")

	errCh := make(chan error, 1)
	go func() {
		if err := a.notifier.Start(ctx, a.service); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info().Msg("shutting down")
		return nil
	case err := <-errCh:
		return err
	}
}
