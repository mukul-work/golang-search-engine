package server

import (
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sweepEvery     = time.Minute
	defaultMaxKeys = 100_000
)

// bucket is one client's allowance: it holds up to burst tokens, each request
// spends one, and tokens drip back in at the limiter's rate.
type bucket struct {
	tokens float64
	last   time.Time // when tokens was last brought up to date
}

// RateLimiter is a token-bucket limiter keyed by client. It is safe for
// concurrent use and takes the time as a parameter, so tests need no sleeping.
type RateLimiter struct {
	mu        sync.Mutex
	rate      float64 // tokens added per second
	burst     float64 // bucket capacity
	buckets   map[string]*bucket
	lastSweep time.Time
	maxKeys   int
}

// NewRateLimiter allows perMinute requests per minute per key on average, with
// bursts of up to burst. Values below 1 are raised to 1.
func NewRateLimiter(perMinute, burst int) *RateLimiter {
	if perMinute < 1 {
		perMinute = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &RateLimiter{
		rate:    float64(perMinute) / 60,
		burst:   float64(burst),
		buckets: make(map[string]*bucket),
		maxKeys: defaultMaxKeys,
	}
}

// Allow spends a token for key if one is available. If not, it reports how
// long until one will be.
func (l *RateLimiter) Allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.sweep(now)

	b, ok := l.buckets[key]
	if ok {
		l.refill(b, now)
	} else {
		if len(l.buckets) >= l.maxKeys {
			for k := range l.buckets { // map order is random: this drops an arbitrary entry
				delete(l.buckets, k)
				break
			}
		}
		b = &bucket{tokens: l.burst, last: now} // a new client starts with a full bucket
		l.buckets[key] = b
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
}

// refill adds the tokens earned since b.last, up to the capacity. A clock that
// went backwards earns nothing.
func (l *RateLimiter) refill(b *bucket, now time.Time) {
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = math.Min(l.burst, b.tokens+elapsed*l.rate)
		b.last = now
	}
}

// sweep forgets clients whose bucket has refilled to full: a full bucket is
// exactly what a brand-new client gets, so nothing is lost. The caller holds mu.
func (l *RateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepEvery {
		return
	}
	l.lastSweep = now
	for k, b := range l.buckets { // deleting during a range is safe in Go
		l.refill(b, now)
		if b.tokens >= l.burst {
			delete(l.buckets, k)
		}
	}
}

// size reports how many clients are tracked. It exists for tests.
func (l *RateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// clientKey returns the key to rate limit r by. X-Forwarded-For is believed
// only when the connection comes from one of the trusted proxies, and is then
// read from the right, because entries on the left were supplied by the client.
func clientKey(r *http.Request, trusted []netip.Prefix) string {
	remote, ok := parseRemote(r.RemoteAddr)
	if !ok {
		return r.RemoteAddr // not an address we understand: still a stable key
	}
	if !isTrusted(remote, trusted) {
		return keyFor(remote)
	}

	parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part == "" {
			continue
		}
		ip, err := netip.ParseAddr(part)
		if err != nil {
			break // garbage: stop trusting the header
		}
		ip = normalize(ip)
		if isTrusted(ip, trusted) {
			continue // another of our own proxies: keep walking left
		}
		return keyFor(ip)
	}
	return keyFor(remote) // no usable header: the proxy itself is all we know
}

func parseRemote(s string) (netip.Addr, bool) {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return normalize(ap.Addr()), true
	}
	if a, err := netip.ParseAddr(s); err == nil {
		return normalize(a), true
	}
	return netip.Addr{}, false
}

// normalize turns ::ffff:1.2.3.4 into 1.2.3.4 and drops any IPv6 zone, so
// addresses compare and match prefixes correctly.
func normalize(ip netip.Addr) netip.Addr {
	return ip.Unmap().WithZone("")
}

func isTrusted(ip netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// keyFor groups IPv6 addresses by their /64, the unit one subscriber controls.
func keyFor(ip netip.Addr) string {
	if ip.Is6() {
		if p, err := ip.Prefix(64); err == nil {
			return p.String()
		}
	}
	return ip.String()
}

// limited wraps next so that a client over its allowance gets a 429 written
// by deny, with a Retry-After header, and next is never called.
func (s *Server) limited(deny, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.limiter != nil {
			if ok, wait := s.limiter.Allow(clientKey(r, s.trusted), time.Now()); !ok {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(wait)))
				deny(w, r)
				return
			}
		}
		next(w, r)
	}
}

func retryAfterSeconds(d time.Duration) int {
	n := int(math.Ceil(d.Seconds()))
	if n < 1 {
		n = 1
	}
	return n
}

func (s *Server) denyAPI(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusTooManyRequests, "too many requests, slow down")
}

func (s *Server) denyPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, http.StatusTooManyRequests, pageData{
		Query: strings.TrimSpace(r.URL.Query().Get("q")), // keep what they typed; the template escapes it
		Error: "Too many requests. Please wait a moment and try again.",
	})
}
