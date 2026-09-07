package main

import (
	"fmt"
	"sync"

	"github.com/mukul-work/golang-web-crawler/cmd/crawler"
)

type Result struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

func fetchUrl(url string, wg *sync.WaitGroup) {
	defer wg.Done()
	htmlBody, contentType, statusCode, err := crawler.GetPageData(url)
	if err != nil {
		fmt.Printf("Error while fetching the page data for %s: %v\n", url, err)
		return
	}
	fmt.Printf("Content Type: %s\n", contentType)
	fmt.Printf("Status Code: %d\n", statusCode)

	links, err := crawler.GetHTMLData(htmlBody)
	fmt.Printf("Links for URL: %s", url)
	for _, link := range links {
		fmt.Printf("Link: %s\n", link)
	}

}

func main() {
	urls := []string{"https://en.wikipedia.org/wiki/Computer", "https://dev.to/jones_charles_ad50858dbc0/building-a-high-concurrency-web-crawler-in-go-a-practical-guide-i3a"}
	ch := make(chan Result, len(urls))
	var wg sync.WaitGroup

	for _, url := range urls {
		wg.Add(1)
		go fetchUrl(url, &wg)
	}

	wg.Wait()
	close(ch)

}
