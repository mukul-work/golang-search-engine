package crawler

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/temoto/robotstxt"
)

type entry struct {
	once    sync.Once
	data    *robotstxt.RobotsData
	err     error
	expires time.Time
}

type RobotCache struct {
	mu      sync.Mutex
	entries map[string]*entry
	client  *http.Client
}

const (
	userAgent     = "WebCrawler/0.1 (+https://github.com/mukul-work/golang-search-engine)"
	robotsToken   = "WebCrawler"
	maxRobotsSize = 512 << 10 // 512 KiB
	successTTL    = 24 * time.Hour
	failureTTL    = 5 * time.Minute
)

func NewRobotCache(client *http.Client) *RobotCache {
	return &RobotCache{
		entries: make(map[string]*entry),
		client:  client,
	}
}

func (rc *RobotCache) Get(ctx context.Context, scheme, host string) (*robotstxt.RobotsData, error) {
	origin := scheme + "://" + strings.ToLower(host)

	for {

		if err := ctx.Err(); err != nil { // also stops the retry loop from spinning if we are cancelled
			return nil, err
		}

		rc.mu.Lock()
		e, ok := rc.entries[origin]
		if !ok {
			e = &entry{}
			rc.entries[origin] = e
		}
		rc.mu.Unlock()

		e.once.Do(func() {
			e.data, e.err = rc.fetch(ctx, origin)
			ttl := successTTL
			if e.err != nil {
				ttl = failureTTL
				if ctx.Err() != nil {
					ttl = 0 // cancelled mid-fetch: expire at once instead of caching it
				}
			}
			e.expires = time.Now().Add(ttl)
		})

		if time.Now().Before(e.expires) {
			return e.data, e.err
		}

		// Expired: remove it (only if nobody already replaced it), then loop
		// to create a fresh entry and fetch again.
		rc.mu.Lock()
		if rc.entries[origin] == e { //Several goroutines can see the same expired entry. The first deletes it and a new entry gets created. Without this check, a second goroutine would delete the new entry and cause a duplicate fetch.
			delete(rc.entries, origin)
		}
		rc.mu.Unlock()
	}
}

func (rc *RobotCache) fetch(ctx context.Context, origin string) (*robotstxt.RobotsData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/robots.txt", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := rc.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRobotsSize))
	if err != nil {
		return nil, err
	}
	return robotstxt.FromStatusAndBytes(resp.StatusCode, body)
}

func (rc *RobotCache) Allowed(ctx context.Context, rawURL string) (bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false, err
	}

	data, err := rc.Get(ctx, u.Scheme, u.Host)
	if err != nil {
		return false, err
	}
	group := data.FindGroup(robotsToken)
	return group.Test(u.RequestURI()), nil
}
