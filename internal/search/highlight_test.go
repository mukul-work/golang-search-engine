package search

import (
	"reflect"
	"strings"
	"testing"
)

func TestHighlight(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		query string
		want  []Segment
	}{
		{
			"marks matching words, ignoring case",
			"Go is fun. GOPHERS love go.", "go gophers",
			[]Segment{{"Go", true}, {" is fun. ", false}, {"GOPHERS", true}, {" love ", false}, {"go", true}, {".", false}},
		},
		{"no match gives one plain segment", "hello world", "golang", []Segment{{"hello world", false}}},
		{"whole words only", "gopher gophers", "gopher", []Segment{{"gopher", true}, {" gophers", false}}},
		{"stopword-only query highlights nothing", "the cat and the hat", "the and", []Segment{{"the cat and the hat", false}}},
		{"markup stays plain text", "<script>go</script>", "go", []Segment{{"<script>", false}, {"go", true}, {"</script>", false}}},
		{"empty text", "", "go", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Highlight(tc.text, tc.query)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Highlight(%q, %q) = %v, want %v", tc.text, tc.query, got, tc.want)
			}

			// Property: the segments always rebuild the original text exactly.
			var rebuilt strings.Builder
			for _, s := range got {
				rebuilt.WriteString(s.Text)
			}
			if rebuilt.String() != tc.text {
				t.Errorf("segments rebuild to %q, want %q", rebuilt.String(), tc.text)
			}
		})
	}
}
