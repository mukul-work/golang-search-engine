package crawler

import "sync"

// AddResult says what Add did with a URL.
type AddResult int

const (
	Added     AddResult = iota // new URL, now recorded
	Duplicate                  // already recorded
	OverLimit                  // new URL, but the set is full
)

// VisitedSet records URLs that have been queued. A limit above zero caps how
// many distinct URLs it will accept.
type VisitedSet struct {
	mu      sync.Mutex
	visited map[string]struct{}
	limit   int // 0 = unlimited
}

func NewVisitedSet(limit int) *VisitedSet {
	return &VisitedSet{visited: make(map[string]struct{}), limit: limit}
}
func (v *VisitedSet) Count() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.visited)
}

// Add records url unless it is already known or the set is full. The check
// and the insert happen under one lock, so the limit can never be overshot.
func (v *VisitedSet) Add(url string) AddResult {
	v.mu.Lock()
	defer v.mu.Unlock()

	if _, exists := v.visited[url]; exists {
		return Duplicate
	}
	if v.limit > 0 && len(v.visited) >= v.limit {
		return OverLimit
	}
	v.visited[url] = struct{}{}
	return Added
}
