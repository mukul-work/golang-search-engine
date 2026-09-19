package indexer

import (
	"regexp"
	"strings"
)

var wordRe = regexp.MustCompile(`[a-zA-Z]+`) // currently it accepts ASCII codes only. I'll add the support for other codes later

// will implement NLP later on
var stopwords = map[string]struct{}{
	"i": {}, "me": {}, "my": {}, "myself": {}, "we": {}, "our": {}, "ours": {},
	"ourselves": {}, "you": {}, "your": {}, "yours": {}, "yourself": {}, "yourselves": {},
	"he": {}, "him": {}, "his": {}, "himself": {}, "she": {}, "her": {}, "hers": {},
	"herself": {}, "it": {}, "its": {}, "itself": {}, "they": {}, "them": {}, "their": {},
	"theirs": {}, "themselves": {}, "what": {}, "which": {}, "who": {}, "whom": {},
	"this": {}, "that": {}, "these": {}, "those": {}, "am": {}, "is": {}, "are": {},
	"was": {}, "were": {}, "be": {}, "been": {}, "being": {}, "have": {}, "has": {},
	"had": {}, "having": {}, "do": {}, "does": {}, "did": {}, "doing": {}, "a": {},
	"an": {}, "the": {}, "and": {}, "but": {}, "if": {}, "or": {}, "because": {},
	"as": {}, "until": {}, "while": {}, "of": {}, "at": {}, "by": {}, "for": {},
	"with": {}, "about": {}, "against": {}, "between": {}, "into": {}, "through": {},
	"during": {}, "before": {}, "after": {}, "above": {}, "below": {}, "to": {},
	"from": {}, "up": {}, "down": {}, "in": {}, "out": {}, "on": {}, "off": {},
	"over": {}, "under": {}, "again": {}, "further": {}, "then": {}, "once": {},
	"here": {}, "there": {}, "when": {}, "where": {}, "why": {}, "how": {}, "all": {},
	"any": {}, "both": {}, "each": {}, "few": {}, "more": {}, "most": {}, "other": {},
	"some": {}, "such": {}, "no": {}, "nor": {}, "not": {}, "only": {}, "own": {},
	"same": {}, "so": {}, "than": {}, "too": {}, "very": {}, "s": {}, "t": {},
	"can": {}, "will": {}, "just": {}, "don": {}, "should": {}, "now": {},
}

func Tokenize(text string) map[string]int {
	counts := make(map[string]int)
	words := wordRe.FindAllString(strings.ToLower(text), -1)
	for _, w := range words {
		if _, isStopWord := stopwords[w]; isStopWord {
			continue
		}
		counts[w]++
	}
	return counts
}
