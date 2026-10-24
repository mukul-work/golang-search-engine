package server

import (
	"bytes"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mukul-work/golang-search-engine/internal/search"
)

// webFS holds the page template and static files, baked into the binary.
//
//go:embed web
var webFS embed.FS

// pageTmpl is parsed once at startup. The template is embedded, so a parse
// failure is a programming error, and a panic at startup finds it at once.
var pageTmpl = template.Must(template.ParseFS(webFS, "web/index.html"))

// staticFS is the web/static folder, so URLs can say /static/style.css.
var staticFS = func() fs.FS {
	sub, err := fs.Sub(webFS, "web/static")
	if err != nil {
		panic(err) // only possible if the path is malformed
	}
	return sub
}()

// pageData is everything the template needs. All of it is plain data; the
// template decides how to escape each field.
type pageData struct {
	Query      string
	Searched   bool // a query was submitted
	Error      string
	Results    []resultView
	Total      int
	Page       int
	TotalPages int
	From, To   int // "Results From-To of Total"
	HasPrev    bool
	HasNext    bool
	PrevURL    string
	NextURL    string
	PastEnd    bool // the requested page is beyond the last one
	LastURL    string
}

type resultView struct {
	Title   []search.Segment
	URL     string
	Snippet []search.Segment
}

// pageURL builds the address of a results page. Page 1 has no page parameter,
// and url.Values escapes everything safely, even queries with & or spaces.
func pageURL(q string, page int) string {
	v := url.Values{"q": {q}}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	return "/?" + v.Encode()
}

// buildPage turns search results into template data.
func buildPage(q string, res search.Results) pageData {
	d := pageData{
		Query:      q,
		Searched:   true,
		Total:      res.Total,
		Page:       res.Page,
		TotalPages: res.TotalPages(),
		HasPrev:    res.HasPrev(),
		HasNext:    res.HasNext(),
	}
	if d.HasPrev {
		d.PrevURL = pageURL(q, res.Page-1)
	}
	if d.HasNext {
		d.NextURL = pageURL(q, res.Page+1)
	}

	if len(res.Items) == 0 && res.Total > 0 {
		d.PastEnd = true
		d.LastURL = pageURL(q, res.TotalPages())
		return d
	}

	d.From = (res.Page-1)*res.PerPage + 1
	d.To = d.From + len(res.Items) - 1
	for _, it := range res.Items {
		title := it.Title
		if title == "" {
			title = it.URL // a page without a <title> would otherwise get an empty link
		}
		d.Results = append(d.Results, resultView{
			Title:   search.Highlight(title, q),
			URL:     it.URL,
			Snippet: search.Highlight(it.Snippet, q),
		})
	}
	return d
}

// renderPage executes the template into memory first, so a template failure
// can still become a clean 500 instead of a half-written page.
func (s *Server) renderPage(w http.ResponseWriter, status int, data pageData) {
	var buf bytes.Buffer
	if err := pageTmpl.Execute(&buf, data); err != nil {
		s.logger.Error("render page", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	p, err := parseParams(r)
	switch {
	case errors.Is(err, errMissingQuery):
		s.renderPage(w, http.StatusOK, pageData{}) // the landing page: just the form
		return
	case err != nil:
		s.renderPage(w, http.StatusBadRequest, pageData{Query: p.q, Error: "Invalid request: " + err.Error()})
		return
	}

	out := s.runSearch(r, p)
	switch {
	case out.gone:
		return
	case out.status != http.StatusOK:
		s.renderPage(w, out.status, pageData{Query: p.q, Error: out.msg})
	default:
		s.renderPage(w, http.StatusOK, buildPage(p.q, out.res))
	}
}
