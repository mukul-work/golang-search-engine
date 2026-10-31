package crawler

import (
	"fmt"
	"testing"
	"time"
)

// recvTask receives from the frontier or fails the test after a timeout.
func recvTask(t *testing.T, f *Frontier) Task {
	t.Helper()
	select {
	case got, ok := <-f.out:
		if !ok {
			t.Fatal("frontier closed unexpectedly")
		}
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a task from the frontier")
		return Task{}
	}
}

func TestFrontier_CloseEndsReceivers(t *testing.T) {
	f := NewFrontier(0)
	f.Push(Task{URL: "a", Depth: 3})
	if got := recvTask(t, f); got != (Task{URL: "a", Depth: 3}) {
		t.Fatalf("got %+v, want the pushed task with its depth", got)
	}

	f.Close()

	select {
	case _, ok := <-f.out:
		if ok {
			t.Error("out delivered a value, want it closed")
		}
	case <-time.After(time.Second):
		t.Fatal("out was not closed after Close")
	}
}

func TestFrontier_SpacesRequestsToOneHost(t *testing.T) {
	const interval = 150 * time.Millisecond
	f := NewFrontier(interval)
	defer f.Close()
	f.Push(Task{URL: "http://a.test/1"})
	f.Push(Task{URL: "http://a.test/2"})

	recvTask(t, f)
	start := time.Now()
	recvTask(t, f)
	if gap := time.Since(start); gap < interval-50*time.Millisecond { // generous: a lower bound only
		t.Errorf("second URL arrived after %v, want about %v", gap, interval)
	}
}

func TestFrontier_SmallHostIsNotStuckBehindBigHost(t *testing.T) {
	f := NewFrontier(200 * time.Millisecond)
	defer f.Close()
	for i := 0; i < 20; i++ {
		f.Push(Task{URL: fmt.Sprintf("http://big.test/%d", i)})
	}
	f.Push(Task{URL: "http://small.test/1"})

	// A single shared queue would only reach small.test after all 20 big.test
	// URLs, about 4 seconds from now.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case got := <-f.out:
			if got.URL == "http://small.test/1" {
				return
			}
		case <-deadline:
			t.Fatal("small.test's URL was stuck behind big.test's queue")
		}
	}
}

func TestFrontier_DrainIgnoresCooldown(t *testing.T) {
	f := NewFrontier(time.Hour)
	defer f.Close()
	for i := 0; i < 3; i++ {
		f.Push(Task{URL: fmt.Sprintf("http://a.test/%d", i)})
	}

	recvTask(t, f) // the first is immediate; the other two now wait an hour
	f.Drain()
	f.Drain() // safe to call twice
	recvTask(t, f)
	recvTask(t, f)
}
