package crawler

import (
	"testing"
	"time"
)

func TestFrontier_CloseEndsReceivers(t *testing.T) {
	f := NewFrontier()
	f.Push("a")
	if got := <-f.out; got != "a" {
		t.Fatalf("got %q, want %q", got, "a")
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
