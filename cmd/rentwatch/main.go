package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/axonigma/rent-watcher/internal/app"
	"github.com/axonigma/rent-watcher/internal/config"
	"github.com/axonigma/rent-watcher/internal/logging"
	"github.com/axonigma/rent-watcher/internal/version"
	"github.com/rs/zerolog/log"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("load config")
	}
	logging.Configure(cfg.Debug)
	log.Info().
		Str("version", version.String).
		Bool("debug", cfg.Debug).
		Msg("starting rent-watcher")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("create app")
	}

	if err := a.Run(ctx); err != nil {
		log.Fatal().Err(err).Msg("run app")
	}
}
