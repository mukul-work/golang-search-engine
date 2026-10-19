package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestFetcher(timeout time.Duration) *HTTPFetcher {
	return NewHTTPFetcher(&http.Client{Timeout: timeout})
}

func TestFetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "Text/HTML; charset=UTF-8") // odd casing on purpose
		w.Write([]byte("<html>hi</html>"))
	}))
	defer srv.Close()

	page, err := newTestFetcher(time.Second).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Body != "<html>hi</html>" || page.StatusCode != 200 {
		t.Errorf("got body=%q status=%d", page.Body, page.StatusCode)
	}
}

func TestFetch_SendsUserAgent(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html")
	}))
	defer srv.Close()

	if _, err := newTestFetcher(time.Second).Fetch(context.Background(), srv.URL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ua := <-got; ua != userAgent {
		t.Errorf("User-Agent = %q, want %q", ua, userAgent)
	}
}

func TestFetch_Errors(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
	}{
		{"404 is an error", 404, "text/html"},
		{"500 is an error", 500, "text/html"},
		{"non-HTML is rejected", 200, "application/pdf"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			if _, err := newTestFetcher(time.Second).Fetch(context.Background(), srv.URL); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestFetch_TruncatesHugeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(strings.Repeat("a", maxPageSize+1000)))
	}))
	defer srv.Close()

	page, err := newTestFetcher(5*time.Second).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Body) != maxPageSize {
		t.Errorf("len(body) = %d, want %d", len(page.Body), maxPageSize)
	}
}

func TestFetch_TimesOutOnSilentServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select { // hang until the client gives up
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()

	start := time.Now()
	_, err := newTestFetcher(100*time.Millisecond).Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %v, the timeout did not fire", elapsed)
	}
}
