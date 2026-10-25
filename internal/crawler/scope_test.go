package crawler

import (
	"reflect"
	"testing"
)

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
		{"star allows any host", "any.test", []string{"*"}, true},
		{"star among other entries", "any.test", []string{"a.test", "*"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostAllowed(tc.host, tc.allowed); got != tc.want {
				t.Errorf("hostAllowed(%q, %v) = %v, want %v", tc.host, tc.allowed, got, tc.want)
			}
		})
	}
}

func TestSeedHosts(t *testing.T) {
	got := seedHosts([]string{
		"https://www.Example.com/a",
		"https://example.com/b", // same site as the line above: one entry
		"http://blog.other.test:8080/x",
		"mailto:me@example.com", // not usable: skipped
		"/relative",             // not usable: skipped
	})
	want := []string{"example.com", "blog.other.test"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("seedHosts = %v, want %v", got, want)
	}
	if got := seedHosts(nil); len(got) != 0 {
		t.Errorf("seedHosts(nil) = %v, want none", got)
	}
}
