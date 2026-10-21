package crawler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
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

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
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
	c := New(nil, discardLogger(), testOptions())

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
	c := New(nil, discardLogger(), testOptions())
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
		c.Run(context.Background(), seeds)
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
	c := New(idx, discardLogger(), testOptions())
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
	c := New(idx, discardLogger(), testOptions())
	c.fetcher = &fakeFetcher{} // knows no pages, so every Fetch fails

	runWithTimeout(t, c, []string{srv.URL + "/missing"})

	if len(idx.urls) != 0 {
		t.Errorf("indexed %v, want nothing", idx.urls)
	}
}

// blockingFetcher simulates a request in flight: it signals that it started,
// then blocks until the context is cancelled.
type blockingFetcher struct {
	started chan struct{}
}

func (b *blockingFetcher) Fetch(ctx context.Context, _ string) (*Page, error) {
	select {
	case b.started <- struct{}{}:
	default: // already signalled
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestRun_CancelStopsTheCrawl(t *testing.T) {
	srv := newRobotsServer(t)
	idx := &fakeIndexer{}
	c := New(idx, discardLogger(), testOptions()) // 2 workers
	bf := &blockingFetcher{started: make(chan struct{}, 1)}
	c.fetcher = bf

	// 4 seeds, 2 workers: two fetches block, two URLs wait in the queue.
	seeds := []string{srv.URL + "/1", srv.URL + "/2", srv.URL + "/3", srv.URL + "/4"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errc := make(chan error, 1)
	go func() { errc <- c.Run(ctx, seeds) }()

	select {
	case <-bf.started: // a fetch is now genuinely in flight
	case <-time.After(5 * time.Second):
		t.Fatal("no fetch ever started")
	}
	cancel()

	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel: queued URLs were never drained")
	}
	if len(idx.urls) != 0 {
		t.Errorf("indexed %v, want nothing", idx.urls)
	}
}

func newDisallowAllServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Write([]byte("User-agent: *\nDisallow: /\n"))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_RobotsDisallowedIsCountedNotIndexed(t *testing.T) {
	srv := newDisallowAllServer(t)
	seed := srv.URL + "/"

	idx := &fakeIndexer{}
	c := New(idx, discardLogger(), testOptions())
	c.fetcher = &fakeFetcher{pages: map[string]string{seed: "<html><body>hi</body></html>"}}

	runWithTimeout(t, c, []string{seed})

	if s := c.Stats(); s.Disallowed != 1 || s.Indexed != 0 || s.Failed != 0 {
		t.Errorf("stats = %+v, want disallowed=1 only", s)
	}
	if len(idx.urls) != 0 {
		t.Errorf("indexed %v, want nothing", idx.urls)
	}
}

func TestRun_LogsEachCrawledPage(t *testing.T) {
	srv := newRobotsServer(t)
	seed := srv.URL + "/"

	var buf bytes.Buffer
	c := New(&fakeIndexer{}, slog.New(slog.NewTextHandler(&buf, nil)), testOptions())
	c.fetcher = &fakeFetcher{pages: map[string]string{seed: "<html><body>hi</body></html>"}}

	runWithTimeout(t, c, []string{seed})

	out := buf.String()
	for _, want := range []string{"crawl started", "page crawled", "crawl finished"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "level=DEBUG") {
		t.Errorf("debug records appeared at the default info level:\n%s", out)
	}
}
