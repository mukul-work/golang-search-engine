package config

import (
	"os"
	"strings"
)

// ReadSeedsFile reads seed URLs from path: one per line, blank lines and
// lines starting with # ignored. A # inside a line is part of the URL (a
// fragment), so only whole-line comments are recognised.
func ReadSeedsFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimPrefix(string(data), "\ufeff") // some Windows editors add a BOM
	var seeds []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line) // also strips the \r of Windows line endings
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		seeds = append(seeds, line)
	}
	return seeds, nil
}
