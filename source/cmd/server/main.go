package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sollie/tommedag/internal/api"
	"github.com/sollie/tommedag/internal/avfallsor"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	addr := envOr("TOMMEDAG_ADDR", ":8080")
	cacheTTL := envDurationOr("TOMMEDAG_CACHE_TTL", time.Hour)
	upstreamTimeout := envDurationOr("TOMMEDAG_UPSTREAM_TIMEOUT", 10*time.Second)

	client := avfallsor.NewClient(
		avfallsor.WithCacheTTL(cacheTTL),
		avfallsor.WithHTTPClient(&http.Client{Timeout: upstreamTimeout}),
	)

	router := api.NewRouter(client, logger)

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting server", "addr", addr, "cache_ttl", cacheTTL.String())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	logger.Info("server stopped cleanly")
	return nil
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	// Bare integer is treated as seconds for convenience.
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	return fallback
}
