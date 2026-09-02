package crawler

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

type Result struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

func fetchUrl(url string, wg *sync.WaitGroup, ch chan Result) {
	defer wg.Done()
	res, err := http.Get(url)
	if err != nil {
		log.Printf("Error whlie fetching %s: %v", url, err)
		return
	}
	defer res.Body.Close()
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Printf("Error while parsing webpage: %v", err)
		return
	}
	title := doc.Find("title").Text()
	ch <- Result{URL: url, Title: title}

}

func main() {
	urls := []string{"https://en.wikipedia.org/wiki/Computer", "https://dev.to/jones_charles_ad50858dbc0/building-a-high-concurrency-web-crawler-in-go-a-practical-guide-i3a"}
	ch := make(chan Result, len(urls))
	var wg sync.WaitGroup

	for _, url := range urls {
		wg.Add(1)
		go fetchUrl(url, &wg, ch)
	}

	wg.Wait()
	close(ch)
	for result := range ch {
		fmt.Printf("URL: %s, Title: %s\n", result.URL, result.Title)
	}
}
