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
	"github.com/M0okz/cairnops/internal/connectors"
	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/controlplane"
	"github.com/M0okz/cairnops/internal/database"
	"github.com/M0okz/cairnops/internal/devices"
	"github.com/M0okz/cairnops/internal/httpapi"
	"github.com/M0okz/cairnops/internal/identity"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/indicators"
	"github.com/M0okz/cairnops/internal/maintenance"
	"github.com/M0okz/cairnops/internal/metrics"
	"github.com/M0okz/cairnops/internal/migrations"
	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/realtime"
	"github.com/M0okz/cairnops/internal/reconciliation"
	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/M0okz/cairnops/internal/systemhealth"
	"github.com/M0okz/cairnops/internal/version"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck("http://127.0.0.1:8080/api/v1/health/ready"))
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadServer()
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

	if err := migrations.Apply(ctx, pool); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	secrets, err := secretbox.LoadOrCreate(cfg.MasterKeyFile)
	if err != nil {
		return fmt.Errorf("load master key: %w", err)
	}
	connectorStore := connectors.NewPostgresStore(pool)
	zabbixClient := zabbix.NewClient()
	uptimeKumaClient := uptimekuma.NewClient()
	patchMonClient := patchmon.NewClient()
	incidentStore := incidents.NewPostgresStore(pool)
	connectorService := connectors.NewService(connectorStore, zabbixClient, uptimeKumaClient, patchMonClient, secrets)
	webhookService := connectors.NewWebhookService(connectorStore, incidentStore, secrets, cfg.PublicURL)
	incidentService := incidents.NewService(incidentStore, connectors.NewAcknowledger(connectorStore, zabbixClient, secrets))
	maintenanceService := maintenance.NewService(maintenance.NewPostgresStore(pool))
	notificationService := notifications.NewService(
		notifications.NewPostgresStore(pool),
		notifications.NewMattermostClient(&http.Client{Timeout: 10 * time.Second}),
		secrets,
	)
	deviceStore := devices.NewStore(pool, secrets, cfg.PublicURL)
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "local"
	}
	connectorSync := connectors.NewSynchronizer(connectorStore, incidentStore, zabbixClient, secrets, "server:"+hostname, logger)
	uptimeKumaSync := connectors.NewUptimeKumaSynchronizer(connectorStore, incidentStore, uptimeKumaClient, secrets, "server:"+hostname, logger)
	patchMonSync := connectors.NewPatchMonSynchronizer(connectorStore, incidentStore, patchMonClient, secrets, "server:"+hostname, logger)
	indicatorStore := indicators.NewStore(pool)
	indicatorService := indicators.NewService(indicatorStore, zabbixClient, uptimeKumaClient, patchMonClient, secrets)
	indicatorCollector := indicators.NewCollector(indicatorStore, zabbixClient, uptimeKumaClient, patchMonClient, secrets, logger)
	reconciliationStore := reconciliation.NewStore(pool)

	server := httpapi.NewServer(httpapi.ServerOptions{
		Address:         cfg.HTTPAddress,
		WebDir:          cfg.WebDir,
		PublicURL:       cfg.PublicURL,
		Pinger:          pool,
		Logger:          logger,
		Service:         "server",
		BootstrapToken:  cfg.BootstrapToken,
		Identity:        identity.NewStore(pool),
		ControlPlane:    controlplane.NewStore(pool),
		Metrics:         metrics.NewStore(pool),
		Indicators:      indicatorService,
		Connectors:      connectorService,
		Webhooks:        webhookService,
		Incidents:       incidentService,
		Maintenances:    maintenanceService,
		Notifications:   notificationService,
		Devices:         deviceStore,
		Events:          realtime.NewStore(pool),
		SystemHealth:    systemhealth.NewStore(pool),
		Reconciliations: reconciliationStore,
	})

	errCh := make(chan error, 5)
	go func() {
		logger.Info("server listening", "address", cfg.HTTPAddress, "version", version.Version)
		errCh <- server.ListenAndServe()
	}()
	go func() {
		if err := connectorSync.Run(ctx); err != nil {
			errCh <- err
		}
	}()
	go func() {
		if err := uptimeKumaSync.Run(ctx); err != nil {
			errCh <- err
		}
	}()
	go func() {
		if err := patchMonSync.Run(ctx); err != nil {
			errCh <- err
		}
	}()
	go func() {
		if err := indicatorCollector.Run(ctx); err != nil {
			errCh <- err
		}
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
