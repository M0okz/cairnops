package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/M0okz/cairnops/internal/config"
	"github.com/M0okz/cairnops/internal/pushrelay"
	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/M0okz/cairnops/internal/version"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		pinger := pushrelay.HealthPinger{URL: "http://127.0.0.1:8082/v1/health"}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := pinger.Ping(ctx); err != nil {
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("push relay stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadPushRelay()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	secrets, err := secretbox.LoadOrCreate(cfg.MasterKeyFile)
	if err != nil {
		return fmt.Errorf("load relay master key: %w", err)
	}
	store, err := pushrelay.NewFileStore(cfg.StorageDir, secrets)
	if err != nil {
		return fmt.Errorf("open relay registration store: %w", err)
	}
	provider, err := pushrelay.NewAPNSProvider(
		cfg.APNSTopic, cfg.APNSKeyID, cfg.APNSTeamID,
		cfg.APNSKeyFile, &http.Client{Timeout: 10 * time.Second},
	)
	if err != nil {
		return fmt.Errorf("initialize APNs provider: %w", err)
	}

	server := pushrelay.NewServer(cfg.HTTPAddress, pushrelay.NewHandler(store, provider, logger))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		logger.Info("push relay listening", "address", cfg.HTTPAddress, "version", version.Version)
		errCh <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
