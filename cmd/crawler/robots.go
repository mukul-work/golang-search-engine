package crawler

import (
	"net/http"
	"net/url"
	"sync"

	"github.com/temoto/robotstxt"
)

type RobotCache struct {
	mu    sync.Mutex
	cache map[string]*robotstxt.RobotsData
}

const userAgent = "WebCrawler/0.1 (+https://github.com/mukul-work/golang-web-crawler)"

func NewRobotCache() *RobotCache {
	return &RobotCache{
		mu:    sync.Mutex{},
		cache: make(map[string]*robotstxt.RobotsData),
	}
}

func (rc *RobotCache) Get(host string) (*robotstxt.RobotsData, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if data, ok := rc.cache[host]; ok {
		return data, nil
	}
	req, err := http.NewRequest("GET", "https://"+host+"/robots.txt", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := robotstxt.FromResponse(resp)
	if err != nil {
		return nil, err
	}
	rc.cache[host] = data
	return data, nil
}

func (rc *RobotCache) Allowed(rawUrl string) (bool, error) {
	url, err := url.Parse(rawUrl)
	if err != nil {
		return false, err
	}

	data, err := rc.Get(url.Host)
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
