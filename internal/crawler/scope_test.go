package crawler

import "testing"

func TestHostAllowed(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		allowed []string
		want    bool
	}{
		{"empty list allows everything", "any.test", nil, true},
		{"exact match", "example.com", []string{"example.com"}, true},
		{"subdomain match", "www.example.com", []string{"example.com"}, true},
		{"deep subdomain match", "a.b.example.com", []string{"example.com"}, true},
		{"shared suffix is not a match", "notexample.com", []string{"example.com"}, false},
		{"parent of a subdomain entry is not a match", "example.com", []string{"www.example.com"}, false},
		{"one of several", "b.test", []string{"a.test", "b.test"}, true},
		{"unrelated host", "other.test", []string{"example.com"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostAllowed(tc.host, tc.allowed); got != tc.want {
				t.Errorf("hostAllowed(%q, %v) = %v, want %v", tc.host, tc.allowed, got, tc.want)
			}
		})
	}
}
