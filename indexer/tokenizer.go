package indexer

import (
	"regexp"
	"strings"
)

var wordRe = regexp.MustCompile(`[a-zA-Z]+`) // currently it accepts ASCII codes only. I'll add the support for other codes later

func Tokenize(text string) map[string]int {
	counts := make(map[string]int)
	words := wordRe.FindAllString(strings.ToLower(text), -1)
	for _, w := range words {
		counts[w]++
	}
	return counts
}
