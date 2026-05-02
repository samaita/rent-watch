package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/axonigma/rent-watcher/internal/app"
	"github.com/axonigma/rent-watcher/internal/config"
	"github.com/axonigma/rent-watcher/internal/version"
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
		log.Fatalf("load config: %v", err)
	}
	log.Printf("rent-watcher version=%s", version.String)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatalf("create app: %v", err)
	}

	if err := a.Run(ctx); err != nil {
		log.Fatalf("run app: %v", err)
	}
}
