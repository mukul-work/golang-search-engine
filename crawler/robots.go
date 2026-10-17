package crawler

import (
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/temoto/robotstxt"
)

type entry struct {
	once sync.Once
	data *robotstxt.RobotsData
	err  error
}

type RobotCache struct {
	mu      sync.Mutex
	entries map[string]*entry
	client  *http.Client
}

const userAgent = "WebCrawler/0.1 (+https://github.com/mukul-work/golang-web-crawler)"

func NewRobotCache() *RobotCache {
	return &RobotCache{
		entries: make(map[string]*entry),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (rc *RobotCache) Get(host string, scheme string) (*robotstxt.RobotsData, error) {
	origin := scheme + "://" + host
	rc.mu.Lock()
	e, ok := rc.entries[host]
	if !ok {
		e = &entry{}
		rc.entries[host] = e
	}
	rc.mu.Unlock()

	e.once.Do(func() {
		e.data, e.err = rc.fetch(origin)
	})
	return e.data, e.err
}

func (rc *RobotCache) fetch(origin string) (*robotstxt.RobotsData, error) {
	req, err := http.NewRequest("GET", origin+"/robots.txt", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-Agent", userAgent)
	resp, err := rc.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return robotstxt.FromResponse(resp)
}

func (rc *RobotCache) Allowed(rawUrl string) (bool, error) {
	url, err := url.Parse(rawUrl)
	if err != nil {
		return false, err
	}

	data, err := rc.Get(url.Host, url.Scheme)
	if err != nil {
		return false, err
	}
	group := data.FindGroup(userAgent)

	path := url.Path
	if url.RawQuery != "" {
		path += "?" + url.RawQuery
	}
	return group.Test(path), nil
}
