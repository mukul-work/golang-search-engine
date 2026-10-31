package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mukul-work/golang-search-engine/internal/search"
)

var rlBase = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// ---- the token bucket, on a fake clock ----

func TestRateLimiter_BurstThenBlocked(t *testing.T) {
	l := NewRateLimiter(60, 3) // one token per second, bucket holds 3
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("a", rlBase); !ok {
			t.Fatalf("request %d of the burst was denied", i+1)
		}
	}
	ok, wait := l.Allow("a", rlBase)
	if ok {
		t.Fatal("the 4th immediate request was allowed, want it denied")
	}
	if wait != time.Second {
		t.Errorf("wait = %v, want 1s", wait)
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	l := NewRateLimiter(60, 2)
	l.Allow("a", rlBase)
	l.Allow("a", rlBase) // the bucket is now empty

	if ok, wait := l.Allow("a", rlBase.Add(500*time.Millisecond)); ok || wait != 500*time.Millisecond {
		t.Errorf("after 0.5s: allowed=%v wait=%v, want denied with 500ms to go", ok, wait)
	}
	if ok, _ := l.Allow("a", rlBase.Add(time.Second)); !ok {
		t.Error("after 1s a token should have dripped back in")
	}
	if ok, _ := l.Allow("a", rlBase.Add(time.Second)); ok {
		t.Error("only one token had refilled, but a second request was allowed")
	}
}

func TestRateLimiter_RefillStopsAtBurst(t *testing.T) {
	l := NewRateLimiter(60, 3)
	l.Allow("a", rlBase)
	later := rlBase.Add(time.Hour)

	allowed := 0
	for i := 0; i < 10; i++ {
		if ok, _ := l.Allow("a", later); ok {
			allowed++
		}
	}
	if allowed != 3 {
		t.Errorf("%d requests allowed after an hour idle, want exactly the burst of 3", allowed)
	}
}

func TestRateLimiter_KeysAreIndependent(t *testing.T) {
	l := NewRateLimiter(60, 1)
	l.Allow("a", rlBase)
	if ok, _ := l.Allow("a", rlBase); ok {
		t.Fatal("a should be out of tokens")
	}
	if ok, _ := l.Allow("b", rlBase); !ok {
		t.Error("b was denied because of a's requests")
	}
}

func TestRateLimiter_ClockGoingBackwardsEarnsNothing(t *testing.T) {
	l := NewRateLimiter(60, 1)
	l.Allow("a", rlBase.Add(10*time.Second)) // spends the only token
	if ok, _ := l.Allow("a", rlBase); ok {
		t.Error("a request was allowed after the clock jumped back")
	}
}

func TestRateLimiter_SweepForgetsIdleClientsButKeepsActiveOnes(t *testing.T) {
	l := NewRateLimiter(6, 2) // 0.1 token per second: an empty bucket takes 20s to refill
	for i := 0; i < 100; i++ {
		l.Allow(fmt.Sprintf("ip-%d", i), rlBase) // each spends a token, so none is full
	}
	if got := l.size(); got != 100 {
		t.Fatalf("size = %d, want 100", got)
	}

	// "busy" empties its bucket half a second before the sweep.
	l.Allow("busy", rlBase.Add(119*time.Second+500*time.Millisecond))
	l.Allow("busy", rlBase.Add(119*time.Second+500*time.Millisecond))

	l.Allow("fresh", rlBase.Add(2*time.Minute)) // the sweep runs first
	if got := l.size(); got != 2 {
		t.Errorf("size = %d, want 2: the 100 idle clients forgotten, busy and fresh kept", got)
	}
}

func TestRateLimiter_MemoryIsBounded(t *testing.T) {
	l := NewRateLimiter(60, 2)
	l.maxKeys = 5
	for i := 0; i < 50; i++ {
		l.Allow(fmt.Sprintf("ip-%d", i), rlBase)
	}
	if got := l.size(); got > 5 {
		t.Errorf("size = %d, want at most 5", got)
	}
}

