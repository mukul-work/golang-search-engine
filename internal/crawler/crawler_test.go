package crawler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

func testOptions() Options {
	return Options{
		Workers:        2,
		RequestTimeout: time.Second,
		MaxPageBytes:   1 << 20,
		HostInterval:   0, // no rate limiting: tests don't need to wait
	}
}

type fakeFetcher struct {
	pages map[string]string // url -> HTML; unknown URLs fail
}

func (f *fakeFetcher) Fetch(_ context.Context, url string) (*Page, error) {
	body, ok := f.pages[url]
	if !ok {
		return nil, errors.New("not found")
	}
	return &Page{Body: body, ContentType: "text/html", StatusCode: 200}, nil
}

type fakeIndexer struct {
	mu   sync.Mutex // workers are goroutines, so guard the slice
	urls []string
}

func (f *fakeIndexer) IndexPage(_ context.Context, _ map[string]int, url, _, _ string, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls = append(f.urls, url)
	return nil
}

func TestEnqueue_RejectsDuplicates(t *testing.T) {
	c := New(nil, testOptions())

	if !c.enqueue("https://example.com/a") {
		t.Error("first enqueue should be accepted")
	}
	if c.enqueue("https://example.com/a") {
		t.Error("duplicate enqueue should be rejected")
	}
	if got := c.VisitedCount(); got != 1 {
		t.Errorf("VisitedCount = %d, want 1", got)
	}
}

func TestEnqueue_WaitGroupMatchesAcceptedURLs(t *testing.T) {
	c := New(nil, testOptions())
	c.enqueue("https://example.com/a")
	c.enqueue("https://example.com/a") // duplicate: must NOT touch the counter
	c.enqueue("https://example.com/b")

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	// Act as a worker: take the two accepted URLs and mark them done.
	for i := 0; i < 2; i++ {
		<-c.frontier.out
		c.wg.Done()
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitGroup never reached zero: counter and queue disagree")
	}
}

// robots.txt still goes through the real RobotCache, so serve one:
// 404 on every path means "no rules", which allows everything.
func newRobotsServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close) // runs when the test ends
	return srv
}

func runWithTimeout(t *testing.T, c *Crawler, seeds []string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		c.Run(seeds)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("crawl did not finish: WaitGroup accounting is broken")
	}
}

func TestRun_CrawlsDiscoveredLinks(t *testing.T) {
	srv := newRobotsServer(t)
	seed, next := srv.URL+"/", srv.URL+"/a"

	idx := &fakeIndexer{}
	c := New(idx, testOptions())
	c.fetcher = &fakeFetcher{pages: map[string]string{
		seed: `<html><head><title>Home</title></head><body>hello <a href="/a">a</a></body></html>`,
		next: `<html><body>second page</body></html>`,
	}}

	runWithTimeout(t, c, []string{seed})

	sort.Strings(idx.urls)
	if want := []string{seed, next}; !reflect.DeepEqual(idx.urls, want) {
		t.Errorf("indexed %v, want %v", idx.urls, want)
	}
	if got := c.VisitedCount(); got != 2 {
		t.Errorf("VisitedCount = %d, want 2", got)
	}
}

func TestRun_FetchErrorStillFinishes(t *testing.T) {
	srv := newRobotsServer(t)
	idx := &fakeIndexer{}
	c := New(idx, testOptions())
	c.fetcher = &fakeFetcher{} // knows no pages, so every Fetch fails

	runWithTimeout(t, c, []string{srv.URL + "/missing"})

	if len(idx.urls) != 0 {
		t.Errorf("indexed %v, want nothing", idx.urls)
	}
}
