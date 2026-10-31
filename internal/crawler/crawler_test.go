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
		MaxDepth:       5,
		MaxPages:       1000,
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

	if !c.enqueue("https://example.com/a", 0) {
		t.Error("first enqueue should be accepted")
	}
	if c.enqueue("https://example.com/a", 0) {
		t.Error("duplicate enqueue should be rejected")
	}
	if got := c.VisitedCount(); got != 1 {
		t.Errorf("VisitedCount = %d, want 1", got)
	}
}

func TestEnqueue_WaitGroupMatchesAcceptedURLs(t *testing.T) {
	c := New(nil, discardLogger(), testOptions())
	c.enqueue("https://example.com/a", 0)
	c.enqueue("https://example.com/a", 0) // duplicate: must NOT touch the counter
	c.enqueue("https://example.com/b", 0)

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

func TestEnqueue_NormalizesBeforeDedup(t *testing.T) {
	c := New(nil, discardLogger(), testOptions())

	if !c.enqueue("https://example.com/a#x", 0) {
		t.Error("first spelling should be accepted")
	}
	if c.enqueue("HTTPS://EXAMPLE.COM:443/a?utm_source=mail", 0) {
		t.Error("same page under another spelling should be rejected")
	}
	if got := c.VisitedCount(); got != 1 {
		t.Errorf("VisitedCount = %d, want 1", got)
	}
}

func TestEnqueue_RejectsUnusableURLs(t *testing.T) {
	c := New(nil, discardLogger(), testOptions())
	for _, raw := range []string{"mailto:a@b.com", "javascript:void(0)", "tel:+123456", "ftp://example.com/f", "/relative", ""} {
		if c.enqueue(raw, 0) {
			t.Errorf("enqueue(%q) = true, want false", raw)
		}
	}
	if got := c.VisitedCount(); got != 0 {
		t.Errorf("VisitedCount = %d, want 0: junk must not enter the visited set", got)
	}

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitGroup is non-zero: a rejected URL was counted")
	}
}

func TestRun_VariantLinksAreCrawledOnce(t *testing.T) {
	srv := newRobotsServer(t)
	seed := srv.URL // no trailing slash on purpose: normalized to srv.URL + "/"
	home, a := srv.URL+"/", srv.URL+"/a"

	idx := &fakeIndexer{}
	c := New(idx, discardLogger(), testOptions())
	c.fetcher = &fakeFetcher{pages: map[string]string{
		home: `<html><body>
			<a href="/a#top">1</a>
			<a href="/a?utm_source=mail">2</a>
			<a href="/a">3</a>
			<a href="mailto:x@y.z">4</a>
			<a href="javascript:void(0)">5</a>
		</body></html>`,
		a: `<html><body>second page</body></html>`,
	}}

	runWithTimeout(t, c, []string{seed})

	sort.Strings(idx.urls)
	if want := []string{home, a}; !reflect.DeepEqual(idx.urls, want) {
		t.Errorf("indexed %v, want %v", idx.urls, want)
	}
	if s := c.Stats(); s.Seen != 2 || s.Indexed != 2 || s.Failed != 0 {
		t.Errorf("stats = %+v, want seen=2 indexed=2 failed=0", s)
	}
}

func TestRun_MaxDepth(t *testing.T) {
	srv := newRobotsServer(t)
	home, a, b := srv.URL+"/", srv.URL+"/a", srv.URL+"/b"
	pages := map[string]string{
		home: `<html><body><a href="/a">a</a></body></html>`,
		a:    `<html><body><a href="/b">b</a></body></html>`,
		b:    `<html><body>end of the chain</body></html>`,
	}

	tests := []struct {
		name  string
		depth int
		want  []string
	}{
		{"seeds only", 0, []string{home}},
		{"one hop", 1, []string{home, a}},
		{"two hops", 2, []string{home, a, b}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx := &fakeIndexer{}
			opts := testOptions()
			opts.MaxDepth = tc.depth
			c := New(idx, discardLogger(), opts)
			c.fetcher = &fakeFetcher{pages: pages}

			runWithTimeout(t, c, []string{home})

			sort.Strings(idx.urls)
			if !reflect.DeepEqual(idx.urls, tc.want) {
				t.Errorf("indexed %v, want %v", idx.urls, tc.want)
			}
		})
	}
}

