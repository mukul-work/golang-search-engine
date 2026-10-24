package search

import (
	"context"

	"github.com/mukul-work/golang-search-engine/internal/indexer"
	"github.com/mukul-work/golang-search-engine/internal/models"
)

const (
	DefaultPerPage = 10
	MaxPerPage     = 50
	maxPage        = 1000 // deep offsets are slow, and this also prevents overflow
)

// PageSearcher is what Service needs from storage.
type PageSearcher interface {
	SearchPages(ctx context.Context, terms []string, limit, offset int) ([]models.Result, int, error)
}

// Results is one page of search results plus what a UI needs to render a pager.
type Results struct {
	Query   string
	Items   []models.Result
	Total   int // all matches, not just this page
	Page    int // 1-based, after clamping
	PerPage int // after clamping
}

func (r Results) TotalPages() int {
	if r.Total == 0 {
		return 0
	}
	return (r.Total + r.PerPage - 1) / r.PerPage // ceiling division
}

func (r Results) HasPrev() bool { return r.Page > 1 }
func (r Results) HasNext() bool { return r.Page < r.TotalPages() }

type Service struct {
	store PageSearcher
}

func NewService(store PageSearcher) *Service {
	return &Service{store: store}
}

// normalize clamps untrusted paging input into safe bounds.
func normalize(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if page > maxPage {
		page = maxPage
	}
	if perPage < 1 {
		perPage = DefaultPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}
	return page, perPage
}

func (s *Service) Query(ctx context.Context, q string, page, perPage int) (Results, error) {
	page, perPage = normalize(page, perPage)
	res := Results{Query: q, Page: page, PerPage: perPage}

	counts := indexer.Tokenize(q)
	if len(counts) == 0 {
		return res, nil // only stopwords or punctuation: nothing to search for
	}
	terms := make([]string, 0, len(counts))
	for term := range counts {
		terms = append(terms, term)
	}

	items, total, err := s.store.SearchPages(ctx, terms, perPage, (page-1)*perPage)
	if err != nil {
		return Results{}, err
	}
	if len(items) == 0 && page > 1 {
		// Past the last page: no rows came back, so total reads 0. Ask for the
		// first row only to learn the real total, so the caller can say
		// "that page doesn't exist; there are N results".
		_, total, err = s.store.SearchPages(ctx, terms, 1, 0)
		if err != nil {
			return Results{}, err
		}
	}
	res.Items = items
	res.Total = total
	return res, nil
}
