package config

import (
	"log/slog"
	"reflect"
	"testing"
	"time"
)

// env turns a map into the getenv function Load expects.
func env(kv map[string]string) func(string) string {
	return func(key string) string { return kv[key] }
}

const dbURL = "postgres://user:pw@localhost/db"

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{"DB_URL": dbURL}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Default()
	want.DatabaseURL = dbURL
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
