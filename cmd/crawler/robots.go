package crawler

import (
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

func (rc *RobotCache) Allowed(rawUrl string) (bool, error) {
	url, err := url.Parse(rawUrl)
	if err != nil {
		return false, err
	}

	data := rc.Get(url.Host)
	group := data.FindGroup(userAgent)

	path := url.Path
	if url.RawQuery != "" {
		path += "?" + url.RawQuery
	}
	return group.Test(path)
}
