package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
)

// ServerConfig holds the settings for the web server binary.
type ServerConfig struct {
	DatabaseURL string
	Addr        string // host:port to listen on
	LogLevel    slog.Level
	LogFormat   string // "text" or "json"
}

func DefaultServer() ServerConfig {
	return ServerConfig{
		Addr:      "127.0.0.1:8080", // loopback only; deployments set HTTP_ADDR explicitly
		LogLevel:  slog.LevelInfo,
		LogFormat: "text",
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

	flagSet := flag.NewFlagSet("server", flag.ContinueOnError)
	flagSet.StringVar(&cfg.Addr, "addr", cfg.Addr, "address to listen on, e.g. 127.0.0.1:8080 or :8080")
	flagSet.TextVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug, info, warn or error")
	flagSet.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: text or json")
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
	return errors.Join(errs...)
}
