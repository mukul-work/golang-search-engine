package indexer

// Span is a half-open byte range [Start, End) of some text.
type Span struct {
	Start, End int
}

// WordSpans returns where each word sits in text, using the same definition
// of a word as Tokenize (runs of ASCII letters). The offsets point into text
// itself, not into a lowercased copy.
func WordSpans(text string) []Span {
	locs := wordRe.FindAllStringIndex(text, -1)
	spans := make([]Span, len(locs))
	for i, loc := range locs {
		spans[i] = Span{Start: loc[0], End: loc[1]}
	}
	return spans
}
