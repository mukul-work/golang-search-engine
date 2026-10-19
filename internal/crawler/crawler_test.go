package crawler

import (
	"testing"
	"time"
)

func TestEnqueue_RejectsDuplicates(t *testing.T) {
	c := New(nil, 1)

	if !c.enqueue("https://example.com/a") {
		t.Error("first enqueue should be accepted")
	}
	if c.enqueue("https://example.com/a") {
		t.Error("duplicate enqueue should be rejected")
	}
	if got := c.VisitedCount(); got != 1 {
		t.Errorf("VisitedCount = %d, want 1", got)
	}
}

func TestEnqueue_WaitGroupMatchesAcceptedURLs(t *testing.T) {
	c := New(nil, 1)
	c.enqueue("https://example.com/a")
	c.enqueue("https://example.com/a") // duplicate: must NOT touch the counter
	c.enqueue("https://example.com/b")

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	// Act as a worker: take the two accepted URLs and mark them done.
	for i := 0; i < 2; i++ {
		<-c.frontier.out
		c.wg.Done()
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitGroup never reached zero: counter and queue disagree")
	}
}
