package crawler

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type Options struct {
	Workers        int
	RequestTimeout time.Duration
	MaxPageBytes   int64
	HostInterval   time.Duration
}

type PageIndexer interface {
	IndexPage(ctx context.Context, wordCounts map[string]int, url, title, content string, totalWords int) error
}

type Crawler struct {
	frontier *Frontier
	visited  *VisitedSet
	robots   *RobotCache
	limiter  *HostLimiter
	fetcher  Fetcher
	store    PageIndexer
	workers  int
	wg       sync.WaitGroup
}

func New(store PageIndexer, opts Options) *Crawler {
	client := &http.Client{Timeout: opts.RequestTimeout} // one client, one connection pool
	return &Crawler{
		frontier: NewFrontier(),
		visited:  NewVisitedSet(),
		robots:   NewRobotCache(client),
		limiter:  NewHostLimiter(opts.HostInterval),
		fetcher:  NewHTTPFetcher(client, opts.MaxPageBytes),
		store:    store,
		workers:  opts.Workers,
	}
}

func (c *Crawler) enqueue(url string) bool {
	if c.visited.IsVisited(url) {
		return false
	}
	c.wg.Add(1)
	c.frontier.Push(url)
	return true
}

func (c *Crawler) Run(seeds []string) {
	for i := 0; i < c.workers; i++ {
		go c.worker(i)
	}
	for _, seed := range seeds {
		c.enqueue(seed)
	}
	c.wg.Wait()
	c.frontier.Close()
}

func (c *Crawler) VisitedCount() int {
	return c.visited.Count()
}
