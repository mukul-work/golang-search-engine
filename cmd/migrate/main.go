package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/mukul-work/golang-search-engine/internal/config"
	"github.com/mukul-work/golang-search-engine/internal/db"
)

const usage = "usage: migrate up | migrate status | migrate down --yes"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	args := os.Args[1:]
	switch {
	case len(args) == 1 && (args[0] == "up" || args[0] == "status"):
	case len(args) == 2 && args[0] == "down" && args[1] == "--yes":
	default:
		return errors.New(usage)
	}
	cmd := args[0]

	if err := config.LoadDotEnv(".env.local"); err != nil {
		return fmt.Errorf("loading .env.local: %w", err)
	}
	dsn := os.Getenv("DB_URL")
	if dsn == "" {
		return errors.New("DB_URL is required")
	}

	m, err := db.NewMigrator(dsn)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer m.Close()

	ctx := context.Background()
	switch cmd {
	case "up":
		applied, err := m.Up(ctx)
		for _, v := range applied {
			fmt.Printf("applied %05d\n", v)
		}
		if err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
		if len(applied) == 0 {
			fmt.Println("nothing to apply: database is up to date")
		}
	case "down":
		v, err := m.Down(ctx)
		if err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		fmt.Printf("rolled back %05d\n", v)
	case "status":
		statuses, err := m.Status(ctx)
		if err != nil {
			return fmt.Errorf("migrate status: %w", err)
		}
		for _, s := range statuses {
			state := "pending"
			if s.Applied {
				state = "applied"
			}
			fmt.Printf("%05d  %s\n", s.Version, state)
		}
	}
	return nil
}
