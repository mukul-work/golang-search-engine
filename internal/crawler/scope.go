package crawler

import "strings"

// hostAllowed reports whether host (a lowercase hostname without a port) may
// be crawled. An empty allowlist allows everything. An entry matches itself
// and its subdomains; the "." in the suffix keeps "notexample.com" out.
func hostAllowed(host string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if host == a || strings.HasSuffix(host, "."+a) {
			return true
		}
	}
	return false
}
