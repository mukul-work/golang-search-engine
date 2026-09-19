package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/mukul-work/golang-search-engine/db"
	"github.com/mukul-work/golang-search-engine/search"
)

func main() {
	limit := flag.Int("n", 10, "max results")
	flag.Parse()

	q := strings.Join(flag.Args(), " ")
	if q == "" {
		fmt.Fprintln(os.Stderr, "usage: search [-n 10] <query>")
		os.Exit(1)
	}

	// load environment variables
	err := godotenv.Load(".env.local")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// create a db pool
	dbURL := os.Getenv("DB_URL")
	ctx := context.Background()
	err = db.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer db.Pool.Close()

	results, err := search.Query(context.Background(), q, *limit)
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
