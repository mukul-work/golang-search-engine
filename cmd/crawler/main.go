package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mukul-work/golang-search-engine/internal/config"
	"github.com/mukul-work/golang-search-engine/internal/crawler"
	"github.com/mukul-work/golang-search-engine/internal/db"
	"github.com/mukul-work/golang-search-engine/internal/logging"
)

func main() {
	// Plain log is used only here: config may be broken, so the logger may not exist yet.
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := config.LoadDotEnv(".env.local"); err != nil {
		return fmt.Errorf("loading .env.local: %w", err)
	}
	cfg, err := config.Load(os.Args[1:], os.Getenv)
	if errors.Is(err, flag.ErrHelp) {
		return nil // -h was passed and usage was already printed
	}
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}

	logger := logging.New(os.Stderr, cfg.LogLevel, cfg.LogFormat)

	// ctx is cancelled on Ctrl-C or SIGTERM (what `docker stop` sends).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stop() // restore default handling: a second Ctrl-C now kills us immediately
	}()

	store, err := db.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to db: %w", err)
	}
	defer store.Close()

	c := crawler.New(store, logger, crawler.Options{
		Workers:        cfg.Workers,
		RequestTimeout: cfg.RequestTimeout,
		MaxPageBytes:   cfg.MaxPageBytes,
		HostInterval:   cfg.HostInterval,
		MaxDepth:       cfg.MaxDepth,
		MaxPages:       cfg.MaxPages,
	})

	// Run logs its own summary. A cancelled crawl is a normal exit, not an error.
	if err := c.Run(ctx, cfg.Seeds); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
