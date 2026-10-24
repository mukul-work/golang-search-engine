package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/mukul-work/golang-search-engine/internal/search"
)

const (
	maxQueryLen  = 256 // bytes; bounds the work a single request can ask for
	queryTimeout = 5 * time.Second
)

// Searcher is what the server needs from the search layer.
type Searcher interface {
	Query(ctx context.Context, q string, page, perPage int) (search.Results, error)
}

type Server struct {
	searcher Searcher
	logger   *slog.Logger
	handler  http.Handler
}

func New(searcher Searcher, logger *slog.Logger) *Server {
	s := &Server{searcher: searcher, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handlePage)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/search", s.handleSearch)

	// secureHeaders is outermost so every response gets the headers, including
	// 404s and 405s the mux writes itself.
	s.handler = secureHeaders(s.logRequests(s.recoverPanics(mux)))
	return s
}

// Handler returns the fully wired handler, ready for http.Server.
func (s *Server) Handler() http.Handler { return s.handler }

// ---- JSON shapes: the API contract, deliberately separate from models.Result ----

type resultJSON struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type searchResponse struct {
	Query      string       `json:"query"`
	Page       int          `json:"page"`
	PerPage    int          `json:"per_page"`
	Total      int          `json:"total"`
	TotalPages int          `json:"total_pages"`
	HasPrev    bool         `json:"has_prev"`
	HasNext    bool         `json:"has_next"`
	Results    []resultJSON `json:"results"`
}

func newSearchResponse(res search.Results) searchResponse {
	items := make([]resultJSON, 0, len(res.Items)) // non-nil, so an empty page encodes as [] and not null
	for _, it := range res.Items {
		items = append(items, resultJSON{Title: it.Title, URL: it.URL, Snippet: it.Snippet, Score: it.Score})
	}
	return searchResponse{
		Query:      res.Query,
		Page:       res.Page,
		PerPage:    res.PerPage,
		Total:      res.Total,
		TotalPages: res.TotalPages(),
		HasPrev:    res.HasPrev(),
		HasNext:    res.HasNext(),
		Results:    items,
	}
}

// ---- handlers ----

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

var errMissingQuery = errors.New("missing query parameter: q")

type searchParams struct {
	q       string
	page    int
	perPage int
}

// parseParams reads and validates the query parameters shared by the JSON API
// and the HTML page. On error it still returns the trimmed q, so the page can
// show it back to the user. Messages are safe to show.
func parseParams(r *http.Request) (searchParams, error) {
	p := searchParams{q: strings.TrimSpace(r.URL.Query().Get("q"))}
	switch {
	case p.q == "":
		return p, errMissingQuery
	case len(p.q) > maxQueryLen:
		return p, fmt.Errorf("q must be at most %d bytes", maxQueryLen)
	}

	var err error
	if p.page, err = intParam(r, "page", 1); err != nil {
		return p, err
	}
	if p.perPage, err = intParam(r, "per_page", search.DefaultPerPage); err != nil {
		return p, err
	}
	// Values above the maximums are not errors: the service clamps them and
	// the response reports what was actually used.
	return p, nil
}

// outcome is a finished search with failures already translated into an HTTP
// status and a message that is safe to show people.
type outcome struct {
	res    search.Results
	status int    // http.StatusOK on success
	msg    string // user-facing message when status is not 200
	gone   bool   // the client disconnected: write nothing
}

// runSearch queries with a timeout and classifies any failure. Both the JSON
// API and the HTML page use it, so they can never disagree.
func (s *Server) runSearch(r *http.Request, p searchParams) outcome {
	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()

	res, err := s.searcher.Query(ctx, p.q, p.page, p.perPage)
	switch {
	case err == nil:
		return outcome{res: res, status: http.StatusOK}
	case r.Context().Err() != nil:
		s.logger.Debug("client disconnected during search") // nobody is listening for a reply
		return outcome{gone: true}
	case errors.Is(err, context.DeadlineExceeded):
		s.logger.Warn("search timed out", "timeout", queryTimeout)
		return outcome{status: http.StatusServiceUnavailable, msg: "Search timed out. Please try again."}
	default:
		s.logger.Error("search failed", "err", err) // details go to the log, never to the client
		return outcome{status: http.StatusInternalServerError, msg: "Something went wrong on our side."}
	}
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	p, err := parseParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out := s.runSearch(r, p)
	switch {
	case out.gone:
		return
	case out.status != http.StatusOK:
		writeError(w, out.status, out.msg)
	default:
		writeJSON(w, http.StatusOK, newSearchResponse(out.res))
	}
}

// intParam reads an optional positive-integer query parameter.
func intParam(r *http.Request, name string, def int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return n, nil
}

// ---- response helpers ----

const jsonContentType = "application/json; charset=utf-8"

func writeJSON(w http.ResponseWriter, status int, v any) {
	// Marshal first: if encoding fails we can still send a clean 500, which is
	// impossible once part of a body has been written.
	body, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", jsonContentType)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	w.Write(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---- middleware ----

// statusRecorder remembers the status code a handler wrote, because
// http.ResponseWriter has no way to read it back.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the real writer underneath.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK} // 200 unless WriteHeader says otherwise
		next.ServeHTTP(rec, r)

		level := slog.LevelInfo
		if r.URL.Path == "/healthz" {
			level = slog.LevelDebug // probes hit this constantly
		}
		// The query string is deliberately not logged: it holds what people search for.
		s.logger.Log(r.Context(), level, "request",
			"method", r.Method, "path", r.URL.Path, "status", rec.status,
			"took_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec) // net/http's own signal to abort a response: pass it on
				}
				s.logger.Error("panic in handler", "panic", rec, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// secureHeaders adds browser-facing protections to every response.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Pages may load only our own stylesheet and submit forms to ourselves.
		h.Set("Content-Security-Policy",
			"default-src 'none'; style-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer") // our URLs contain what people searched for
		next.ServeHTTP(w, r)
	})
}
