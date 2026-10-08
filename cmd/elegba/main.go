// filepath: elegba/cmd/elegba/main.go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"

	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/elegba-dev/elegba/internal/adminserver"
	"github.com/elegba-dev/elegba/internal/config"
	"github.com/elegba-dev/elegba/internal/engine"
	"github.com/elegba-dev/elegba/internal/observability"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Elegba stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := os.Getenv("ELEGBA_CONFIG")
	if configPath == "" {
		configPath = "elegba.yaml"
	}
	logLevel := os.Getenv("ELEGBA_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	configFlag := flag.String("config", configPath, "path to the Elegba YAML or JSON configuration")
	logLevelFlag := flag.String("log-level", logLevel, "log level: debug, info, warn, or error")
	flag.Parse()
	level := new(slog.LevelVar)
	switch strings.ToLower(*logLevelFlag) {
	case "debug":
		level.Set(slog.LevelDebug)
	case "info":
		level.Set(slog.LevelInfo)
	case "warn", "warning":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		return fmt.Errorf("invalid log level %q", *logLevelFlag)
	}
	slog.SetDefault(slog.New(observability.NewRedactingHandler(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))))

	cfg, err := config.LoadConfig(*configFlag)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	if address := os.Getenv("ELEGBA_ADMIN_ADDR"); address != "" {
		cfg.Server.Admin.Address = address
		cfg.Server.Admin.Enabled = true
	}
	if endpoint := os.Getenv("ELEGBA_OTEL_ENDPOINT"); endpoint != "" {
		cfg.Tracing.Endpoint = endpoint
		cfg.Tracing.Enabled = true
	}
	if insecure := os.Getenv("ELEGBA_OTEL_INSECURE"); insecure != "" {
		cfg.Tracing.Insecure, err = strconv.ParseBool(insecure)
		if err != nil {
			return fmt.Errorf("invalid ELEGBA_OTEL_INSECURE value: %w", err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	tracing, err := observability.ConfigureTracing(context.Background(), cfg.Tracing)
	if err != nil {
		return fmt.Errorf("initialize tracing: %w", err)
	}
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracing.Shutdown(shutdownContext); err != nil {
			slog.Error("tracing shutdown failed", "error", err)
		}
	}()
	appServer, err := engine.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize engine: %w", err)
	}
	defer appServer.Close()
	if hotReloadEnabled() {
		if err := appServer.Watch(context.Background(), *configFlag, hotReloadDebounce()); err != nil {
			return fmt.Errorf("start configuration watcher: %w", err)
		}
	}

	srv := &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           appServer,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeout),
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeout),
		IdleTimeout:       time.Duration(cfg.Server.IdleTimeout),
		ReadHeaderTimeout: time.Duration(cfg.Server.ReadHeaderTimeout),
	}
	var adminServer *http.Server
	if cfg.Server.Admin.Enabled {
		adminServer = &http.Server{
			Addr:              cfg.Server.Admin.Address,
			Handler:           adminserver.NewHandler(appServer.MetricsHandler(), http.HandlerFunc(appServer.HealthHandler), http.HandlerFunc(appServer.ReadyHandler)),
			ReadTimeout:       time.Duration(cfg.Server.ReadTimeout),
			WriteTimeout:      time.Duration(cfg.Server.WriteTimeout),
			IdleTimeout:       time.Duration(cfg.Server.IdleTimeout),
			ReadHeaderTimeout: time.Duration(cfg.Server.ReadHeaderTimeout),
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 2)
	go func() {
		slog.Info("starting Elegba", "address", srv.Addr)
		serverErrors <- srv.ListenAndServe()
	}()
	if adminServer != nil {
		go func() {
			slog.Info("starting Elegba admin server", "address", adminServer.Addr)
			serverErrors <- adminServer.ListenAndServe()
		}()
	}
	var serverErr error
	select {
	case err := <-serverErrors:
		serverErr = err
	case <-ctx.Done():
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Server.ShutdownTimeout))
	defer cancel()
	if err := srv.Shutdown(shutdownContext); err != nil {
		_ = srv.Close()
		return fmt.Errorf("main server shutdown: %w", err)
	}
	if adminServer != nil {
		if err := adminServer.Shutdown(shutdownContext); err != nil {
			_ = adminServer.Close()
			return fmt.Errorf("admin server shutdown: %w", err)
		}
	}
	if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
		return fmt.Errorf("server failed: %w", serverErr)
	}
	slog.Info("Elegba stopped")
	return nil
}

func hotReloadEnabled() bool {
	value := os.Getenv("ELEGBA_HOT_RELOAD")
	if value == "" {
		return true
	}
	enabled, err := strconv.ParseBool(value)
	return err == nil && enabled
}

func hotReloadDebounce() time.Duration {
	value := os.Getenv("ELEGBA_HOT_RELOAD_DEBOUNCE")
	if value == "" {
		return 500 * time.Millisecond
	}
	debounce, err := time.ParseDuration(value)
	if err != nil || debounce <= 0 {
		return 500 * time.Millisecond
	}
	return debounce
}
