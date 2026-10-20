package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRobots_CancelledFetchIsNotCached(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			<-r.Context().Done() // the first request hangs until the client gives up
			return
		}
		w.Write([]byte("User-agent: *\nDisallow: /private\n"))
	}))
	defer srv.Close()

	rc := NewRobotCache(&http.Client{Timeout: 5 * time.Second})

	ctx1, cancel1 := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel1)
	if _, err := rc.Allowed(ctx1, srv.URL+"/x"); err == nil {
		t.Fatal("expected an error from the cancelled fetch")
	}

	// A fresh context must trigger a new fetch, not reuse the cancelled failure.
	allowed, err := rc.Allowed(context.Background(), srv.URL+"/private")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("/private should be disallowed by the fetched robots.txt")
	}
}