func TestRateLimiter_ConcurrentCallsNeverExceedBurst(t *testing.T) {
	l := NewRateLimiter(60, 10)
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Allow("same", rlBase); ok { // same instant, so nothing refills
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != 10 {
		t.Errorf("%d concurrent requests allowed, want exactly the burst of 10", got)
	}
}

// ---- who is the client? ----

func TestClientKey(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name    string
		remote  string
		xff     []string // each element is one X-Forwarded-For header line
		trusted []netip.Prefix
		want    string
	}{
		{"no proxies: the header is ignored", "203.0.113.9:4000", []string{"1.2.3.4"}, nil, "203.0.113.9"},
		{"trusted proxy: the header is believed", "172.18.0.2:5000", []string{"203.0.113.9"}, proxies, "203.0.113.9"},
		{"forged entries on the left are ignored", "172.18.0.2:5000", []string{"6.6.6.6, 203.0.113.9"}, proxies, "203.0.113.9"},
		{"two of our own proxies in the chain", "172.18.0.2:5000", []string{"203.0.113.9, 10.0.0.5"}, proxies, "203.0.113.9"},
		{"header split over two lines", "172.18.0.2:5000", []string{"6.6.6.6", "203.0.113.9"}, proxies, "203.0.113.9"},
		{"trusted proxy but no header", "172.18.0.2:5000", nil, proxies, "172.18.0.2"},
		{"trusted proxy but garbage header", "172.18.0.2:5000", []string{"not-an-ip"}, proxies, "172.18.0.2"},
		{"untrusted sender with a header", "198.51.100.7:1", []string{"203.0.113.9"}, proxies, "198.51.100.7"},
		{"ipv6 is grouped by /64", "[2001:db8:1:2:aaaa:bbbb:cccc:dddd]:443", nil, nil, "2001:db8:1:2::/64"},
		{"ipv4-mapped ipv6 is plain ipv4", "[::ffff:203.0.113.9]:80", nil, nil, "203.0.113.9"},
		{"address without a port", "203.0.113.9", nil, nil, "203.0.113.9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			for _, h := range tc.xff {
				r.Header.Add("X-Forwarded-For", h)
			}
			if got := clientKey(r, tc.trusted); got != tc.want {
				t.Errorf("clientKey = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---- through the server ----

func doFrom(h http.Handler, remote, target string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.RemoteAddr = remote
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// limitedHandler builds a server allowing burst requests per client and then
// one a minute, so a test's own requests never refill the bucket in any
// meaningful way.
func limitedHandler(burst int, opts ...Option) (http.Handler, *fakeSearcher) {
	fs := &fakeSearcher{res: search.Results{Query: "go", Page: 1, PerPage: 10}}
	all := append([]Option{WithRateLimit(NewRateLimiter(1, burst))}, opts...)
	return New(fs, discardLogger(), all...).Handler(), fs
}

func TestRateLimit_APIGets429WithRetryAfter(t *testing.T) {
	h, fs := limitedHandler(2)
	for i := 0; i < 2; i++ {
		if rec := doFrom(h, "198.51.100.1:1000", "/api/search?q=go"); rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d, want 200", i+1, rec.Code)
		}
	}

	rec := doFrom(h, "198.51.100.1:1000", "/api/search?q=go")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	secs, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	if err != nil || secs < 1 || secs > 60 {
		t.Errorf("Retry-After = %q, want whole seconds between 1 and 60", rec.Header().Get("Retry-After"))
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
		t.Errorf("want a JSON error body, got: %s", rec.Body.String())
	}
	if fs.calls != 2 {
		t.Errorf("searcher called %d times, want 2: a rejected request must not reach it", fs.calls)
	}
}

func TestRateLimit_PageGets429Page(t *testing.T) {
	h, _ := limitedHandler(1)
	doFrom(h, "198.51.100.1:1000", "/?q=go")

	rec := doFrom(h, "198.51.100.1:1000", "/?q=go")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want an HTML page", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Too many requests") || !strings.Contains(body, `value="go"`) {
		t.Errorf("want a message and the search box still filled in:\n%s", body)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header")
	}
}

func TestRateLimit_ClientsAreIndependent(t *testing.T) {
	h, _ := limitedHandler(1)
	doFrom(h, "198.51.100.1:1", "/api/search?q=go")

	if rec := doFrom(h, "198.51.100.1:1", "/api/search?q=go"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("first client: status %d, want 429", rec.Code)
	}
	if rec := doFrom(h, "198.51.100.2:1", "/api/search?q=go"); rec.Code != http.StatusOK {
		t.Errorf("second client: status %d, want 200", rec.Code)
	}
}

func TestRateLimit_HealthAndStaticAreExempt(t *testing.T) {
	h, _ := limitedHandler(1)
	for i := 0; i < 5; i++ {
		for _, path := range []string{"/healthz", "/static/style.css"} {
			if rec := doFrom(h, "198.51.100.1:1", path); rec.Code != http.StatusOK {
				t.Fatalf("%s: status %d, want 200 (never limited)", path, rec.Code)
			}
		}
	}
}

func TestRateLimit_BehindAProxyLimitsTheRealClient(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12")}
	h, _ := limitedHandler(1, WithTrustedProxies(proxies))
	const proxy = "172.18.0.2:5000"

	if rec := doFrom(h, proxy, "/api/search?q=go", "X-Forwarded-For", "203.0.113.1"); rec.Code != http.StatusOK {
		t.Fatalf("client 1, first request: status %d, want 200", rec.Code)
	}
	if rec := doFrom(h, proxy, "/api/search?q=go", "X-Forwarded-For", "203.0.113.2"); rec.Code != http.StatusOK {
		t.Errorf("client 2 was throttled: every visitor behind the proxy shares one bucket")
	}
	if rec := doFrom(h, proxy, "/api/search?q=go", "X-Forwarded-For", "203.0.113.1"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("client 1, second request: status %d, want 429", rec.Code)
	}
}

func TestRateLimit_ForgedHeaderFromAnUntrustedClientIsIgnored(t *testing.T) {
	h, _ := limitedHandler(1) // no trusted proxies configured
	doFrom(h, "198.51.100.1:1", "/api/search?q=go", "X-Forwarded-For", "1.1.1.1")

	rec := doFrom(h, "198.51.100.1:1", "/api/search?q=go", "X-Forwarded-For", "2.2.2.2")
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429: a forged X-Forwarded-For must not buy a fresh allowance", rec.Code)
	}
}

func TestRateLimit_OffUnlessConfigured(t *testing.T) {
	fs := &fakeSearcher{res: search.Results{Query: "go", Page: 1, PerPage: 10}}
	h := New(fs, discardLogger()).Handler()
	for i := 0; i < 50; i++ {
		if rec := doFrom(h, "198.51.100.1:1", "/api/search?q=go"); rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d, want 200 with no limiter", i+1, rec.Code)
		}
	}
}
