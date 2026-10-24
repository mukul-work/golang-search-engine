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
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/search", s.handleSearch)

	// logRequests is the outer layer so it sees the status code that
	// recoverPanics writes after a panic.
	s.handler = s.logRequests(s.recoverPanics(mux))
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

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	switch {
	case q == "":
		writeError(w, http.StatusBadRequest, "missing query parameter: q")
		return
	case len(q) > maxQueryLen:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("q must be at most %d bytes", maxQueryLen))
		return
	}

	page, err := intParam(r, "page", 1)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	perPage, err := intParam(r, "per_page", search.DefaultPerPage)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Values above the maximums are not errors: the service clamps them and
	// the response reports the page and per_page actually used.

	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()

	res, err := s.searcher.Query(ctx, q, page, perPage)
	if err != nil {
		switch {
		case r.Context().Err() != nil:
			s.logger.Debug("client disconnected during search") // nobody is listening for a reply
		case errors.Is(err, context.DeadlineExceeded):
			s.logger.Warn("search timed out", "timeout", queryTimeout)
			writeError(w, http.StatusServiceUnavailable, "search timed out, try again")
		default:
			s.logger.Error("search failed", "err", err) // details go to the log, not to the client
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, newSearchResponse(res))
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
