package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mukul-work/golang-search-engine/internal/models"
	"github.com/mukul-work/golang-search-engine/internal/search"
)

type fakeSearcher struct {
	res         search.Results
	err         error
	calls       int
	gotQ        string
	gotPage     int
	gotPerPage  int
	hadDeadline bool
}

func (f *fakeSearcher) Query(ctx context.Context, q string, page, perPage int) (search.Results, error) {
	f.calls++
	f.gotQ, f.gotPage, f.gotPerPage = q, page, perPage
	_, f.hadDeadline = ctx.Deadline()
	return f.res, f.err
}

type panicSearcher struct{}

func (panicSearcher) Query(context.Context, string, int, int) (search.Results, error) {
	panic("boom")
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func do(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestSearch_Success(t *testing.T) {
	fs := &fakeSearcher{res: search.Results{
		Query: "golang", Total: 23, Page: 2, PerPage: 10,
		Items: []models.Result{
			{Title: "Go", URL: "https://go.dev/", Snippet: "The Go language", Score: 1.5},
			{Title: "Tour", URL: "https://go.dev/tour/", Snippet: "A tour", Score: 1.2},
		},
	}}
	rec := do(New(fs, discardLogger()).Handler(), http.MethodGet, "/api/search?q=golang&page=2&per_page=10")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not valid JSON: %v\n%s", err, rec.Body.String())
	}
	if got.Total != 23 || got.TotalPages != 3 || got.Page != 2 || got.PerPage != 10 {
		t.Errorf("paging fields wrong: %+v", got)
	}
	if !got.HasPrev || !got.HasNext {
		t.Errorf("HasPrev=%v HasNext=%v, want both true on the middle page", got.HasPrev, got.HasNext)
	}
	if len(got.Results) != 2 || got.Results[0].URL != "https://go.dev/" {
		t.Errorf("results wrong: %+v", got.Results)
	}

	if fs.gotQ != "golang" || fs.gotPage != 2 || fs.gotPerPage != 10 {
		t.Errorf("searcher got q=%q page=%d perPage=%d", fs.gotQ, fs.gotPage, fs.gotPerPage)
	}
	if !fs.hadDeadline {
		t.Error("the context passed to the searcher has no deadline")
	}
}

func TestSearch_DefaultsAndTrimming(t *testing.T) {
	fs := &fakeSearcher{res: search.Results{Page: 1, PerPage: search.DefaultPerPage}}
	rec := do(New(fs, discardLogger()).Handler(), http.MethodGet, "/api/search?q=%20go%20")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if fs.gotQ != "go" {
		t.Errorf("q = %q, want %q (trimmed)", fs.gotQ, "go")
	}
	if fs.gotPage != 1 || fs.gotPerPage != search.DefaultPerPage {
		t.Errorf("page=%d perPage=%d, want 1 and %d", fs.gotPage, fs.gotPerPage, search.DefaultPerPage)
	}
}

func TestSearch_EmptyResultsEncodeAsArray(t *testing.T) {
	fs := &fakeSearcher{res: search.Results{Query: "zzz", Page: 1, PerPage: 10}} // Items is nil
	rec := do(New(fs, discardLogger()).Handler(), http.MethodGet, "/api/search?q=zzz")

	if !strings.Contains(rec.Body.String(), `"results":[]`) {
		t.Errorf("want an empty array, got: %s", rec.Body.String())
	}
}

func TestSearch_BadRequests(t *testing.T) {
	tests := []struct{ name, target string }{
		{"missing q", "/api/search"},
		{"blank q", "/api/search?q=%20%20"},
		{"q too long", "/api/search?q=" + strings.Repeat("a", maxQueryLen+1)},
		{"page not a number", "/api/search?q=go&page=abc"},
		{"page zero", "/api/search?q=go&page=0"},
		{"page negative", "/api/search?q=go&page=-1"},
		{"per_page not a number", "/api/search?q=go&per_page=x"},
		{"per_page zero", "/api/search?q=go&per_page=0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeSearcher{}
			rec := do(New(fs, discardLogger()).Handler(), http.MethodGet, tc.target)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
				t.Errorf("want a JSON body with an error message, got: %s", rec.Body.String())
			}
			if fs.calls != 0 {
				t.Errorf("searcher called %d times, want 0: bad input must not reach it", fs.calls)
			}
		})
	}
}

func TestSearch_StoreErrorIs500AndHidesDetails(t *testing.T) {
	fs := &fakeSearcher{err: errors.New("pq: password=hunter2 rejected")}
	rec := do(New(fs, discardLogger()).Handler(), http.MethodGet, "/api/search?q=go")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Errorf("internal error details leaked to the client: %s", rec.Body.String())
	}
}

func TestSearch_TimeoutIs503(t *testing.T) {
	fs := &fakeSearcher{err: fmt.Errorf("query: %w", context.DeadlineExceeded)}
	rec := do(New(fs, discardLogger()).Handler(), http.MethodGet, "/api/search?q=go")

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	rec := do(New(&fakeSearcher{}, discardLogger()).Handler(), http.MethodGet, "/healthz")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["status"] != "ok" {
		t.Errorf("body = %s, want status ok", rec.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	fs := &fakeSearcher{}
	rec := do(New(fs, discardLogger()).Handler(), http.MethodPost, "/api/search?q=go")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, "GET") {
		t.Errorf("Allow = %q, want it to list GET", allow)
	}
	if fs.calls != 0 {
		t.Error("searcher was called for a POST")
	}
}

func TestUnknownPathIs404(t *testing.T) {
	rec := do(New(&fakeSearcher{}, discardLogger()).Handler(), http.MethodGet, "/nope")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestPanicBecomes500(t *testing.T) {
	rec := do(New(panicSearcher{}, discardLogger()).Handler(), http.MethodGet, "/api/search?q=go")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("panic value leaked to the client: %s", rec.Body.String())
	}
}

func TestRequestsAreLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil)) // default level: info
	h := New(&fakeSearcher{res: search.Results{Page: 1, PerPage: 10}}, logger).Handler()

	do(h, http.MethodGet, "/api/search?q=secret-term")
	do(h, http.MethodGet, "/healthz")

	out := buf.String()
	if !strings.Contains(out, "path=/api/search") || !strings.Contains(out, "status=200") {
		t.Errorf("search request not logged as expected:\n%s", out)
	}
	if strings.Contains(out, "secret-term") {
		t.Errorf("the search query was logged:\n%s", out)
	}
	if strings.Contains(out, "/healthz") {
		t.Errorf("health checks should only be logged at debug level:\n%s", out)
	}
}

// Everything above calls the handler directly. This one goes through a real
// TCP connection to a real HTTP server on localhost.
func TestOverRealHTTP(t *testing.T) {
	fs := &fakeSearcher{res: search.Results{
		Query: "go", Total: 1, Page: 1, PerPage: 10,
		Items: []models.Result{{URL: "https://go.dev/"}},
	}}
	srv := httptest.NewServer(New(fs, discardLogger()).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/search?q=go")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if got.Total != 1 || len(got.Results) != 1 {
		t.Errorf("unexpected response: %+v", got)
	}
}
