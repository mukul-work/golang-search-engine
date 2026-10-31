package crawler

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newTask(url string) Task { return Task{URL: url} }

// take mirrors what the frontier does: look at the next task, then remove it.
func take(s *hostScheduler, now time.Time, force bool) (Task, bool) {
	t, ok := s.peek(now, force)
	if ok {
		s.pop(now)
	}
	return t, ok
}

func TestHostKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"http://a.test/x", "a.test"},
		{"https://A.Test:8443/x", "a.test:8443"},
		{"a", ""},
		{"http://%zz", ""},
	}
	for _, tc := range tests {
		if got := hostKey(tc.in); got != tc.want {
			t.Errorf("hostKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestScheduler_SpacesRequestsToOneHost(t *testing.T) {
	s := newHostScheduler(2 * time.Second)
	s.add(newTask("http://a.test/1"))
	s.add(newTask("http://a.test/2"))

	if got, ok := take(s, t0, false); !ok || got.URL != "http://a.test/1" {
		t.Fatalf("first take = %v, %v; want a.test/1", got, ok)
	}
	if _, ok := take(s, t0.Add(time.Second), false); ok {
		t.Error("second URL was handed out after only 1s, want the full 2s")
	}
	if at, ok := s.wakeAt(); !ok || !at.Equal(t0.Add(2*time.Second)) {
		t.Errorf("wakeAt = %v, %v; want t0+2s", at, ok)
	}
	if got, ok := take(s, t0.Add(2*time.Second), false); !ok || got.URL != "http://a.test/2" {
		t.Errorf("take at +2s = %v, %v; want a.test/2", got, ok)
	}
}

func TestScheduler_SmallHostIsNotStuckBehindBigHost(t *testing.T) {
	s := newHostScheduler(time.Second)
	for i := 0; i < 50; i++ {
		s.add(newTask(fmt.Sprintf("http://big.test/%d", i)))
	}
	s.add(newTask("http://small.test/1"))
	s.add(newTask("http://small.test/2"))

	// Two clock ticks, one second apart. A single shared queue would hand
	// out big.test's URLs for all of them and never reach small.test.
	var handedOut []string
	for sec := 0; sec <= 1; sec++ {
		now := t0.Add(time.Duration(sec) * time.Second)
		for {
			got, ok := take(s, now, false)
			if !ok {
				break
			}
			handedOut = append(handedOut, got.URL)
		}
	}

	small := 0
	for _, u := range handedOut {
		if strings.Contains(u, "small.test") {
			small++
		}
	}
	if small != 2 || len(handedOut) != 4 {
		t.Errorf("handed out %v; want 2 small.test and 2 big.test URLs in two ticks", handedOut)
	}
}

func TestScheduler_NeverBreaksTheInterval(t *testing.T) {
	const interval = 500 * time.Millisecond
	s := newHostScheduler(interval)
	for i := 0; i < 20; i++ {
		for _, h := range []string{"a.test", "b.test", "c.test"} {
			s.add(newTask(fmt.Sprintf("http://%s/%d", h, i)))
		}
	}

	last := map[string]time.Time{}
	total := 0
	for now := t0; total < 60 && now.Before(t0.Add(time.Hour)); now = now.Add(100 * time.Millisecond) {
		for {
			got, ok := take(s, now, false)
			if !ok {
				break
			}
			total++
			host := hostKey(got.URL)
			if prev, seen := last[host]; seen && now.Sub(prev) < interval {
				t.Fatalf("%s was contacted again after %v, want at least %v", host, now.Sub(prev), interval)
			}
			last[host] = now
		}
	}
	if total != 60 {
		t.Errorf("handed out %d URLs, want all 60", total)
	}
}

func TestScheduler_RemembersCooldownWhenQueueEmpties(t *testing.T) {
	s := newHostScheduler(2 * time.Second)
	s.add(newTask("http://a.test/1"))
	take(s, t0, false)                // a.test is cooling down and its queue is empty
	s.add(newTask("http://a.test/2")) // a new URL arrives for the same host

	if _, ok := take(s, t0.Add(time.Second), false); ok {
		t.Error("a recently used host was served early: its cooldown was forgotten")
	}
	if _, ok := take(s, t0.Add(2*time.Second), false); !ok {
		t.Error("the URL should be ready once the cooldown is over")
	}
}

func TestScheduler_ForceIgnoresCooldown(t *testing.T) {
	s := newHostScheduler(time.Hour)
	for i := 0; i < 3; i++ {
		s.add(newTask(fmt.Sprintf("http://a.test/%d", i)))
	}
	if _, ok := take(s, t0, false); !ok {
		t.Fatal("the first URL should be immediate")
	}
	if _, ok := take(s, t0, false); ok {
		t.Fatal("the second URL should have to wait an hour")
	}

	drained := 0
	for {
		if _, ok := take(s, t0, true); !ok {
			break
		}
		drained++
	}
	if drained != 2 {
		t.Errorf("force released %d URLs, want the 2 that were waiting", drained)
	}
}