func TestRun_MaxPagesBudget(t *testing.T) {
	srv := newRobotsServer(t)
	home := srv.URL + "/"
	pages := map[string]string{
		home: `<html><body><a href="/a">a</a><a href="/b">b</a><a href="/c">c</a><a href="/d">d</a></body></html>`,
	}
	for _, p := range []string{"/a", "/b", "/c", "/d"} {
		pages[srv.URL+p] = `<html><body>leaf</body></html>`
	}

	idx := &fakeIndexer{}
	opts := testOptions()
	opts.MaxPages = 3 // the seed plus two links
	c := New(idx, discardLogger(), opts)
	c.fetcher = &fakeFetcher{pages: pages}

	runWithTimeout(t, c, []string{home})

	if s := c.Stats(); s.Seen != 3 || s.Indexed != 3 || s.OutOfScope != 2 || s.Failed != 0 {
		t.Errorf("stats = %+v, want seen=3 indexed=3 out_of_scope=2 failed=0", s)
	}
}

func TestRun_AllowedHostsKeepsCrawlOnSite(t *testing.T) {
	srv := newRobotsServer(t)
	home, a := srv.URL+"/", srv.URL+"/a"

	idx := &fakeIndexer{}
	opts := testOptions()
	opts.AllowedHosts = []string{"127.0.0.1"} // httptest servers listen on 127.0.0.1
	c := New(idx, discardLogger(), opts)
	c.fetcher = &fakeFetcher{pages: map[string]string{
		home: `<html><body><a href="/a">a</a><a href="https://other.test/x">off site</a></body></html>`,
		a:    `<html><body>second page</body></html>`,
	}}

	runWithTimeout(t, c, []string{home})

	sort.Strings(idx.urls)
	if want := []string{home, a}; !reflect.DeepEqual(idx.urls, want) {
		t.Errorf("indexed %v, want %v", idx.urls, want)
	}
	// If the off-site link had been queued, the fake fetcher would fail on it.
	if s := c.Stats(); s.Failed != 0 || s.OutOfScope != 1 {
		t.Errorf("stats = %+v, want failed=0 out_of_scope=1", s)
	}
}

func TestRun_ErrorsWhenNoSeedIsInScope(t *testing.T) {
	opts := testOptions()
	opts.AllowedHosts = []string{"allowed.test"}
	c := New(&fakeIndexer{}, discardLogger(), opts)
	c.fetcher = &fakeFetcher{}

	errc := make(chan error, 1)
	go func() { errc <- c.Run(context.Background(), []string{"https://other.test/"}) }()

	select {
	case err := <-errc:
		if err == nil {
			t.Error("expected an error when no seed is queued, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestEnqueue_StarAllowsAnyHost(t *testing.T) {
	opts := testOptions()
	opts.AllowedHosts = []string{"*"}
	c := New(nil, discardLogger(), opts)

	if !c.enqueue("https://anything.test/x", 0) {
		t.Error(`with "*" any host should be accepted`)
	}
}

func TestRun_StaysOnSeedHostsByDefault(t *testing.T) {
	srv := newRobotsServer(t)
	home, a := srv.URL+"/", srv.URL+"/a"

	idx := &fakeIndexer{}
	c := New(idx, discardLogger(), testOptions()) // no AllowedHosts: derived from the seed
	c.fetcher = &fakeFetcher{pages: map[string]string{
		home: `<html><body><a href="/a">a</a><a href="https://other.test/x">elsewhere</a></body></html>`,
		a:    `<html><body>second page</body></html>`,
	}}

	runWithTimeout(t, c, []string{home})

	sort.Strings(idx.urls)
	if want := []string{home, a}; !reflect.DeepEqual(idx.urls, want) {
		t.Errorf("indexed %v, want %v", idx.urls, want)
	}
	// The off-site link must be dropped before it is queued.
	if s := c.Stats(); s.Seen != 2 || s.OutOfScope != 1 || s.Failed != 0 {
		t.Errorf("stats = %+v, want seen=2 out_of_scope=1 failed=0", s)
	}
}

func TestRun_CancelDuringCooldownStillFinishes(t *testing.T) {
	srv := newRobotsServer(t)
	opts := testOptions()
	opts.HostInterval = time.Hour // after the first URL, the host is closed for an hour
	c := New(&fakeIndexer{}, discardLogger(), opts)
	bf := &blockingFetcher{started: make(chan struct{}, 1)}
	c.fetcher = bf

	seeds := []string{srv.URL + "/1", srv.URL + "/2", srv.URL + "/3"} // one host
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errc := make(chan error, 1)
	go func() { errc <- c.Run(ctx, seeds) }()

	select {
	case <-bf.started: // one fetch is in flight; the other two wait out their cooldown
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
		t.Fatal("Run did not return: URLs waiting out their cooldown were never released")
	}
}
