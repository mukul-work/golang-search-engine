package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mukul-work/golang-search-engine/internal/indexer"
	"golang.org/x/net/html"
)

func (c *Crawler) fetchUrl(ctx context.Context, log *slog.Logger, url string) ([]string, error) {
	page, err := c.fetcher.Fetch(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	log.Debug("fetched", "status", page.StatusCode, "content_type", page.ContentType)

	doc, err := html.Parse(strings.NewReader(page.Body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	links, err := GetHTMLData(doc, url)
	if err != nil {
		return nil, fmt.Errorf("extract links: %w", err)
	}
	text, err := GetPageText(doc)
	if err != nil {
		return nil, fmt.Errorf("extract text: %w", err)
	}

	wordCounts := indexer.Tokenize(text)
	total := 0
	for _, n := range wordCounts {
		total += n
	}
	title, err := GetPageTitle(doc)
	if err != nil {
		return nil, fmt.Errorf("extract title: %w", err)
	}
	if err := c.store.IndexPage(ctx, wordCounts, url, title, text, total); err != nil {
		return nil, fmt.Errorf("index page: %w", err)
	}
	return links, nil
}

func (c *Crawler) worker(ctx context.Context, num int) {
	for task := range c.frontier.out {
		c.process(ctx, num, task)
	}
}

func (c *Crawler) process(ctx context.Context, workerNum int, task Task) {
	defer c.wg.Done() // exactly one Done per dequeued URL, on every return path

	// After a cancel, workers keep receiving only to drain the queue. Skip the work.
	if ctx.Err() != nil {
		return
	}

	url := task.URL
	log := c.logger.With("worker", workerNum, "url", url, "depth", task.Depth)

	allowed, err := c.robots.Allowed(ctx, url)
	if err != nil {
		if ctx.Err() == nil { // errors caused by shutdown are not failures
			c.failed.Add(1)
			log.Warn("robots.txt check failed", "err", err)
		}
		return
	}
	if !allowed {
		c.disallowed.Add(1)
		log.Debug("skipped: disallowed by robots.txt")
		return
	}

	start := time.Now()
	links, err := c.fetchUrl(ctx, log, url)
	if err != nil {
		if ctx.Err() == nil {
			c.failed.Add(1)
			log.Warn("crawl failed", "err", err)
		}
		return
	}
	c.indexed.Add(1)
	if ctx.Err() != nil { // don't queue children of a page we are abandoning
		return
	}

	newLinks := 0
	for _, link := range links {
		if c.enqueue(link, task.Depth+1) {
			newLinks++
		}
	}
	log.Info("page crawled", "links", len(links), "new_links", newLinks, "took", time.Since(start))
}
