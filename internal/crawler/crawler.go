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

// Run crawls from the seeds and blocks until no URLs remain or ctx is
// cancelled. It returns ctx.Err(), so a non-nil result means the crawl was
// cut short.
func (c *Crawler) Run(ctx context.Context, seeds []string) error {
	for i := 0; i < c.workers; i++ {
		go c.worker(ctx, i)
	}
	for _, seed := range seeds {
		c.enqueue(seed)
	}
	c.wg.Wait() // after a cancel this still reaches zero: workers drain the queue
	c.frontier.Close()
	return ctx.Err()
}

func (c *Crawler) VisitedCount() int {
	return c.visited.Count()
}
