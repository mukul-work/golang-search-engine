package crawler

import (
	"context"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type HostLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func NewHostLimiter() *HostLimiter {
	return &HostLimiter{
		limiters: make(map[string]*rate.Limiter),
	}
}

func (hl *HostLimiter) getLimiter(host string) *rate.Limiter {
	hl.mu.Lock()
	defer hl.mu.Unlock()

	lim, ok := hl.limiters[host]
	if !ok {
		lim = rate.NewLimiter(rate.Every(time.Second), 1) // 1 req/sec, burst 1
		hl.limiters[host] = lim
	}
	return lim
}

func (hl *HostLimiter) Wait(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	lim := hl.getLimiter(u.Host)
	return lim.Wait(context.Background())
}
