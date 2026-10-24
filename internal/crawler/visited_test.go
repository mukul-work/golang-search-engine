package crawler

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestVisitedSet_ConcurrentUse(t *testing.T) {
	v := NewVisitedSet(500)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v.Add(fmt.Sprintf("u%d", i%10)) // 10 distinct URLs, hit 5 times each
			v.Count()
		}(i)
	}
	wg.Wait()
	if got := v.Count(); got != 10 {
		t.Errorf("Count = %d, want 10", got)
	}
}

func TestVisitedSet_Add(t *testing.T) {
	v := NewVisitedSet(2)
	if got := v.Add("a"); got != Added {
		t.Errorf("first a: got %v, want Added", got)
	}
	if got := v.Add("a"); got != Duplicate {
		t.Errorf("second a: got %v, want Duplicate", got)
	}
	if got := v.Add("b"); got != Added {
		t.Errorf("b: got %v, want Added", got)
	}
	if got := v.Add("c"); got != OverLimit {
		t.Errorf("c: got %v, want OverLimit", got)
	}
	if got := v.Add("a"); got != Duplicate { // known URLs stay duplicates even when full
		t.Errorf("a when full: got %v, want Duplicate", got)
	}
	if got := v.Count(); got != 2 {
		t.Errorf("Count = %d, want 2", got)
	}
}

func TestVisitedSet_LimitHoldsUnderConcurrency(t *testing.T) {
	v := NewVisitedSet(10)
	var added atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if v.Add(fmt.Sprintf("u%d", i)) == Added {
				added.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if got := added.Load(); got != 10 {
		t.Errorf("%d goroutines were told Added, want exactly 10", got)
	}
	if got := v.Count(); got != 10 {
		t.Errorf("Count = %d, want 10", got)
	}
}
