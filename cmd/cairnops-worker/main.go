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

	"github.com/M0okz/cairnops/internal/bursts"
	"github.com/M0okz/cairnops/internal/config"
	"github.com/M0okz/cairnops/internal/connectors"
	"github.com/M0okz/cairnops/internal/connectors/argus"
	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/database"
	"github.com/M0okz/cairnops/internal/httpapi"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/indicators"
	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/push"
	"github.com/M0okz/cairnops/internal/reconciliation"
	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/M0okz/cairnops/internal/worker"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck("http://127.0.0.1:8081/api/v1/health/ready"))
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadWorker()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()
	secrets, err := secretbox.LoadOrCreate(cfg.MasterKeyFile)
	if err != nil {
		return fmt.Errorf("load master key: %w", err)
	}
	connectorStore := connectors.NewPostgresStore(pool)
	zabbixClient := zabbix.NewClient()
	uptimeKumaClient := uptimekuma.NewClient()
	patchMonClient := patchmon.NewClient()
	argusClient := argus.NewClient()
	incidentStore := incidents.NewPostgresStore(pool)
	incidentService := incidents.NewService(
		incidentStore,
		connectors.NewAcknowledger(connectorStore, zabbixClient, secrets),
	)
	runnerOwner := "worker:" + cfg.InstanceID
	connectorSync := connectors.NewSynchronizer(connectorStore, incidentStore, zabbixClient, secrets, runnerOwner, logger)
	uptimeKumaSync := connectors.NewUptimeKumaSynchronizer(connectorStore, incidentStore, uptimeKumaClient, secrets, runnerOwner, logger)
	patchMonSync := connectors.NewPatchMonSynchronizer(connectorStore, incidentStore, patchMonClient, secrets, runnerOwner, logger)
	argusSync := connectors.NewArgusSynchronizer(connectorStore, incidentStore, argusClient, secrets, runnerOwner, logger)
	indicatorCollector := indicators.NewCollector(
		indicators.NewStore(pool), zabbixClient, uptimeKumaClient, patchMonClient, secrets, logger,
	)
	burstAcknowledgements := bursts.NewAcknowledgementSynchronizer(pool, incidentService, logger)

	healthServer := httpapi.NewServer(httpapi.ServerOptions{
		Address: cfg.HealthAddress,
		Pinger:  pool,
		Logger:  logger,
		Service: "worker",
	})

	errCh := make(chan error, 2)
	go func() {
		logger.Info("worker health server listening", "address", cfg.HealthAddress)
		errCh <- healthServer.ListenAndServe()
	}()
	go func() {
		notificationDispatcher := notifications.NewDispatcher(
			notifications.NewPostgresStore(pool),
			notifications.NewMattermostClient(&http.Client{Timeout: 10 * time.Second}),
			secrets, cfg.InstanceID, cfg.PublicURL,
		)
		var relay push.Relay
		if cfg.PushRelayURL != "" {
			relay = push.NewRelayClient(cfg.PushRelayURL, &http.Client{Timeout: 10 * time.Second})
		}
		pushDispatcher := push.NewDispatcher(
			push.NewPostgresStore(pool), relay, secrets,
			cfg.InstanceID, cfg.PublicURL, logger,
		)
		reconciliationDetector := reconciliation.NewDetector(pool, logger)
		reconciliationProcessor := reconciliation.NewProcessor(pool, cfg.InstanceID, logger)
		runtime := worker.New(
			pool, cfg.InstanceID, logger,
			notificationDispatcher, pushDispatcher,
			reconciliationDetector, reconciliationProcessor,
		).WithSupervisedRunners(
			connectorSync, uptimeKumaSync, patchMonSync, argusSync,
			indicatorCollector, burstAcknowledgements,
		)
		errCh <- runtime.Run(ctx)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return healthServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func runHealthcheck(url string) int {
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
