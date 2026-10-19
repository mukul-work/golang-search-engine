package search

import (
	"context"

	"github.com/mukul-work/golang-search-engine/internal/indexer"
	"github.com/mukul-work/golang-search-engine/internal/models"
)

type PageSearcher interface {
	SearchPages(ctx context.Context, terms []string, limit int) ([]models.Result, error)
}

type Service struct {
	store PageSearcher //anything that implements PageSearcher can be used to create store --> prevents dependence on postgres
}

func NewService(store PageSearcher) *Service {
	return &Service{store: store}
}

func (s *Service) Query(ctx context.Context, q string, limit int) ([]models.Result, error) {
	counts := indexer.Tokenize(q)
	if len(counts) == 0 {
		return nil, nil
	}
	terms := make([]string, 0, len(counts))
	for term := range counts {
		terms = append(terms, term)
	}
	return s.store.SearchPages(ctx, terms, limit)
}
