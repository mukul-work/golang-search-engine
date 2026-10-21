package crawler

import (
	"context"
	"testing"
	"time"
)

func TestHostLimiter_SpacesRequestsToSameHost(t *testing.T) {
	hl := NewHostLimiter(200 * time.Millisecond)
	if _, err := hl.Wait(context.Background(), "http://a.test/1"); err != nil { // burst token: instant
		t.Fatalf("unexpected error: %v", err)
	}

	waited, err := hl.Wait(context.Background(), "http://a.test/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if waited < 150*time.Millisecond {
		t.Errorf("second request waited %v, want about 200ms", waited)
	}
}

func TestHostLimiter_HostsAreIndependent(t *testing.T) {
	hl := NewHostLimiter(time.Second)
	hl.Wait(context.Background(), "http://a.test/")

	start := time.Now()
	hl.Wait(context.Background(), "http://b.test/") // different host: must not wait for a.test
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("different host waited %v, want no wait", elapsed)
	}
}

func TestHostLimiter_ZeroIntervalDoesNotLimit(t *testing.T) {
	hl := NewHostLimiter(0)

	start := time.Now()
	for i := 0; i < 5; i++ {
		hl.Wait(context.Background(), "http://a.test/")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("5 requests took %v, want no limiting", elapsed)
	}
}

func TestHostLimiter_WaitStopsWhenContextCancelled(t *testing.T) {
	hl := NewHostLimiter(time.Hour)
	if _, err := hl.Wait(context.Background(), "http://a.test/1"); err != nil { // burst token: instant
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	start := time.Now()
	_, err := hl.Wait(ctx, "http://a.test/2") // would otherwise block for an hour
	if err == nil {
		t.Fatal("expected an error after cancel, got nil")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %v, cancel did not interrupt the wait", elapsed)
	}
}
