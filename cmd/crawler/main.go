package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mukul-work/golang-search-engine/internal/config"
	"github.com/mukul-work/golang-search-engine/internal/crawler"
	"github.com/mukul-work/golang-search-engine/internal/db"
)

func main() {
	if err := config.LoadDotEnv(".env.local"); err != nil {
		log.Fatalf("loading .env.local: %v", err)
	}
	cfg, err := config.Load(os.Args[1:], os.Getenv)
	if errors.Is(err, flag.ErrHelp) {
		return // -h was passed and usage was already printed
	}
	if err != nil {
		log.Fatalf("invalid configuration:\n%v", err)
	}

	store, err := db.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer store.Close()

	c := crawler.New(store, crawler.Options{
		Workers:        cfg.Workers,
		RequestTimeout: cfg.RequestTimeout,
		MaxPageBytes:   cfg.MaxPageBytes,
		HostInterval:   cfg.HostInterval,
	})
	c.Run(cfg.Seeds)
	fmt.Println("Crawl complete. Total URLs visited:", c.VisitedCount())
}
