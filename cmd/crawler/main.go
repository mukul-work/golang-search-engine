package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/mukul-work/golang-search-engine/internal/crawler"
	"github.com/mukul-work/golang-search-engine/internal/db"
)

// type Urls struct {
// 	URL   string `json:"url"`
// }

// func fetchUrl(url string, wg *sync.WaitGroup) {
// 	defer wg.Done()
// 	htmlBody, contentType, statusCode, err := crawler.GetPageData(url)
// 	if err != nil {
// 		fmt.Printf("Error while fetching the page data for %s: %v\n", url, err)
// 		return
// 	}
// 	fmt.Printf("Content Type: %s\n", contentType)
// 	fmt.Printf("Status Code: %d\n", statusCode)

// 	links, err := crawler.GetHTMLData(htmlBody)
// 	fmt.Printf("Links for URL: %s", url)
// 	for _, link := range links {
// 		fmt.Printf("Link: %s\n", link)
// 	}

// }

func main() {
	// load environment variables
	err := godotenv.Load(".env.local")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// create a db pool
	dbURL := os.Getenv("DB_URL")
	ctx := context.Background()
	store, err := db.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer store.Close()

	c := crawler.New(store, 10)
	seeds := []string{
		"https://example.com",
		"https://scrapfly.io/blog/posts/best-websites-to-practice-web-scraping",
		"https://www.searchenginejournal.com/alternative-search-engines/271409/",
	}
	c.Run(seeds)
	fmt.Println("Crawl complete. Total URLs visited:", c.VisitedCount())

}
