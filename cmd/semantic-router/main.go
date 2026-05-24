// Package main is the entry point for the semantic-router service.
// It initializes the router, loads configuration, and starts the HTTP server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	// defaultAddr is the default address the server listens on.
	defaultAddr = ":8080"
	// defaultShutdownTimeout is the maximum time to wait for graceful shutdown.
	// Reduced from 30s to 10s for faster local dev iteration.
	defaultShutdownTimeout = 10 * time.Second

	// defaultReadTimeout and defaultWriteTimeout are tuned for LLM backends,
	// which can be slow to respond under load. Bumped from 15s to 60s.
	defaultReadTimeout  = 60 * time.Second
	defaultWriteTimeout = 60 * time.Second
	defaultIdleTimeout  = 120 * time.Second
)

func main() {
	addr := flag.String("addr", defaultAddr, "HTTP server listen address")
	logLevel := flag.String("log-level", "info", "Log level: debug, info, warn, error")
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Configure structured logger.
	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level %q: %v\n", *logLevel, err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	slog.Info("starting semantic-router",
		"addr", *addr,
		"log_level", *logLevel,
		"config", *configPath,
	)

	// TODO: load config from *configPath and wire up router.
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/readyz", readyzHandler)

	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
		IdleTimeout:  defaultIdleTimeout,
	}

	// Start server in a goroutine so we can listen for shutdown signals.
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait for interrupt or termination signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		slog.Error("server error", "err", err)
		os.Exit(1)
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)
	}

	// Graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

// healthzHandler responds to liveness probe requests.
func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// readyzHandler responds to readiness probe requests.
func readyzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}
