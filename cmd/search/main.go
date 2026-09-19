package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

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

	results, err := search.Query(context.Background(), q, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search failed:", err)
		os.Exit(1)
	}

	for i, r := range results {
		fmt.Printf("%d. %s\n   %s\n   %s\n   %f\n\n", i+1, r.Title, r.URL, r.Snippet, r.Score)
	}
}
