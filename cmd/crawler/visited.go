package crawler

import "sync"

type VisitedSet struct {
	mu      sync.Mutex
	visited map[string]struct{}
}

func NewVisitedSet() *VisitedSet {
	return &VisitedSet{
		mu:      sync.Mutex{},
		visited: make(map[string]struct{}),
	}
}

func (v *VisitedSet) IsVisited(url string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	if _, exists := v.visited[url]; exists {
		return true
	}
	v.visited[url] = struct{}{}
	return false
}
