package config

import (
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// env turns a map into the getenv function Load expects.
// env turns a map into the getenv function Load expects. CRAWLER_SEEDS is
// filled in with a valid seed unless the test sets it (even to ""), because
// there are no built-in seeds and every valid config needs one.
func env(kv map[string]string) func(string) string {
	return func(key string) string {
		if v, ok := kv[key]; ok {
			return v
		}
		if key == "CRAWLER_SEEDS" {
			return "https://seed.test/"
		}
		return ""
	}
}

const dbURL = "postgres://user:pw@localhost/db"

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{"DB_URL": dbURL}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Default()
	want.DatabaseURL = dbURL
	want.Seeds = []string{"https://seed.test/"} // supplied by the env helper
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestLoad_EnvOverridesDefaults(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{
		"DB_URL":                 dbURL,
		"CRAWLER_WORKERS":        "3",
		"CRAWLER_TIMEOUT":        "2s",
		"CRAWLER_HOST_INTERVAL":  "500ms",
		"CRAWLER_MAX_PAGE_BYTES": "1024",
		"CRAWLER_SEEDS":          "https://a.test, https://b.test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workers != 3 || cfg.RequestTimeout != 2*time.Second ||
		cfg.HostInterval != 500*time.Millisecond || cfg.MaxPageBytes != 1024 {
		t.Errorf("env values not applied: %+v", cfg)
	}
	if want := []string{"https://a.test", "https://b.test"}; !reflect.DeepEqual(cfg.Seeds, want) {
		t.Errorf("Seeds = %v, want %v", cfg.Seeds, want)
	}
}

func TestLoad_FlagsOverrideEnv(t *testing.T) {
	cfg, err := Load(
		[]string{"-workers", "7", "-seeds", "https://c.test"},
		env(map[string]string{"DB_URL": dbURL, "CRAWLER_WORKERS": "3", "CRAWLER_TIMEOUT": "2s"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workers != 7 {
		t.Errorf("Workers = %d, want 7 (flag beats env)", cfg.Workers)
	}
	if cfg.RequestTimeout != 2*time.Second {
		t.Errorf("RequestTimeout = %v, want 2s (env kept: no flag passed)", cfg.RequestTimeout)
	}
	if want := []string{"https://c.test"}; !reflect.DeepEqual(cfg.Seeds, want) {
		t.Errorf("Seeds = %v, want %v", cfg.Seeds, want)
	}
}

func TestLoad_ZeroHostIntervalIsAllowed(t *testing.T) {
	cfg, err := Load([]string{"-host-interval=0s"}, env(map[string]string{"DB_URL": dbURL}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HostInterval != 0 {
		t.Errorf("HostInterval = %v, want 0", cfg.HostInterval)
	}
}

func TestLoad_Errors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{"missing DB_URL", nil, nil},
		{"workers not a number", nil, map[string]string{"DB_URL": dbURL, "CRAWLER_WORKERS": "many"}},
		{"zero workers", []string{"-workers=0"}, map[string]string{"DB_URL": dbURL}},
		{"bad duration", nil, map[string]string{"DB_URL": dbURL, "CRAWLER_TIMEOUT": "fast"}},
		{"zero timeout", []string{"-timeout=0s"}, map[string]string{"DB_URL": dbURL}},
		{"negative interval", []string{"-host-interval=-1s"}, map[string]string{"DB_URL": dbURL}},
		{"non-http seed", []string{"-seeds=ftp://a.test"}, map[string]string{"DB_URL": dbURL}},
		{"seed without host", []string{"-seeds=https://"}, map[string]string{"DB_URL": dbURL}},
		{"bad log level in env", nil, map[string]string{"DB_URL": dbURL, "LOG_LEVEL": "loud"}},
		{"bad log level flag", []string{"-log-level=loud"}, map[string]string{"DB_URL": dbURL}},
		{"bad log format", []string{"-log-format=xml"}, map[string]string{"DB_URL": dbURL}},
		{"negative max depth", []string{"-max-depth=-1"}, map[string]string{"DB_URL": dbURL}},
		{"max depth not a number", nil, map[string]string{"DB_URL": dbURL, "CRAWLER_MAX_DEPTH": "deep"}},
		{"zero max pages", []string{"-max-pages=0"}, map[string]string{"DB_URL": dbURL}},
		{"allowed host with scheme", []string{"-allowed-hosts=https://a.test"}, map[string]string{"DB_URL": dbURL}},
		{"allowed host with port", []string{"-allowed-hosts=a.test:8080"}, map[string]string{"DB_URL": dbURL}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Load(tc.args, env(tc.env)); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestLoad_LoggingFromEnv(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{"DB_URL": dbURL, "LOG_LEVEL": "debug", "LOG_FORMAT": "JSON"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want debug", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json (lowercased)", cfg.LogFormat)
	}
}

func TestLoad_LogLevelFlagOverridesEnv(t *testing.T) {
	cfg, err := Load([]string{"-log-level=warn"}, env(map[string]string{"DB_URL": dbURL, "LOG_LEVEL": "debug"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != slog.LevelWarn {
		t.Errorf("LogLevel = %v, want warn", cfg.LogLevel)
	}
}

func TestLoad_ScopeFromEnv(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{
		"DB_URL":                dbURL,
		"CRAWLER_MAX_DEPTH":     "3",
		"CRAWLER_MAX_PAGES":     "50",
		"CRAWLER_ALLOWED_HOSTS": "Example.COM, b.test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxDepth != 3 || cfg.MaxPages != 50 {
		t.Errorf("depth=%d pages=%d, want 3 and 50", cfg.MaxDepth, cfg.MaxPages)
	}
	if want := []string{"example.com", "b.test"}; !reflect.DeepEqual(cfg.AllowedHosts, want) {
		t.Errorf("AllowedHosts = %v, want %v (lowercased, trimmed)", cfg.AllowedHosts, want)
	}
}

func TestLoad_ScopeFlagsOverrideEnv(t *testing.T) {
	cfg, err := Load(
		[]string{"-max-depth=0", "-max-pages=10", "-allowed-hosts=c.test"},
		env(map[string]string{"DB_URL": dbURL, "CRAWLER_MAX_DEPTH": "3", "CRAWLER_ALLOWED_HOSTS": "a.test"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxDepth != 0 { // zero is valid: seeds only
		t.Errorf("MaxDepth = %d, want 0", cfg.MaxDepth)
	}
	if cfg.MaxPages != 10 {
		t.Errorf("MaxPages = %d, want 10", cfg.MaxPages)
	}
	if want := []string{"c.test"}; !reflect.DeepEqual(cfg.AllowedHosts, want) {
		t.Errorf("AllowedHosts = %v, want %v", cfg.AllowedHosts, want)
	}
}

func TestLoad_EmptyAllowedHostsFlagClearsEnv(t *testing.T) {
	cfg, err := Load([]string{"-allowed-hosts="}, env(map[string]string{"DB_URL": dbURL, "CRAWLER_ALLOWED_HOSTS": "a.test"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.AllowedHosts) != 0 {
		t.Errorf("AllowedHosts = %v, want none", cfg.AllowedHosts)
	}
}

func TestLoadServer_Defaults(t *testing.T) {
	cfg, err := LoadServer(nil, env(map[string]string{"DB_URL": dbURL}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := DefaultServer()
	want.DatabaseURL = dbURL
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadServer_EnvThenFlags(t *testing.T) {
	cfg, err := LoadServer(
		[]string{"-addr=:9090"},
		env(map[string]string{
			"DB_URL": dbURL, "HTTP_ADDR": "127.0.0.1:7000", "LOG_LEVEL": "debug", "LOG_FORMAT": "JSON",
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":9090" {
		t.Errorf("Addr = %q, want :9090 (flag beats env)", cfg.Addr)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want debug (from env)", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json (lowercased)", cfg.LogFormat)
	}
}

func TestLoadServer_Errors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{"missing DB_URL", nil, nil},
		{"address without a port", []string{"-addr=localhost"}, map[string]string{"DB_URL": dbURL}},
		{"address is not an address", []string{"-addr=nonsense"}, map[string]string{"DB_URL": dbURL}},
		{"port out of range", []string{"-addr=:70000"}, map[string]string{"DB_URL": dbURL}},
		{"port zero", []string{"-addr=:0"}, map[string]string{"DB_URL": dbURL}},
		{"bad log level", nil, map[string]string{"DB_URL": dbURL, "LOG_LEVEL": "loud"}},
		{"bad log format", []string{"-log-format=xml"}, map[string]string{"DB_URL": dbURL}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadServer(tc.args, env(tc.env)); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func writeSeeds(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "seeds.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing seeds file: %v", err)
	}
	return path
}

func TestReadSeedsFile(t *testing.T) {
	path := writeSeeds(t, "\ufeff# my corpus\n\nhttps://a.test/\r\n  https://b.test/page#intro  \n# https://skipped.test/\n")

	got, err := ReadSeedsFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Comments, blank lines, the BOM and Windows line endings are handled,
	// and the # inside a URL is kept.
	want := []string{"https://a.test/", "https://b.test/page#intro"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLoad_SeedsFileReplacesOtherSeeds(t *testing.T) {
	path := writeSeeds(t, "https://file.test/\n")
	cfg, err := Load(
		[]string{"-seeds-file=" + path, "-seeds=https://flag.test/"},
		env(map[string]string{"DB_URL": dbURL, "CRAWLER_SEEDS": "https://env.test/"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"https://file.test/"}; !reflect.DeepEqual(cfg.Seeds, want) {
		t.Errorf("Seeds = %v, want %v (the file wins)", cfg.Seeds, want)
	}
}

func TestLoad_SeedsFileFromEnv(t *testing.T) {
	path := writeSeeds(t, "https://file.test/\n")
	cfg, err := Load(nil, env(map[string]string{"DB_URL": dbURL, "CRAWLER_SEEDS_FILE": path}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"https://file.test/"}; !reflect.DeepEqual(cfg.Seeds, want) {
		t.Errorf("Seeds = %v, want %v", cfg.Seeds, want)
	}
}

func TestLoad_SeedsErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{"no seeds anywhere", nil, map[string]string{"DB_URL": dbURL, "CRAWLER_SEEDS": ""}},
		{"seeds file does not exist", []string{"-seeds-file=/nonexistent/seeds.txt"}, map[string]string{"DB_URL": dbURL}},
		{"seeds file has only comments", []string{"-seeds-file=" + writeSeeds(t, "# nothing here\n\n")}, map[string]string{"DB_URL": dbURL}},
		{"seeds file has a bad URL", []string{"-seeds-file=" + writeSeeds(t, "ftp://nope.test/\n")}, map[string]string{"DB_URL": dbURL}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Load(tc.args, env(tc.env)); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestLoadServer_RateLimitSettings(t *testing.T) {
	cfg, err := LoadServer(
		[]string{"-rate-burst=5"},
		env(map[string]string{"DB_URL": dbURL, "RATE_LIMIT_PER_MIN": "30", "RATE_LIMIT_BURST": "20", "TRUSTED_PROXIES": "172.16.0.0/12, 10.1.2.3"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RateLimitPerMin != 30 {
		t.Errorf("RateLimitPerMin = %d, want 30 (from env)", cfg.RateLimitPerMin)
	}
	if cfg.RateBurst != 5 {
		t.Errorf("RateBurst = %d, want 5 (the flag beats env)", cfg.RateBurst)
	}
	want := []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("10.1.2.3/32")}
	if !reflect.DeepEqual(cfg.TrustedProxies, want) {
		t.Errorf("TrustedProxies = %v, want %v", cfg.TrustedProxies, want)
	}
}

func TestLoadServer_RateLimitCanBeTurnedOff(t *testing.T) {
	cfg, err := LoadServer([]string{"-rate-limit=0", "-rate-burst=0"}, env(map[string]string{"DB_URL": dbURL}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RateLimitPerMin != 0 {
		t.Errorf("RateLimitPerMin = %d, want 0 (off)", cfg.RateLimitPerMin)
	}
}

func TestLoadServer_RateLimitErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{"negative rate", []string{"-rate-limit=-1"}, map[string]string{"DB_URL": dbURL}},
		{"zero burst while limiting is on", []string{"-rate-burst=0"}, map[string]string{"DB_URL": dbURL}},
		{"rate is not a number", nil, map[string]string{"DB_URL": dbURL, "RATE_LIMIT_PER_MIN": "many"}},
		{"bad proxy address", []string{"-trusted-proxies=not-an-ip"}, map[string]string{"DB_URL": dbURL}},
		{"bad proxy range", nil, map[string]string{"DB_URL": dbURL, "TRUSTED_PROXIES": "10.0.0.0/99"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadServer(tc.args, env(tc.env)); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}
