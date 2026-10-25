package crawler

import (
	"net/url"
	"strings"

	"github.com/mukul-work/golang-search-engine/internal/urlnorm"
)

// anyHost in the allowlist means "do not restrict hosts".
const anyHost = "*"

// hostAllowed reports whether host (a lowercase hostname without a port) may
// be crawled. An empty allowlist allows everything (Run never leaves it empty:
// see seedHosts). An entry matches itself and its subdomains; the "." in the
// suffix keeps "notexample.com" out.
func hostAllowed(host string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if a == anyHost || host == a || strings.HasSuffix(host, "."+a) {
			return true
		}
	}
	return false
}

// seedHosts returns the hosts of the seed URLs, ready to use as an allowlist:
// ports dropped, a leading "www." stripped (an entry already covers its
// subdomains, so example.com allows www.example.com), duplicates removed.
// Seeds that are not usable URLs are skipped.
func seedHosts(seeds []string) []string {
	seen := make(map[string]bool)
	var hosts []string
	for _, s := range seeds {
		norm, err := urlnorm.Normalize(s) // lowercases the host
		if err != nil {
			continue
		}
		u, err := url.Parse(norm)
		if err != nil {
			continue
		}
		h := strings.TrimPrefix(u.Hostname(), "www.")
		if h != "" && !seen[h] {
			seen[h] = true
			hosts = append(hosts, h)
		}
	}
	return hosts
}
