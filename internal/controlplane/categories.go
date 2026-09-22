package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Category describes the resource, independently of its connector and health.
type Category string

const (
	CategoryService        Category = "service"
	CategoryInfrastructure Category = "infrastructure"
	CategoryScheduledTask  Category = "scheduled_task"
	CategorySoftware       Category = "software"
	CategoryUnclassified   Category = "unclassified"
)

func (c Category) Valid() bool {
	switch c {
	case CategoryService, CategoryInfrastructure, CategoryScheduledTask, CategorySoftware, CategoryUnclassified:
		return true
	}
	return false
}

type categorySignal struct {
	Kind     string         `json:"kind"`
	Metadata map[string]any `json:"metadata"`
}

func suggestCategory(signals []categorySignal) Category {
	found := map[Category]bool{}
	unknown := false
	for _, signal := range signals {
		category := CategoryUnclassified
		switch signal.Kind {
		case "http", "tcp", "dns":
			category = CategoryService
		case "icmp":
			category = CategoryInfrastructure
		case "uptime_kuma":
			switch signal.Metadata["type"] {
			case "http", "keyword", "json-query", "port", "dns", "grpc-keyword", "websocket":
				category = CategoryService
			case "ping":
				category = CategoryInfrastructure
			}
		case "patchmon":
			if signal.Metadata["machine_id"] != nil || signal.Metadata["os_type"] != nil {
				category = CategoryInfrastructure
			}
		case "proxmox":
			switch signal.Metadata["resource_type"] {
			case "node", "qemu", "lxc", "storage", "cluster":
				category = CategoryInfrastructure
			}
		case "zabbix":
			if items, ok := signal.Metadata["interfaces"].([]any); ok && len(items) > 0 {
				category = CategoryInfrastructure
			}
		case "argus":
			if _, ok := signal.Metadata["deployed_version"]; ok {
				category = CategorySoftware
			}
		}
		if category == CategoryUnclassified {
			unknown = true
		} else {
			found[category] = true
		}
	}
	// Version tracking enriches a resource whose operational type is known.
	if len(found) > 1 {
		delete(found, CategorySoftware)
	}
	if len(found) == 1 {
		for category := range found {
			if category == CategorySoftware && unknown {
				return CategoryUnclassified
			}
			return category
		}
	}
	return CategoryUnclassified
}

// One bulk read keeps category projection independent of fleet size. Manual
// choices are stored separately and never overwritten by connector discovery.
func (store *Store) classifyTargets(ctx context.Context, targets []Target) error {
	rows, err := store.pool.Query(ctx, `
 SELECT target.id::text, target.category,
 (SELECT max(success.observed_at) FROM cairnops_signal_sources source
 CROSS JOIN LATERAL (SELECT observation.observed_at FROM cairnops_observations observation
 WHERE observation.source_id = source.id AND observation.outcome = 'healthy'
 ORDER BY observation.observed_at DESC, observation.id DESC LIMIT 1) success
 WHERE source.target_id = target.id AND source.kind = 'heartbeat'),
 coalesce((SELECT jsonb_agg(jsonb_build_object('kind', signal.kind, 'metadata', signal.metadata)) FROM (
 SELECT source.kind, source.config AS metadata FROM cairnops_signal_sources source WHERE source.target_id = target.id AND source.origin = 'native'
 UNION ALL
 SELECT connector.kind, binding.metadata FROM cairnops_connector_bindings binding JOIN cairnops_connectors connector ON connector.id = binding.connector_id WHERE binding.target_id = target.id
 ) signal), '[]'::jsonb)
 FROM cairnops_targets target WHERE target.archived_at IS NULL AND target.id = ANY($1::uuid[])`, resourceIDs(targets))
	if err != nil {
		return fmt.Errorf("read resource categories: %w", err)
	}
	defer rows.Close()
	indexes := map[string]int{}
	for i := range targets {
		indexes[targets[i].ID] = i
	}
	for rows.Next() {
		var id string
		var manual *Category
		var lastSuccess *time.Time
		var raw []byte
		if err := rows.Scan(&id, &manual, &lastSuccess, &raw); err != nil {
			return err
		}
		var signals []categorySignal
		if err := json.Unmarshal(raw, &signals); err != nil {
			return err
		}
		if i, ok := indexes[id]; ok {
			targets[i].LastSuccessAt = lastSuccess
			targets[i].SuggestedCategory = suggestCategory(signals)
			targets[i].Category = targets[i].SuggestedCategory
			targets[i].CategoryManual = manual != nil
			if manual != nil {
				targets[i].Category = *manual
			}
		}
	}
	return rows.Err()
}

func resourceIDs(targets []Target) []string {
	ids := make([]string, len(targets))
	for i := range targets {
		ids[i] = targets[i].ID
	}
	return ids
}
