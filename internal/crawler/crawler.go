package crawler

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
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
	logger   *slog.Logger
	workers  int
	wg       sync.WaitGroup

	// outcome counters, updated by workers
	indexed    atomic.Int64
	failed     atomic.Int64
	disallowed atomic.Int64
}

// New builds a Crawler. logger must not be nil.
func New(store PageIndexer, logger *slog.Logger, opts Options) *Crawler {
	client := &http.Client{Timeout: opts.RequestTimeout}
	return &Crawler{
		frontier: NewFrontier(),
		visited:  NewVisitedSet(),
		robots:   NewRobotCache(client),
		limiter:  NewHostLimiter(opts.HostInterval),
		fetcher:  NewHTTPFetcher(client, opts.MaxPageBytes),
		store:    store,
		logger:   logger,
		workers:  opts.Workers,
	}
}

// Stats is a snapshot of crawl progress.
type Stats struct {
	Seen       int // distinct URLs queued so far, seeds included
	Indexed    int // pages fetched and written to the store
	Failed     int // URLs that errored (robots, fetch, parse or store)
	Disallowed int // URLs skipped because robots.txt forbids them
}

func (c *Crawler) Stats() Stats {
	return Stats{
		Seen:       c.visited.Count(),
		Indexed:    int(c.indexed.Load()),
		Failed:     int(c.failed.Load()),
		Disallowed: int(c.disallowed.Load()),
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
// func (c *Crawler) Run(ctx context.Context, seeds []string) error {
// 	for i := 0; i < c.workers; i++ {
// 		go c.worker(ctx, i)
// 	}
// 	for _, seed := range seeds {
// 		c.enqueue(seed)
// 	}
// 	c.wg.Wait() // after a cancel this still reaches zero: workers drain the queue
// 	c.frontier.Close()
// 	return ctx.Err()
// }

func (c *Crawler) Run(ctx context.Context, seeds []string) error {
	c.logger.Info("crawl started", "seeds", len(seeds), "workers", c.workers)
	for i := 0; i < c.workers; i++ {
		go c.worker(ctx, i)
	}
	for _, seed := range seeds {
		c.enqueue(seed)
	}
	c.wg.Wait() // after a cancel this still reaches zero: workers drain the queue
	c.frontier.Close()

	s := c.Stats()
	c.logger.Info("crawl finished",
		"interrupted", ctx.Err() != nil,
		"seen", s.Seen, "indexed", s.Indexed, "failed", s.Failed, "disallowed", s.Disallowed)
	return ctx.Err()
}

func (c *Crawler) VisitedCount() int {
	return c.visited.Count()
}
