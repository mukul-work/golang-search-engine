package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mukul-work/golang-search-engine/internal/models"
	"github.com/mukul-work/golang-search-engine/internal/search"
)

func pageHandler(fs *fakeSearcher) http.Handler {
	return New(fs, discardLogger()).Handler()
}

func TestPage_LandingPageHasFormAndSkipsSearch(t *testing.T) {
	fs := &fakeSearcher{}
	rec := do(pageHandler(fs), http.MethodGet, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<form") || !strings.Contains(body, `name="q"`) {
		t.Errorf("landing page has no search form:\n%s", body)
	}
	if fs.calls != 0 {
		t.Errorf("searcher called %d times for the landing page, want 0", fs.calls)
	}
}

func TestPage_ShowsHighlightedResultsAndPager(t *testing.T) {
	fs := &fakeSearcher{res: search.Results{
		Query: "golang", Total: 23, Page: 2, PerPage: 10,
		Items: []models.Result{{
			Title: "The Golang Blog", URL: "https://go.dev/blog/", Snippet: "News about Golang releases",
		}},
	}}
	body := do(pageHandler(fs), http.MethodGet, "/?q=golang&page=2").Body.String()

	for _, want := range []string{
		`The <mark>Golang</mark> Blog`,
		`News about <mark>Golang</mark> releases`,
		`Results 11-11 of 23`,
		`Page 2 of 3`,
		`value="golang"`,
		`rel="noopener noreferrer"`,
		`href="/?q=golang"`,            // previous page is page 1: no page parameter
		`href="/?page=3&amp;q=golang"`, // next page; & is escaped inside the attribute
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q:\n%s", want, body)
		}
	}
	if fs.gotQ != "golang" || fs.gotPage != 2 {
		t.Errorf("searcher got q=%q page=%d, want golang and 2", fs.gotQ, fs.gotPage)
	}
}

func TestPage_EscapesUntrustedContent(t *testing.T) {
	fs := &fakeSearcher{res: search.Results{
		Query: "x", Total: 1, Page: 1, PerPage: 10,
		Items: []models.Result{{
			Title:   `<script>alert(1)</script>`,
			URL:     `javascript:alert(1)`,
			Snippet: `<img src=x onerror=alert(1)>`,
		}},
	}}
	body := do(pageHandler(fs), http.MethodGet, "/?q=x").Body.String()

	for _, bad := range []string{"<script>alert(1)", "<img src=x", `href="javascript:`} {
		if strings.Contains(body, bad) {
			t.Errorf("page contains unescaped %q:\n%s", bad, body)
		}
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("the title should appear as escaped text:\n%s", body)
	}
}

func TestPage_EchoedQueryIsEscaped(t *testing.T) {
	body := do(pageHandler(&fakeSearcher{}), http.MethodGet,
		"/?q=%22%3E%3Cscript%3Ealert(1)%3C/script%3E").Body.String() // q = "><script>alert(1)</script>

	if strings.Contains(body, "<script>alert") {
		t.Errorf("the query was echoed without escaping:\n%s", body)
	}
}

func TestPage_NoResults(t *testing.T) {
	fs := &fakeSearcher{res: search.Results{Query: "zzz", Page: 1, PerPage: 10}}
	body := do(pageHandler(fs), http.MethodGet, "/?q=zzz").Body.String()

	if !strings.Contains(body, "No results for") {
		t.Errorf("want a no-results message:\n%s", body)
	}
}

func TestPage_PastTheLastPage(t *testing.T) {
	fs := &fakeSearcher{res: search.Results{Query: "golang", Total: 23, Page: 9, PerPage: 10}}
	body := do(pageHandler(fs), http.MethodGet, "/?q=golang&page=9").Body.String()

	if !strings.Contains(body, "fit on 3 pages") {
		t.Errorf("want to be told the real page count:\n%s", body)
	}
	if !strings.Contains(body, `href="/?page=3&amp;q=golang"`) {
		t.Errorf("want a link to the last page:\n%s", body)
	}
}

func TestPage_BadInputIs400(t *testing.T) {
	for name, target := range map[string]string{
		"page not a number": "/?q=go&page=abc",
		"page zero":         "/?q=go&page=0",
		"q too long":        "/?q=" + strings.Repeat("a", maxQueryLen+1),
	} {
		t.Run(name, func(t *testing.T) {
			fs := &fakeSearcher{}
			rec := do(pageHandler(fs), http.MethodGet, target)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("Content-Type = %q, want an HTML error page", ct)
			}
			if !strings.Contains(rec.Body.String(), "Invalid request") || !strings.Contains(rec.Body.String(), "<form") {
				t.Errorf("want an error message and the search form:\n%s", rec.Body.String())
			}
			if fs.calls != 0 {
				t.Errorf("searcher called %d times, want 0", fs.calls)
			}
		})
	}
}

func TestPage_StoreErrorIs500AndHidesDetails(t *testing.T) {
	fs := &fakeSearcher{err: errors.New("pq: password=hunter2 rejected")}
	rec := do(pageHandler(fs), http.MethodGet, "/?q=go")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Errorf("internal error details leaked into the page:\n%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Errorf("want a friendly message:\n%s", rec.Body.String())
	}
}

func TestPage_TimeoutIs503(t *testing.T) {
	fs := &fakeSearcher{err: fmt.Errorf("query: %w", context.DeadlineExceeded)}
	rec := do(pageHandler(fs), http.MethodGet, "/?q=go")

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	for _, path := range []string{"/", "/healthz", "/api/search?q=go", "/nope"} {
		t.Run(path, func(t *testing.T) {
			h := do(pageHandler(&fakeSearcher{}), http.MethodGet, path).Header()

			if csp := h.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
				t.Errorf("Content-Security-Policy = %q", csp)
			}
			if h.Get("X-Content-Type-Options") != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", h.Get("X-Content-Type-Options"))
			}
			if h.Get("Referrer-Policy") != "no-referrer" {
				t.Errorf("Referrer-Policy = %q, want no-referrer", h.Get("Referrer-Policy"))
			}
		})
	}
}

func TestStaticCSSIsServed(t *testing.T) {
	rec := do(pageHandler(&fakeSearcher{}), http.MethodGet, "/static/style.css")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q, want text/css", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("stylesheet is empty")
	}
}

func TestPageURL(t *testing.T) {
	tests := []struct {
		q    string
		page int
		want string
	}{
		{"go", 1, "/?q=go"},
		{"go", 3, "/?page=3&q=go"},
		{"a b&c", 1, "/?q=a+b%26c"},
	}
	for _, tc := range tests {
		if got := pageURL(tc.q, tc.page); got != tc.want {
			t.Errorf("pageURL(%q, %d) = %q, want %q", tc.q, tc.page, got, tc.want)
		}
	}
}
