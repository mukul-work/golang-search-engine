package main

import (
	"fmt"
	"sync"

	"github.com/mukul-work/golang-web-crawler/cmd/crawler"
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

	f := crawler.NewFrontier()
	visited := crawler.NewVisitedSet()
	rc := crawler.NewRobotCache()
	var wg sync.WaitGroup

	numWorkers := 10
	for i := 0; i < numWorkers; i++ {
		go crawler.Worker(i, f, &wg, visited, rc)
	}
	seeds := []string{
		"https://example.com",
		"https://scrapfly.io/blog/posts/best-websites-to-practice-web-scraping",
	}

	for _, seed := range seeds {
		if !visited.IsVisited(seed) {
			wg.Add(1)
			f.Push(seed)
		}
	}
	wg.Wait()
	f.Close()
	fmt.Println("Crawl complete. Total URLs visited:", visited.Count())

}
