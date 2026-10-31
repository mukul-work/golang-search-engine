package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// ServerConfig holds the settings for the web server binary.
type ServerConfig struct {
	DatabaseURL     string
	Addr            string // host:port to listen on
	LogLevel        slog.Level
	LogFormat       string         // "text" or "json"
	RateLimitPerMin int            // requests per minute per client; 0 turns limiting off
	RateBurst       int            // how many requests a client may make in a quick burst
	TrustedProxies  []netip.Prefix // reverse proxies whose X-Forwarded-For is believed
}

func DefaultServer() ServerConfig {
	return ServerConfig{
		Addr:            "127.0.0.1:8080", // loopback only; deployments set HTTP_ADDR explicitly
		LogLevel:        slog.LevelInfo,
		LogFormat:       "text",
		RateLimitPerMin: 60,
		RateBurst:       10,
	}
}

// LoadServer builds a ServerConfig. Precedence: flags > environment > defaults.
func LoadServer(args []string, getenv func(string) string) (ServerConfig, error) {
	cfg := DefaultServer()
	cfg.DatabaseURL = getenv("DB_URL")

	if v := getenv("HTTP_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := getenv("LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return ServerConfig{}, fmt.Errorf("LOG_LEVEL: %w", err)
		}
	}
	if v := getenv("LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
	}

	if v := getenv("RATE_LIMIT_PER_MIN"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return ServerConfig{}, fmt.Errorf("RATE_LIMIT_PER_MIN: %w", err)
		}
		cfg.RateLimitPerMin = n
	}
	if v := getenv("RATE_LIMIT_BURST"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return ServerConfig{}, fmt.Errorf("RATE_LIMIT_BURST: %w", err)
		}
		cfg.RateBurst = n
	}
	if v := getenv("TRUSTED_PROXIES"); v != "" {
		p, err := parseProxies(v)
		if err != nil {
			return ServerConfig{}, fmt.Errorf("TRUSTED_PROXIES: %w", err)
		}
		cfg.TrustedProxies = p
	}

	flagSet := flag.NewFlagSet("server", flag.ContinueOnError)
	flagSet.StringVar(&cfg.Addr, "addr", cfg.Addr, "address to listen on, e.g. 127.0.0.1:8080 or :8080")
	flagSet.TextVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug, info, warn or error")
	flagSet.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: text or json")
	flagSet.IntVar(&cfg.RateLimitPerMin, "rate-limit", cfg.RateLimitPerMin, "requests per minute allowed per client IP (0 = no limit)")
	flagSet.IntVar(&cfg.RateBurst, "rate-burst", cfg.RateBurst, "requests a client may make in a quick burst")
	flagSet.Func("trusted-proxies", "comma-separated IPs or CIDRs of reverse proxies whose X-Forwarded-For is believed (default: none)", func(v string) error {
		p, err := parseProxies(v)
		if err != nil {
			return err
		}
		cfg.TrustedProxies = p
		return nil
	})
	if err := flagSet.Parse(args); err != nil {
		return ServerConfig{}, err
	}
	cfg.LogFormat = strings.ToLower(cfg.LogFormat)

	if err := cfg.validate(); err != nil {
		return ServerConfig{}, err
	}
	return cfg, nil
}

func (c ServerConfig) validate() error {
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DB_URL is required"))
	}
	if _, port, err := net.SplitHostPort(c.Addr); err != nil {
		errs = append(errs, fmt.Errorf("invalid address %q: %w", c.Addr, err))
	} else if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		errs = append(errs, fmt.Errorf("invalid port in address %q (need 1-65535)", c.Addr))
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		errs = append(errs, fmt.Errorf("log format must be text or json, got %q", c.LogFormat))
	}
	if c.RateLimitPerMin < 0 {
		errs = append(errs, fmt.Errorf("rate limit must not be negative, got %d", c.RateLimitPerMin))
	}
	if c.RateLimitPerMin > 0 && c.RateBurst < 1 {
		errs = append(errs, fmt.Errorf("rate burst must be at least 1 when rate limiting is on, got %d", c.RateBurst))
	}
	return errors.Join(errs...)
}

// parseProxies reads a comma-separated list of IP addresses or CIDR ranges.
func parseProxies(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, item := range splitList(s) {
		if strings.Contains(item, "/") {
			p, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, fmt.Errorf("invalid proxy range %q: %w", item, err)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(item)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy address %q: %w", item, err)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen())) // a single address is a /32 or /128
	}
	return out, nil
}
