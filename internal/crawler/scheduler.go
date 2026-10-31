package crawler

import (
	"container/heap"
	"net/url"
	"strings"
	"time"
)

// hostKey returns the scheduling key for a URL: its lowercase host:port.
// A URL that cannot be parsed, or has no host, shares the empty key.
func hostKey(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// hostQueue holds the URLs waiting for one host and when that host may next
// be contacted.
type hostQueue struct {
	tasks []Task
	next  time.Time // earliest time the next request to this host may start
}

// hostHeap is a min-heap of hosts ordered by next, so index 0 is the host
// that becomes ready first. It implements heap.Interface.
type hostHeap []*hostQueue

func (h hostHeap) Len() int           { return len(h) }
func (h hostHeap) Less(i, j int) bool { return h[i].next.Before(h[j].next) }
func (h hostHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *hostHeap) Push(x any)        { *h = append(*h, x.(*hostQueue)) }
func (h *hostHeap) Pop() any {
	old := *h
	n := len(old)
	hq := old[n-1]
	old[n-1] = nil // let the garbage collector have it
	*h = old[:n-1]
	return hq
}

// hostScheduler decides which URL may start next so that no host is contacted
// more often than once per interval, without one busy host holding up the
// others. It is not safe for concurrent use: only the frontier goroutine
// touches it, and it takes the time as a parameter so tests need no sleeping.
//
// Invariant: a host is in the heap if and only if it has URLs waiting.
type hostScheduler struct {
	interval time.Duration
	hosts    map[string]*hostQueue // every host seen, with its cooldown
	ready    hostHeap
}

func newHostScheduler(interval time.Duration) *hostScheduler {
	return &hostScheduler{interval: interval, hosts: make(map[string]*hostQueue)}
}

// add queues t behind the other URLs of its host.
func (s *hostScheduler) add(t Task) {
	key := hostKey(t.URL)
	hq, ok := s.hosts[key]
	if !ok {
		hq = &hostQueue{} // zero next: a new host is ready immediately
		s.hosts[key] = hq
	}
	hq.tasks = append(hq.tasks, t)
	if len(hq.tasks) == 1 { // the host was idle and now has work: it joins the heap
		heap.Push(&s.ready, hq)
	}
}

// peek returns the task that may start at now, without removing it. With
// force, cooldowns are ignored (used when draining after a cancel).
func (s *hostScheduler) peek(now time.Time, force bool) (Task, bool) {
	if len(s.ready) == 0 {
		return Task{}, false
	}
	hq := s.ready[0]
	if !force && hq.next.After(now) {
		return Task{}, false
	}
	return hq.tasks[0], true
}

// wakeAt returns when the earliest waiting host becomes ready.
func (s *hostScheduler) wakeAt() (time.Time, bool) {
	if len(s.ready) == 0 {
		return time.Time{}, false
	}
	return s.ready[0].next, true
}

// pop removes the task that peek just returned and starts its host's
// cooldown. It must only be called after peek returned true, with nothing
// changed in between.
func (s *hostScheduler) pop(now time.Time) {
	hq := s.ready[0]
	hq.tasks[0] = Task{} // release the URL string for garbage collection
	hq.tasks = hq.tasks[1:]
	hq.next = now.Add(s.interval)
	if len(hq.tasks) == 0 {
		hq.tasks = nil
		heap.Pop(&s.ready) // removes the root, which is hq
	} else {
		heap.Fix(&s.ready, 0) // hq's next moved later: sift it down
	}
}
