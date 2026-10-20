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
)

func main() {
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

	c := crawler.New(store, crawler.Options{
		Workers:        cfg.Workers,
		RequestTimeout: cfg.RequestTimeout,
		MaxPageBytes:   cfg.MaxPageBytes,
		HostInterval:   cfg.HostInterval,
	})

	err = c.Run(ctx, cfg.Seeds)
	switch {
	case errors.Is(err, context.Canceled):
		fmt.Println("Crawl interrupted. URLs seen so far:", c.VisitedCount())
		return nil
	case err != nil:
		return err
	}
	fmt.Println("Crawl complete. Total URLs visited:", c.VisitedCount())
	return nil
}
