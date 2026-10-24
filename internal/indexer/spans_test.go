package indexer

import (
	"reflect"
	"testing"
)

func TestWordSpans(t *testing.T) {
	text := "Go, go-lang & 42 gophers!"
	var got []string
	for _, sp := range WordSpans(text) {
		got = append(got, text[sp.Start:sp.End])
	}
	want := []string{"Go", "go", "lang", "gophers"} // digits and punctuation are not words
	if !reflect.DeepEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}

func TestWordSpans_NoWords(t *testing.T) {
	if got := WordSpans("123 !!!"); len(got) != 0 {
		t.Errorf("got %v, want no spans", got)
	}
}
