package crawler

import (
	"testing"
	"time"
)

func TestFrontier_CloseEndsReceivers(t *testing.T) {
	f := NewFrontier()
	f.Push(Task{URL: "a", Depth: 3})
	if got := <-f.out; got != (Task{URL: "a", Depth: 3}) {
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
