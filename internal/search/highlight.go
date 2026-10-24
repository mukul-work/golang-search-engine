package search

import (
	"strings"

	"github.com/mukul-work/golang-search-engine/internal/indexer"
)

// Segment is a run of text that either matches a search term or doesn't.
// Joining every segment's Text gives back the original text exactly.
type Segment struct {
	Text  string
	Match bool
}

// Highlight splits text into segments and marks the words that match a term
// of query. Matching follows the indexer: whole words, case-insensitive,
// stopwords ignored. It never builds markup; turning segments into HTML is
// the template's job, which keeps escaping in one place.
func Highlight(text, query string) []Segment {
	if text == "" {
		return nil
	}
	terms := indexer.Tokenize(query)
	if len(terms) == 0 {
		return []Segment{{Text: text}} // nothing to highlight
	}

	var segs []Segment
	last := 0 // end of the text already placed into segments
	for _, sp := range indexer.WordSpans(text) {
		if _, ok := terms[strings.ToLower(text[sp.Start:sp.End])]; !ok {
			continue
		}
		if sp.Start > last {
			segs = append(segs, Segment{Text: text[last:sp.Start]})
		}
		segs = append(segs, Segment{Text: text[sp.Start:sp.End], Match: true})
		last = sp.End
	}
	if last < len(text) {
		segs = append(segs, Segment{Text: text[last:]})
	}
	return segs
}
