package crawler

import (
	"fmt"
	"sync"
	"testing"
)

func TestVisitedSet_ConcurrentUse(t *testing.T) {
	v := NewVisitedSet()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v.IsVisited(fmt.Sprintf("u%d", i%10)) // 10 distinct URLs, hit 5 times each
			v.Count()
		}(i)
	}
	wg.Wait()
	if got := v.Count(); got != 10 {
		t.Errorf("Count = %d, want 10", got)
	}
}
