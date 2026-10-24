package search

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/mukul-work/golang-search-engine/internal/models"
)

// fakeStore pretends `all` is every match, already ranked, and slices it the
// way the database would.
type fakeStore struct {
	all       []models.Result
	err       error
	calls     int
	gotTerms  []string
	gotLimit  int
	gotOffset int
}

func (f *fakeStore) SearchPages(_ context.Context, terms []string, limit, offset int) ([]models.Result, int, error) {
	f.calls++
	f.gotTerms, f.gotLimit, f.gotOffset = terms, limit, offset
	if f.err != nil {
		return nil, 0, f.err
	}
	total := len(f.all)
	if offset >= total {
		return nil, 0, nil // like Postgres: no rows means no total either
	}
	end := min(offset+limit, total)
	return f.all[offset:end], total, nil
}

func makeResults(n int) []models.Result {
	out := make([]models.Result, n)
	for i := range out {
		out[i] = models.Result{URL: fmt.Sprintf("https://example.com/%d", i)}
	}
	return out
}

func TestQuery_OnlyStopwordsSkipsStore(t *testing.T) {
	fs := &fakeStore{all: makeResults(5)}
	got, err := NewService(fs).Query(context.Background(), "the and of", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 0 || got.Total != 0 {
		t.Errorf("got %+v, want an empty result", got)
	}
	if fs.calls != 0 {
		t.Errorf("store was called %d times, want 0", fs.calls)
	}
}

func TestQuery_PassesTermsLimitAndOffset(t *testing.T) {
	fs := &fakeStore{all: makeResults(100)}
	if _, err := NewService(fs).Query(context.Background(), "Web crawler WEB", 3, 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sort.Strings(fs.gotTerms) // map iteration order is random
	if want := []string{"crawler", "web"}; !reflect.DeepEqual(fs.gotTerms, want) {
		t.Errorf("terms = %v, want %v", fs.gotTerms, want)
	}
	if fs.gotLimit != 5 {
		t.Errorf("limit = %d, want 5", fs.gotLimit)
	}
	if fs.gotOffset != 10 { // (page 3 - 1) * 5
		t.Errorf("offset = %d, want 10", fs.gotOffset)
	}
}

func TestQuery_PropagatesStoreError(t *testing.T) {
	wantErr := errors.New("db down")
	_, err := NewService(&fakeStore{err: wantErr}).Query(context.Background(), "golang", 1, 10)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestQuery_Pagination(t *testing.T) {
	svc := NewService(&fakeStore{all: makeResults(23)})
	tests := []struct {
		name      string
		page      int
		wantItems int
		wantFirst string
		wantPrev  bool
		wantNext  bool
	}{
		{"first page", 1, 10, "https://example.com/0", false, true},
		{"middle page", 2, 10, "https://example.com/10", true, true},
		{"last page is partial", 3, 3, "https://example.com/20", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := svc.Query(context.Background(), "golang", tc.page, 10)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(res.Items) != tc.wantItems {
				t.Fatalf("got %d items, want %d", len(res.Items), tc.wantItems)
			}
			if res.Items[0].URL != tc.wantFirst {
				t.Errorf("first item = %s, want %s", res.Items[0].URL, tc.wantFirst)
			}
			if res.Total != 23 || res.TotalPages() != 3 {
				t.Errorf("total=%d pages=%d, want 23 and 3", res.Total, res.TotalPages())
			}
			if res.HasPrev() != tc.wantPrev || res.HasNext() != tc.wantNext {
				t.Errorf("HasPrev=%v HasNext=%v, want %v and %v", res.HasPrev(), res.HasNext(), tc.wantPrev, tc.wantNext)
			}
		})
	}
}

func TestQuery_PastTheLastPageStillReportsTotal(t *testing.T) {
	fs := &fakeStore{all: makeResults(23)}
	res, err := NewService(fs).Query(context.Background(), "golang", 9, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 0 {
		t.Errorf("got %d items, want none", len(res.Items))
	}
	if res.Total != 23 || res.TotalPages() != 3 {
		t.Errorf("total=%d pages=%d, want 23 and 3", res.Total, res.TotalPages())
	}
	if res.HasNext() {
		t.Error("HasNext = true past the last page")
	}
	if fs.calls != 2 {
		t.Errorf("store called %d times, want 2 (the page, then a lookup for the total)", fs.calls)
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name                string
		page, perPage       int
		wantPage, wantPerPg int
	}{
		{"zeros get defaults", 0, 0, 1, DefaultPerPage},
		{"negatives get defaults", -5, -1, 1, DefaultPerPage},
		{"valid values pass through", 2, 25, 2, 25},
		{"per page is capped", 1, 1000, 1, MaxPerPage},
		{"page is capped", 999999999, 10, maxPage, 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, pp := normalize(tc.page, tc.perPage)
			if p != tc.wantPage || pp != tc.wantPerPg {
				t.Errorf("normalize(%d, %d) = (%d, %d), want (%d, %d)",
					tc.page, tc.perPage, p, pp, tc.wantPage, tc.wantPerPg)
			}
		})
	}
}

func TestResults_TotalPages(t *testing.T) {
	tests := []struct{ total, perPage, want int }{
		{0, 10, 0},
		{1, 10, 1},
		{10, 10, 1},
		{11, 10, 2},
		{23, 10, 3},
		{23, 50, 1},
	}
	for _, tc := range tests {
		got := Results{Total: tc.total, PerPage: tc.perPage}.TotalPages()
		if got != tc.want {
			t.Errorf("TotalPages(total=%d, perPage=%d) = %d, want %d", tc.total, tc.perPage, got, tc.want)
		}
	}
}
