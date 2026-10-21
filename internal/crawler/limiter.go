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
	interval time.Duration
}

func NewHostLimiter(interval time.Duration) *HostLimiter {
	return &HostLimiter{
		limiters: make(map[string]*rate.Limiter),
		interval: interval,
	}
}

func (hl *HostLimiter) getLimiter(host string) *rate.Limiter {
	hl.mu.Lock()
	defer hl.mu.Unlock()

	lim, ok := hl.limiters[host]
	if !ok {
		lim = rate.NewLimiter(rate.Every(hl.interval), 1) // burst 1
		hl.limiters[host] = lim
	}
	return lim
}

// func (hl *HostLimiter) Wait(ctx context.Context, rawURL string) error {
// 	u, err := url.Parse(rawURL)
// 	if err != nil {
// 		return err
// 	}
// 	lim := hl.getLimiter(u.Host)
// 	start := time.Now()
// 	err = lim.Wait(ctx)
// 	elapsed := time.Since(start)

//		if elapsed > 10*time.Millisecond { // only log if it actually waited
//			fmt.Printf("[RateLimiter] Waited %v for host: %s\n", elapsed, u.Host)
//		}
//		return err
//	}

// Wait blocks until the host's limiter allows a request and returns how long it waited.
func (hl *HostLimiter) Wait(ctx context.Context, rawURL string) (time.Duration, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	err = hl.getLimiter(u.Host).Wait(ctx)
	return time.Since(start), err
}
