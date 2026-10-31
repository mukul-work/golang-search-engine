package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mukul-work/golang-search-engine/internal/config"
	"github.com/mukul-work/golang-search-engine/internal/db"
	"github.com/mukul-work/golang-search-engine/internal/logging"
	"github.com/mukul-work/golang-search-engine/internal/search"
	"github.com/mukul-work/golang-search-engine/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := config.LoadDotEnv(".env.local"); err != nil {
		return fmt.Errorf("loading .env.local: %w", err)
	}
	cfg, err := config.LoadServer(os.Args[1:], os.Getenv)
	if errors.Is(err, flag.ErrHelp) {
		return nil // -h was passed and usage was already printed
	}
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}

	logger := logging.New(os.Stderr, cfg.LogLevel, cfg.LogFormat)

	// ctx is cancelled on Ctrl-C or SIGTERM (what `docker stop` sends).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := db.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to db: %w", err)
	}
	defer store.Close() // runs after Shutdown has drained in-flight requests

	var opts []server.Option
	if cfg.RateLimitPerMin > 0 {
		opts = append(opts, server.WithRateLimit(server.NewRateLimiter(cfg.RateLimitPerMin, cfg.RateBurst)))
	}
	if len(cfg.TrustedProxies) > 0 {
		opts = append(opts, server.WithTrustedProxies(cfg.TrustedProxies))
	}
	logger.Info("rate limit", "per_minute", cfg.RateLimitPerMin, "burst", cfg.RateBurst, "trusted_proxies", cfg.TrustedProxies)
	app := server.New(search.NewService(store), logger, opts...)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,  // time to receive the request headers
		ReadTimeout:       10 * time.Second, // time to receive the whole request
		WriteTimeout:      15 * time.Second, // must exceed the 5s query timeout
		IdleTimeout:       60 * time.Second, // keep-alive connections
		MaxHeaderBytes:    16 << 10,         // 16 KiB
	}

	errc := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", cfg.Addr)
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil // the normal result of calling Shutdown
		}
		errc <- err
	}()

	select {
	case err := <-errc:
		return err // could not start (port in use, say) or died on its own
	case <-ctx.Done():
	}
	stop() // restore default signal handling: a second Ctrl-C now kills the process
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	return <-errc // wait for ListenAndServe's goroutine to finish
}
