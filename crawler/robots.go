package crawler

import (
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
	userAgent     = "WebCrawler/0.1 (+https://github.com/mukul-work/golang-web-crawler)"
	robotsToken   = "WebCrawler"
	maxRobotsSize = 512 << 10 // 512 KiB
	successTTL    = 24 * time.Hour
	failureTTL    = 5 * time.Minute
)

func NewRobotCache() *RobotCache {
	return &RobotCache{
		entries: make(map[string]*entry),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (rc *RobotCache) Get(scheme, host string) (*robotstxt.RobotsData, error) {
	origin := scheme + "://" + strings.ToLower(host)

	for {
		rc.mu.Lock()
		e, ok := rc.entries[origin]
		if !ok {
			e = &entry{}
			rc.entries[origin] = e
		}
		rc.mu.Unlock()

		e.once.Do(func() {
			e.data, e.err = rc.fetch(origin)
			ttl := successTTL
			if e.err != nil {
				ttl = failureTTL
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

func (rc *RobotCache) fetch(origin string) (*robotstxt.RobotsData, error) {
	req, err := http.NewRequest("GET", origin+"/robots.txt", nil)
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

func (rc *RobotCache) Allowed(rawURL string) (bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false, err
	}

	data, err := rc.Get(u.Scheme, u.Host)
	if err != nil {
		return false, err
	}
	group := data.FindGroup(robotsToken)
	return group.Test(u.RequestURI()), nil
}
