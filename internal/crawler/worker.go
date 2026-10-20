package crawler

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/mukul-work/golang-search-engine/internal/indexer"
	"golang.org/x/net/html"
)

func (c *Crawler) fetchUrl(ctx context.Context, url string, workerNum int) ([]string, error) {
	// context.Background() is temporary: the graceful-shutdown step threads a real ctx through.
	// page, err := c.fetcher.Fetch(context.Background(), url)
	page, err := c.fetcher.Fetch(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("error while fetching the page data for '%s': %w", url, err)
	}

	fmt.Printf("Worker: %d Trying to visit: %s\n", workerNum, url)
	fmt.Printf("Worker: %d Content Type: %s\n", workerNum, page.ContentType)
	fmt.Printf("Worker: %d Status Code: %d\n", workerNum, page.StatusCode)

	doc, err := html.Parse(strings.NewReader(page.Body))
	if err != nil {
		return nil, fmt.Errorf("Problem while parsing the HTML before passing it into the GetHTMLData: %w", err)
	}

	links, err := GetHTMLData(doc, url)
	if err != nil {
		return nil, fmt.Errorf("Problem while parsing the HTML: %w", err)
	}

	text, err := GetPageText(doc)
	if err != nil {
		return nil, fmt.Errorf("Problem extracting text: %w", err)
	}

	wordCounts := indexer.Tokenize(text)
	total := 0
	for _, c := range wordCounts {
		total += c
	}
	title, err := GetPageTitle(doc)
	if err != nil {
		return nil, fmt.Errorf("Problem extracting title: %w", err)
	}

	if err := c.store.IndexPage(ctx, wordCounts, url, title, text, total); err != nil {
		return nil, fmt.Errorf("Failed to insert into the database: %w", err)
	}

	// pageID, err := db.InsertPage(context.Background(), url, title, text, total)
	// if err != nil {
	// 	return nil, fmt.Errorf("Failed to insert page: %w", err)
	// }

	// if err := db.InsertWordEntries(context.Background(), pageID, wordCounts); err != nil {
	// 	return nil, fmt.Errorf("Failed to insert word entries: %w", err)
	// }

	return links, nil

}

func (c *Crawler) worker(ctx context.Context, num int) {
	for url := range c.frontier.out {
		c.process(ctx, num, url)
	}
}

func (c *Crawler) process(ctx context.Context, workerNum int, url string) {
	defer c.wg.Done() // exactly one Done per dequeued URL, on every return path

	// After a cancel, workers keep receiving only to drain the queue so the
	// WaitGroup reaches zero. Skip the work.
	if ctx.Err() != nil {
		return
	}
	allowed, err := c.robots.Allowed(ctx, url)
	if err != nil {
		if ctx.Err() == nil { // stay quiet about errors caused by shutdown
			fmt.Printf("Worker: %d Error while working with robots.txt: %v\n", workerNum, err)
		}
		return
	}
	if !allowed {
		fmt.Printf("Worker: %d Skipping (robots.txt disallows): %s\n", workerNum, url)
		return
	}
	if err := c.limiter.Wait(ctx, url); err != nil {
		if ctx.Err() == nil {
			fmt.Printf("Worker: %d Rate limiter error: %v\n", workerNum, err)
		}
		return
	}
	links, err := c.fetchUrl(ctx, url, workerNum)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("Worker: %d %v\n", workerNum, err)
		}
		return
	}
	if ctx.Err() != nil { // don't queue children of a page we are abandoning
		return
	}

	fmt.Printf("Worker: %d List of links for '%s':\n\n", workerNum, url)
	for _, link := range links {
		fmt.Printf("Worker: %d Link before checking in the visited Set: %s\n", workerNum, link)
		if c.enqueue(link) {
			fmt.Printf("Worker: %d Link pushed to visited set\n\n", workerNum)
		} else {
			fmt.Printf("Worker: %d Link already in visited set\n\n", workerNum)
		}
	}
	fmt.Printf("Worker: %d Visited URL: %s\n\n\n\n", workerNum, url)
}
