package crawler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mukul-work/golang-search-engine/internal/urlnorm"
)

// PageIndexer is what the crawler needs from storage.
type PageIndexer interface {
	IndexPage(ctx context.Context, wordCounts map[string]int, url, title, content string, totalWords int) error
}

// Options are the tunable settings of a Crawler. The caller is expected to
// pass validated values (config.Load validates them).
type Options struct {
	Workers        int
	RequestTimeout time.Duration
	MaxPageBytes   int64
	HostInterval   time.Duration
	MaxDepth       int      // link hops to follow from a seed; seeds are depth 0
	MaxPages       int      // most distinct URLs to queue, seeds included; <= 0 means no budget
	AllowedHosts   []string // empty means the hosts of the seeds; "*" means any host; an entry also allows its subdomains
}

// Crawler owns everything a crawl needs. A Crawler runs once: Run closes
// its frontier when the crawl finishes.
type Crawler struct {
	frontier *Frontier
	visited  *VisitedSet
	robots   *RobotCache
	limiter  *HostLimiter
	fetcher  Fetcher
	store    PageIndexer
	logger   *slog.Logger
	workers  int
	wg       sync.WaitGroup // counts URLs that are queued or being processed

	maxDepth     int
	maxPages     int
	allowedHosts []string

	// outcome counters, updated by workers
	indexed      atomic.Int64
	failed       atomic.Int64
	disallowed   atomic.Int64
	outOfScope   atomic.Int64
	budgetLogged atomic.Bool
}

// New builds a Crawler. logger must not be nil.
func New(store PageIndexer, logger *slog.Logger, opts Options) *Crawler {
	client := &http.Client{Timeout: opts.RequestTimeout}

	allowed := make([]string, len(opts.AllowedHosts))
	for i, h := range opts.AllowedHosts {
		allowed[i] = strings.ToLower(h)
	}

	return &Crawler{
		frontier:     NewFrontier(),
		visited:      NewVisitedSet(opts.MaxPages),
		robots:       NewRobotCache(client),
		limiter:      NewHostLimiter(opts.HostInterval),
		fetcher:      NewHTTPFetcher(client, opts.MaxPageBytes),
		store:        store,
		logger:       logger,
		workers:      opts.Workers,
		maxDepth:     opts.MaxDepth,
		maxPages:     opts.MaxPages,
		allowedHosts: allowed,
	}
}

// Stats is a snapshot of crawl progress.
type Stats struct {
	Seen       int // distinct URLs queued so far, seeds included
	Indexed    int // pages fetched and written to the store
	Failed     int // URLs that errored (robots, fetch, parse or store)
	Disallowed int // URLs skipped because robots.txt forbids them
	OutOfScope int // link occurrences skipped by the depth, host or budget limits
}

func (c *Crawler) Stats() Stats {
	return Stats{
		Seen:       c.visited.Count(),
		Indexed:    int(c.indexed.Load()),
		Failed:     int(c.failed.Load()),
		Disallowed: int(c.disallowed.Load()),
		OutOfScope: int(c.outOfScope.Load()),
	}
}

// enqueue queues raw at the given depth unless it is out of scope, unusable
// or already seen. It is the only place that increments the WaitGroup, so
// every queued URL gets exactly one Done, and the only place scope rules live.
func (c *Crawler) enqueue(raw string, depth int) bool {
	if depth > c.maxDepth { // cheapest check first
		c.outOfScope.Add(1)
		return false
	}
	u, err := urlnorm.Normalize(raw)
	if err != nil {
		c.logger.Debug("dropped unusable URL", "url", raw, "err", err)
		return false
	}
	if len(c.allowedHosts) > 0 {
		parsed, err := url.Parse(u)
		if err != nil || !hostAllowed(parsed.Hostname(), c.allowedHosts) {
			c.outOfScope.Add(1)
			return false
		}
	}

	switch c.visited.Add(u) {
	case Duplicate:
		return false
	case OverLimit:
		c.outOfScope.Add(1)
		if c.budgetLogged.CompareAndSwap(false, true) { // log the first time only
			c.logger.Info("page budget reached: no more URLs will be queued", "limit", c.maxPages)
		}
		return false
	}

	c.wg.Add(1)
	c.frontier.Push(Task{URL: u, Depth: depth})
	return true
}

// Run crawls from the seeds and blocks until no URLs remain or ctx is
// cancelled. It returns ctx.Err() if the crawl was cut short, and an error if
// no seed could be queued.
func (c *Crawler) Run(ctx context.Context, seeds []string) error {
	if len(c.allowedHosts) == 0 {
		// Staying on the sites we started from is the safe default; "*" opts out.
		// This runs before any worker starts, so nothing reads the slice yet.
		c.allowedHosts = seedHosts(seeds)
	}
	c.logger.Info("crawl started", "seeds", len(seeds), "workers", c.workers,
		"max_depth", c.maxDepth, "max_pages", c.maxPages, "allowed_hosts", c.allowedHosts)
	for i := 0; i < c.workers; i++ {
		go c.worker(ctx, i)
	}

	queued := 0
	for _, seed := range seeds {
		if c.enqueue(seed, 0) {
			queued++
		}
	}
	c.wg.Wait() // after a cancel this still reaches zero: workers drain the queue
	c.frontier.Close()

	s := c.Stats()
	c.logger.Info("crawl finished",
		"interrupted", ctx.Err() != nil,
		"seen", s.Seen, "indexed", s.Indexed, "failed", s.Failed,
		"disallowed", s.Disallowed, "out_of_scope", s.OutOfScope)

	if queued == 0 {
		return errors.New("no seed URL was queued: check the seeds and -allowed-hosts")
	}
	return ctx.Err()
}

func (c *Crawler) VisitedCount() int {
	return c.visited.Count()
}
