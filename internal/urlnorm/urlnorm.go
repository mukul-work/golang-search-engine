// Package urlnorm turns URLs into a canonical form so that the same page is
// not crawled or indexed under several spellings.
package urlnorm

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Normalize returns the canonical form of raw. It returns an error for
// anything that is not an absolute http or https URL with a host.
func Normalize(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	// url.Parse already lowercases the scheme.
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", errors.New("missing host")
	}

	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = "" // default port: not part of the identity
	}
	if strings.Contains(host, ":") { // IPv6 literal: Hostname strips the brackets
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	u.Host = host

	u.User = nil // credentials are not part of the page's identity
	u.Fragment = ""
	u.RawFragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	u.RawQuery = cleanQuery(u.RawQuery)
	u.ForceQuery = false // a bare trailing "?" means nothing
	return u.String(), nil
}

// cleanQuery drops tracking parameters and sorts the rest. Parameters are
// kept exactly as written (no re-encoding); only their order changes.
func cleanQuery(raw string) string {
	if raw == "" {
		return ""
	}
	var kept []string
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		key, _, _ := strings.Cut(pair, "=")
		if k, err := url.QueryUnescape(key); err == nil {
			key = k
		}
		if isTracking(strings.ToLower(key)) {
			continue
		}
		kept = append(kept, pair)
	}
	sort.Strings(kept)
	return strings.Join(kept, "&")
}

func isTracking(key string) bool {
	switch key {
	case "fbclid", "gclid", "msclkid", "mc_cid", "mc_eid":
		return true
	}
	return strings.HasPrefix(key, "utm_")
}
