package urlnorm

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases scheme and host, keeps path case", "HTTP://Example.COM/Path", "http://example.com/Path"},
		{"empty path becomes slash", "https://example.com", "https://example.com/"},
		{"drops fragment", "https://example.com/a#section", "https://example.com/a"},
		{"drops default http port", "http://example.com:80/a", "http://example.com/a"},
		{"drops default https port", "https://example.com:443/a", "https://example.com/a"},
		{"keeps non-default port", "https://example.com:8443/a", "https://example.com:8443/a"},
		{"443 is not default for http", "http://example.com:443/a", "http://example.com:443/a"},
		{"drops credentials", "https://user:pw@example.com/a", "https://example.com/a"},
		{"sorts query parameters", "https://example.com/a?b=2&a=1", "https://example.com/a?a=1&b=2"},
		{"removes tracking parameters", "https://example.com/a?utm_source=x&id=7&fbclid=abc", "https://example.com/a?id=7"},
		{"query of only tracking params disappears", "https://example.com/a?utm_source=x", "https://example.com/a"},
		{"bare question mark disappears", "https://example.com/a?", "https://example.com/a"},
		{"trims surrounding spaces", "  https://example.com/a  ", "https://example.com/a"},
		{"keeps trailing slash", "https://example.com/a/", "https://example.com/a/"},
		{"preserves percent-encoding", "https://example.com/a%2Fb", "https://example.com/a%2Fb"},
		{"ipv6 with default port", "http://[::1]:80/x", "http://[::1]/x"},
		{"ipv6 with custom port", "http://[::1]:8080/x", "http://[::1]:8080/x"},
		{"everything at once", "HTTPS://EXAMPLE.com:443", "https://example.com/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize(tc.in)
			if err != nil {
				t.Fatalf("Normalize(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// Property: normalizing twice changes nothing.
			again, err := Normalize(got)
			if err != nil || again != got {
				t.Errorf("not idempotent: Normalize(%q) = %q, %v", got, again, err)
			}
		})
	}
}

func TestNormalize_Rejects(t *testing.T) {
	for _, in := range []string{
		"mailto:a@b.com",
		"javascript:void(0)",
		"tel:+123456",
		"ftp://example.com/file",
		"/relative/path",
		"#top",
		"",
		"https://",
		"http://exa mple.com",
	} {
		t.Run(in, func(t *testing.T) {
			if got, err := Normalize(in); err == nil {
				t.Errorf("Normalize(%q) = %q, want an error", in, got)
			}
		})
	}
}
