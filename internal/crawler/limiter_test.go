package crawler

import (
	"testing"
	"time"
)

func TestHostLimiter_SpacesRequestsToSameHost(t *testing.T) {
	hl := NewHostLimiter(200 * time.Millisecond)
	if err := hl.Wait("http://a.test/1"); err != nil { // uses the burst token: instant
		t.Fatalf("unexpected error: %v", err)
	}

	start := time.Now()
	if err := hl.Wait("http://a.test/2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("second request waited %v, want about 200ms", elapsed)
	}
}

func TestHostLimiter_HostsAreIndependent(t *testing.T) {
	hl := NewHostLimiter(time.Second)
	hl.Wait("http://a.test/")

	start := time.Now()
	hl.Wait("http://b.test/") // different host: must not wait for a.test
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("different host waited %v, want no wait", elapsed)
	}
}

func TestHostLimiter_ZeroIntervalDoesNotLimit(t *testing.T) {
	hl := NewHostLimiter(0)

	start := time.Now()
	for i := 0; i < 5; i++ {
		hl.Wait("http://a.test/")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("5 requests took %v, want no limiting", elapsed)
	}
}
