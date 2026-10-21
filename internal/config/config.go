package config

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL    string
	Seeds          []string
	Workers        int
	RequestTimeout time.Duration
	MaxPageBytes   int64
	HostInterval   time.Duration // minimum gap between requests to one host; 0 = no limit
	LogLevel       slog.Level
	LogFormat      string // "text" or "json"
}

// DatabaseURL has no default: it must be provided.
func Default() Config {
	return Config{
		Seeds: []string{ // temporary: seeds will move to a file or the database later
			"https://example.com",
			"https://scrapfly.io/blog/posts/best-websites-to-practice-web-scraping",
			"https://www.searchenginejournal.com/alternative-search-engines/271409/",
		},
		Workers:        10,
		RequestTimeout: 10 * time.Second,
		MaxPageBytes:   5 << 20,
		HostInterval:   time.Second,
		LogLevel:       slog.LevelInfo,
		LogFormat:      "text",
	}
}

// LoadDotEnv loads variables from path into the process environment.
// A missing file is fine; variables that are already set are never overridden.
func LoadDotEnv(path string) error {
	if err := godotenv.Load(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Load builds a Config. Precedence: flags > environment > defaults.
func Load(args []string, getenv func(string) string) (Config, error) {
	cfg := Default()
	cfg.DatabaseURL = getenv("DB_URL")

	// 1. Environment overrides defaults.
	if v := getenv("CRAWLER_WORKERS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("CRAWLER_WORKERS: %w", err)
		}
		cfg.Workers = n
	}
	if v := getenv("CRAWLER_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("CRAWLER_TIMEOUT: %w", err)
		}
		cfg.RequestTimeout = d
	}
	if v := getenv("CRAWLER_HOST_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("CRAWLER_HOST_INTERVAL: %w", err)
		}
		cfg.HostInterval = d
	}
	if v := getenv("CRAWLER_MAX_PAGE_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("CRAWLER_MAX_PAGE_BYTES: %w", err)
		}
		cfg.MaxPageBytes = n
	}
	if v := getenv("CRAWLER_SEEDS"); v != "" {
		cfg.Seeds = splitList(v)
	}

	if v := getenv("LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return Config{}, fmt.Errorf("LOG_LEVEL: %w", err)
		}
	}
	if v := getenv("LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
	}

	// 2. Flags override the environment. The current values become the flag
	// defaults, so a flag only wins when it is actually passed.
	fs := flag.NewFlagSet("crawler", flag.ContinueOnError)
	seeds := strings.Join(cfg.Seeds, ",")
	fs.StringVar(&seeds, "seeds", seeds, "comma-separated seed URLs")
	fs.IntVar(&cfg.Workers, "workers", cfg.Workers, "number of concurrent workers")
	fs.DurationVar(&cfg.RequestTimeout, "timeout", cfg.RequestTimeout, "timeout per HTTP request")
	fs.DurationVar(&cfg.HostInterval, "host-interval", cfg.HostInterval, "minimum gap between requests to the same host (0 = no limit)")
	fs.Int64Var(&cfg.MaxPageBytes, "max-page-bytes", cfg.MaxPageBytes, "maximum bytes read per page")
	fs.TextVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug, info, warn or error")
	fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: text or json")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	cfg.Seeds = splitList(seeds)
	cfg.LogFormat = strings.ToLower(cfg.LogFormat)
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DB_URL is required"))
	}
	if c.Workers < 1 {
		errs = append(errs, fmt.Errorf("workers must be at least 1, got %d", c.Workers))
	}
	if c.RequestTimeout <= 0 {
		errs = append(errs, fmt.Errorf("timeout must be positive, got %v", c.RequestTimeout))
	}
	if c.MaxPageBytes <= 0 {
		errs = append(errs, fmt.Errorf("max page bytes must be positive, got %d", c.MaxPageBytes))
	}
	if c.HostInterval < 0 {
		errs = append(errs, fmt.Errorf("host interval must not be negative, got %v", c.HostInterval))
	}
	if len(c.Seeds) == 0 {
		errs = append(errs, errors.New("at least one seed URL is required"))
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		errs = append(errs, fmt.Errorf("log format must be text or json, got %q", c.LogFormat))
	}
	for _, s := range c.Seeds {
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("invalid seed URL %q (need http or https with a host)", s))
		}
	}
	return errors.Join(errs...) // nil when errs is empty
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
