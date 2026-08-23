package indicators

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/secretbox"
)

type Collector struct {
	store      *Store
	zabbix     ZabbixClient
	uptimeKuma UptimeKumaClient
	patchMon   PatchMonClient
	secrets    *secretbox.Box
	logger     *slog.Logger
	interval   time.Duration
	now        func() time.Time
}

func NewCollector(store *Store, zabbixClient ZabbixClient, uptimeKumaClient UptimeKumaClient, patchMonClient PatchMonClient, secrets *secretbox.Box, logger *slog.Logger) *Collector {
	if logger == nil {
		logger = slog.Default()
	}
	return &Collector{store: store, zabbix: zabbixClient, uptimeKuma: uptimeKumaClient, patchMon: patchMonClient, secrets: secrets, logger: logger, interval: time.Minute, now: time.Now}
}

func (collector *Collector) Run(ctx context.Context) error {
	collector.tick(ctx)
	ticker := time.NewTicker(collector.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			collector.tick(ctx)
		}
	}
}

func (collector *Collector) tick(ctx context.Context) {
	now := collector.now().UTC()
	connectors, err := collector.store.RuntimeConnectors(ctx)
	if err != nil {
		collector.logger.Error("list contextual indicators", "error", err)
		return
	}
	for _, connector := range connectors {
		if err := collector.collect(ctx, connector, now); err != nil {
			collector.logger.Warn("collect contextual indicators", "connector_id", connector.ID, "error", err)
			if recordErr := collector.store.Fail(ctx, connector.ID, err.Error(), now); recordErr != nil {
				collector.logger.Error("record indicator capability failure", "connector_id", connector.ID, "error", recordErr)
			}
		}
	}
	if err := collector.store.Consolidate(ctx, now); err != nil {
		collector.logger.Error("apply indicator retention", "error", err)
	}
}

func (collector *Collector) collect(ctx context.Context, connector RuntimeConnector, now time.Time) error {
	credential, err := collector.secrets.Open(connector.CredentialSealed, "connector:"+connector.Kind+":"+connector.Endpoint)
	if err != nil {
		return fmt.Errorf("open connector credential: %w", err)
	}
	readings, missing := []Reading{}, map[string]string{}
	switch connector.Kind {
	case "zabbix":
		itemIDs := make([]string, 0, len(connector.Indicators))
		byItem := map[string][]RuntimeIndicator{}
		for _, indicator := range connector.Indicators {
			itemIDs = append(itemIDs, indicator.ExternalID)
			byItem[indicator.ExternalID] = append(byItem[indicator.ExternalID], indicator)
		}
		items, itemErr := collector.zabbix.Items(ctx, connector.Endpoint, string(credential), nil, itemIDs)
		if itemErr != nil {
			return itemErr
		}
		for _, item := range items {
			for _, indicator := range byItem[item.ID] {
				if item.LastValue == nil {
					missing[indicator.ID] = "Zabbix ne publie pas de valeur récente"
					continue
				}
				observed := now
				if item.LastClock != nil {
					observed = *item.LastClock
				}
				readings = append(readings, Reading{IndicatorID: indicator.ID, Value: *item.LastValue * selectionScale(indicator.Metadata), ObservedAt: observed})
				delete(missing, indicator.ID)
			}
		}
		for _, indicator := range connector.Indicators {
			found := false
			for _, reading := range readings {
				if reading.IndicatorID == indicator.ID {
					found = true
					break
				}
			}
			if !found {
				if _, exists := missing[indicator.ID]; !exists {
					missing[indicator.ID] = "Item Zabbix exact introuvable · nouvelle confirmation requise"
				}
			}
		}
	case "uptime_kuma":
		monitors, monitorErr := collector.uptimeKuma.Monitors(ctx, connector.Endpoint, string(credential))
		if monitorErr != nil {
			return monitorErr
		}
		byMonitor := map[string]any{}
		for _, monitor := range monitors {
			byMonitor[monitor.ID] = monitor
		}
		for _, indicator := range connector.Indicators {
			monitorValue, found := byMonitor[indicator.BindingExternalID]
			if !found {
				missing[indicator.ID] = "Monitor Uptime Kuma introuvable"
				continue
			}
			monitor := monitorValue.(uptimekuma.Monitor)
			switch indicator.SemanticKey {
			case "response.time":
				if monitor.ResponseMilliseconds != nil {
					readings = append(readings, Reading{IndicatorID: indicator.ID, Value: float64(*monitor.ResponseMilliseconds), ObservedAt: now})
				} else {
					missing[indicator.ID] = "Temps de réponse non publié"
				}
			case "certificate.days_remaining":
				if monitor.CertificateDaysRemaining != nil {
					readings = append(readings, Reading{IndicatorID: indicator.ID, Value: *monitor.CertificateDaysRemaining, ObservedAt: now})
				} else {
					missing[indicator.ID] = "Échéance du certificat non publiée"
				}
			case "certificate.valid":
				if monitor.CertificateValid != nil {
					value := 0.0
					if *monitor.CertificateValid {
						value = 1
					}
					readings = append(readings, Reading{IndicatorID: indicator.ID, Value: value, ObservedAt: now})
				} else {
					missing[indicator.ID] = "Validité du certificat non publiée"
				}
			}
		}
	case "patchmon":
		var credentials patchmon.Credentials
		if err := json.Unmarshal(credential, &credentials); err != nil {
			return fmt.Errorf("decode PatchMon credential: %w", err)
		}
		hosts, hostErr := collector.patchMon.Hosts(ctx, connector.Endpoint, credentials)
		if hostErr != nil {
			return hostErr
		}
		byHost := map[string]patchmon.Host{}
		for _, host := range hosts {
			byHost[host.ID] = host
		}
		for _, indicator := range connector.Indicators {
			host, found := byHost[indicator.BindingExternalID]
			if !found {
				missing[indicator.ID] = "Hôte PatchMon introuvable"
				continue
			}
			value, available := patchMonValue(host, indicator.SemanticKey, now)
			if !available {
				missing[indicator.ID] = "Dernière remontée PatchMon non publiée"
				continue
			}
			readings = append(readings, Reading{IndicatorID: indicator.ID, Value: value, ObservedAt: now})
		}
	default:
		return fmt.Errorf("connector %s does not support indicators", connector.Kind)
	}
	return collector.store.Record(ctx, connector.ID, now, readings, missing)
}

func patchMonValue(host patchmon.Host, semantic string, now time.Time) (float64, bool) {
	switch semantic {
	case "updates.count":
		return float64(host.UpdatesCount), true
	case "security_updates.count":
		return float64(host.SecurityUpdatesCount), true
	case "reboot.required":
		if host.NeedsReboot {
			return 1, true
		}
		return 0, true
	case "reporting.age":
		if host.LastUpdate == nil {
			return 0, false
		}
		return max(0, now.Sub(*host.LastUpdate).Seconds()), true
	default:
		return 0, false
	}
}
