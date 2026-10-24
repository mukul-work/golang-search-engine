package main

import (
	"context"
	"errors"
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
	log.SetFlags(0) // plain messages: a CLI doesn't need timestamps
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	perPage := flag.Int("n", search.DefaultPerPage, "results per page")
	page := flag.Int("page", 1, "page number, starting at 1")
	flag.Parse()

	q := strings.Join(flag.Args(), " ")
	if q == "" {
		return errors.New("usage: search [-n 10] [-page 1] <query>")
	}

	if err := config.LoadDotEnv(".env.local"); err != nil {
		return fmt.Errorf("loading .env.local: %w", err)
	}
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return errors.New("DB_URL is required")
	}

	ctx := context.Background()
	store, err := db.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to db: %w", err)
	}
	defer store.Close()

	res, err := search.NewService(store).Query(ctx, q, *page, *perPage)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	printResults(res)
	return nil
}

func printResults(res search.Results) {
	if res.Total == 0 {
		fmt.Println("No results")
		return
	}
	if len(res.Items) == 0 {
		fmt.Printf("Page %d is past the end: %d results fit on %d pages\n", res.Page, res.Total, res.TotalPages())
		return
	}
	first := (res.Page-1)*res.PerPage + 1 // rank of the first result on this page
	for i, r := range res.Items {
		fmt.Printf("%d. %s\n   %s\n   %s\n   %f\n\n", first+i, r.Title, r.URL, r.Snippet, r.Score)
	}
	fmt.Printf("Results %d-%d of %d (page %d of %d)\n",
		first, first+len(res.Items)-1, res.Total, res.Page, res.TotalPages())
}
