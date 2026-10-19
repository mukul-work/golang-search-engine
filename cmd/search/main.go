package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mukul-work/golang-search-engine/internal/config"
	"github.com/mukul-work/golang-search-engine/internal/db"
	"github.com/mukul-work/golang-search-engine/internal/search"
)

func main() {
	limit := flag.Int("n", 10, "max results")
	flag.Parse()

	q := strings.Join(flag.Args(), " ")
	if q == "" {
		fmt.Fprintln(os.Stderr, "usage: search [-n 10] <query>")
		os.Exit(1)
	}

	if err := config.LoadDotEnv(".env.local"); err != nil {
		log.Fatalf("loading .env.local: %v", err)
	}

	// create a db pool
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL is required")
	}
	ctx := context.Background()
	store, err := db.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer store.Close()

	svc := search.NewService(store)
	results, err := svc.Query(ctx, q, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search failed:", err)
		os.Exit(1)
	}
	if len(results) == 0 {
		fmt.Print("Empty result set\n")
	}
	for i, r := range results {
		fmt.Printf("%d. %s\n   %s\n   %s\n   %f\n\n", i+1, r.Title, r.URL, r.Snippet, r.Score)
	}
}
