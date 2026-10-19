package search

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/mukul-work/golang-search-engine/internal/models"
)

type fakeStore struct {
	called   bool
	gotTerms []string
	gotLimit int
	results  []models.Result
	err      error
}

func (f *fakeStore) SearchPages(_ context.Context, terms []string, limit int) ([]models.Result, error) {
	f.called = true
	f.gotTerms = terms
	f.gotLimit = limit
	return f.results, f.err
}

func TestQuery_OnlyStopwordsSkipsStore(t *testing.T) {
	fs := &fakeStore{}
	got, err := NewService(fs).Query(context.Background(), "the and of", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if fs.called {
		t.Error("store was called, want no call")
	}
}

func TestQuery_PassesDedupedTermsAndLimit(t *testing.T) {
	fs := &fakeStore{results: []models.Result{{URL: "https://example.com"}}}
	got, err := NewService(fs).Query(context.Background(), "Web crawler WEB", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sort.Strings(fs.gotTerms) // map iteration order is random
	if want := []string{"crawler", "web"}; !reflect.DeepEqual(fs.gotTerms, want) {
		t.Errorf("terms = %v, want %v", fs.gotTerms, want)
	}
	if fs.gotLimit != 5 {
		t.Errorf("limit = %d, want 5", fs.gotLimit)
	}
	if len(got) != 1 || got[0].URL != "https://example.com" {
		t.Errorf("results = %v, want the store's results", got)
	}
}

func TestQuery_PropagatesStoreError(t *testing.T) {
	wantErr := errors.New("db down")
	_, err := NewService(&fakeStore{err: wantErr}).Query(context.Background(), "golang", 10)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}
