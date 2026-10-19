package crawler

import (
	"context"
	"sync"
)

type PageIndexer interface {
	IndexPage(ctx context.Context, wordCounts map[string]int, url, title, content string, totalWords int) error
}

type Crawler struct {
	frontier *Frontier
	visited  *VisitedSet
	robots   *RobotCache
	limiter  *HostLimiter
	store    PageIndexer
	workers  int
	wg       sync.WaitGroup
}

func New(store PageIndexer, workers int) *Crawler {
	return &Crawler{
		frontier: NewFrontier(),
		visited:  NewVisitedSet(),
		robots:   NewRobotCache(),
		limiter:  NewHostLimiter(),
		store:    store,
		workers:  workers,
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
