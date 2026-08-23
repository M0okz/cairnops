package reconciliation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/connectors"
	"github.com/jackc/pgx/v5/pgxpool"
)

const detectorInterval = 30 * time.Second

type Detector struct {
	pool     *pgxpool.Pool
	logger   *slog.Logger
	interval time.Duration
}

func NewDetector(pool *pgxpool.Pool, logger *slog.Logger) *Detector {
	return &Detector{pool: pool, logger: logger, interval: detectorInterval}
}

func (detector *Detector) Run(ctx context.Context) error {
	if err := detector.Refresh(ctx); err != nil {
		detector.logger.Error("refresh reconciliation suggestions", "error", err)
	}
	ticker := time.NewTicker(detector.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := detector.Refresh(ctx); err != nil {
				detector.logger.Error("refresh reconciliation suggestions", "error", err)
			}
		}
	}
}

type detectedTarget struct {
	identity connectors.TargetIdentity
	sources  []detectedSource
}

type detectedSource struct {
	summary  SourceSummary
	identity connectors.DiscoveredIdentity
}

type detectedSuggestion struct {
	key           string
	kind          string
	leftTargetID  string
	rightTargetID string
	sourceID      string
	match         connectors.TargetMatch
}

func (detector *Detector) Refresh(ctx context.Context) error {
	targets, err := detector.loadIdentities(ctx)
	if err != nil {
		return err
	}
	detected := make([]detectedSuggestion, 0)
	mergedPairs := make(map[string]struct{})
	identityIndex := make(map[string][]int)
	pairs := make(map[string][2]int)
	for targetIndex, target := range targets {
		keys := connectors.IdentityCandidateKeys(target.identity.Names, target.identity.Addresses, target.identity.Identifiers)
		for _, key := range keys {
			for _, otherIndex := range identityIndex[key] {
				pair := targetPairKey(targets[otherIndex].identity.ID, target.identity.ID)
				pairs[pair] = [2]int{otherIndex, targetIndex}
			}
			identityIndex[key] = append(identityIndex[key], targetIndex)
		}
	}
	pairKeys := make([]string, 0, len(pairs))
	for pair := range pairs {
		pairKeys = append(pairKeys, pair)
	}
	sort.Strings(pairKeys)
	for _, pair := range pairKeys {
		indexes := pairs[pair]
		left, right := targets[indexes[0]], targets[indexes[1]]
		matches := connectors.MatchTargets(connectors.DiscoveredIdentity{
			Names: left.identity.Names, Addresses: left.identity.Addresses,
			Identifiers: left.identity.Identifiers,
		}, []connectors.TargetIdentity{right.identity})
		if len(matches) == 0 {
			continue
		}
		match := matches[0]
		match.Confidence = conservativeConfidence(match)
		if match.Confidence != "low" {
			mergedPairs[pair] = struct{}{}
		}
		detected = append(detected, detectedSuggestion{
			key: "merge:" + pair, kind: "target_merge",
			leftTargetID: left.identity.ID, rightTargetID: right.identity.ID,
			match: match,
		})
	}

	// Une correction de Source n'est proposée que lorsque la Cible d'origine
	// possède d'autres Sources qui continuent à porter sa propre identité. Une
	// Cible mono-Source relève du rapprochement de Cibles, plus sûr et lisible.
	for _, origin := range targets {
		if len(origin.sources) < 2 {
			continue
		}
		for _, source := range origin.sources {
			candidateIndexes := make(map[int]struct{})
			for _, key := range connectors.IdentityCandidateKeys(source.identity.Names, source.identity.Addresses, source.identity.Identifiers) {
				for _, index := range identityIndex[key] {
					if targets[index].identity.ID != origin.identity.ID {
						candidateIndexes[index] = struct{}{}
					}
				}
			}
			orderedIndexes := make([]int, 0, len(candidateIndexes))
			for index := range candidateIndexes {
				orderedIndexes = append(orderedIndexes, index)
			}
			sort.Ints(orderedIndexes)
			candidates := make([]connectors.TargetIdentity, 0, len(orderedIndexes))
			for _, index := range orderedIndexes {
				candidates = append(candidates, targets[index].identity)
			}
			matches := connectors.MatchTargets(source.identity, candidates)
			if len(matches) == 0 {
				continue
			}
			matches[0].Confidence = conservativeConfidence(matches[0])
			if matches[0].Confidence == "low" {
				continue
			}
			destinationID := matches[0].Target.ID
			if _, wholeTargetMatch := mergedPairs[targetPairKey(origin.identity.ID, destinationID)]; wholeTargetMatch {
				continue
			}
			detected = append(detected, detectedSuggestion{
				key:  "move:" + source.summary.ID + ":" + destinationID,
				kind: "source_move", leftTargetID: origin.identity.ID,
				rightTargetID: destinationID, sourceID: source.summary.ID,
				match: matches[0],
			})
		}
	}

	tx, err := detector.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reconciliation suggestion refresh: %w", err)
	}
	defer tx.Rollback(ctx)
	refreshedAt := time.Now().UTC()
	for _, suggestion := range detected {
		evidence, err := json.Marshal(suggestion.match.Evidence)
		if err != nil {
			return fmt.Errorf("encode reconciliation evidence: %w", err)
		}
		contradictions, err := json.Marshal(suggestion.match.Contradictions)
		if err != nil {
			return fmt.Errorf("encode reconciliation contradictions: %w", err)
		}
		fingerprintInput := append(append([]byte{}, evidence...), contradictions...)
		digest := sha256.Sum256(fingerprintInput)
		fingerprint := hex.EncodeToString(digest[:])
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_target_reconciliation_suggestions (
				identity_key, kind, left_target_id, right_target_id, source_id,
				confidence, score, evidence, contradictions, evidence_fingerprint, last_detected_at
			) VALUES ($1, $2, $3::uuid, $4::uuid, nullif($5, '')::uuid,
			          $6, $7, $8::jsonb, $9::jsonb, $10, $11)
			ON CONFLICT (identity_key) DO UPDATE SET
				confidence = excluded.confidence,
				score = excluded.score,
				evidence = excluded.evidence,
				contradictions = excluded.contradictions,
				status = CASE
					WHEN cairnops_target_reconciliation_suggestions.status = 'accepted' THEN 'accepted'
					WHEN cairnops_target_reconciliation_suggestions.status = 'rejected'
					 AND cairnops_target_reconciliation_suggestions.evidence_fingerprint = excluded.evidence_fingerprint
					THEN 'rejected'
					WHEN cairnops_target_reconciliation_suggestions.status = 'snoozed'
					 AND cairnops_target_reconciliation_suggestions.evidence_fingerprint = excluded.evidence_fingerprint
					 AND cairnops_target_reconciliation_suggestions.snoozed_until > now()
					THEN 'snoozed'
					ELSE 'pending'
				END,
				evidence_fingerprint = excluded.evidence_fingerprint,
				last_detected_at = excluded.last_detected_at,
				updated_at = now()
		`, suggestion.key, suggestion.kind, suggestion.leftTargetID,
			suggestion.rightTargetID, suggestion.sourceID, suggestion.match.Confidence,
			suggestion.match.Score, evidence, contradictions, fingerprint, refreshedAt); err != nil {
			return fmt.Errorf("upsert reconciliation suggestion: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_target_reconciliation_suggestions
		SET status = 'superseded', updated_at = now()
		WHERE status IN ('pending', 'snoozed') AND last_detected_at < $1
	`, refreshedAt); err != nil {
		return fmt.Errorf("supersede stale reconciliation suggestions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reconciliation suggestion refresh: %w", err)
	}
	return nil
}

// conservativeConfidence sépare le rapprochement continu de l'aide à l'import.
// Un nom exact reste une piste utile, mais il n'est pas une preuve d'identité
// suffisante pour solliciter l'administrateur. Une suggestion actionnable exige
// un identifiant stable, une adresse commune ou deux familles de preuves.
func conservativeConfidence(match connectors.TargetMatch) string {
	if len(match.Contradictions) > 0 {
		return "low"
	}
	kinds := make(map[string]struct{}, len(match.Evidence))
	for _, evidence := range match.Evidence {
		kinds[evidence.Kind] = struct{}{}
	}
	if _, stable := kinds["same_machine_id"]; stable {
		return "high"
	}
	_, hostname := kinds["same_hostname"]
	_, address := kinds["same_ip"]
	if (hostname || address) && len(kinds) > 1 {
		return "high"
	}
	if hostname || address {
		return "medium"
	}
	return "low"
}

func (detector *Detector) loadIdentities(ctx context.Context) ([]detectedTarget, error) {
	rows, err := detector.pool.Query(ctx, `
		SELECT target.id::text, target.name,
		       coalesce(source.id::text, ''), coalesce(source.name, ''),
		       coalesce(source.kind, ''), coalesce(source.origin, ''),
		       coalesce(source.config, '{}'::jsonb), source.last_observed_at,
		       coalesce(binding.external_name, ''), coalesce(binding.metadata, '{}'::jsonb)
		FROM cairnops_targets target
		LEFT JOIN cairnops_signal_sources source ON source.target_id = target.id
		LEFT JOIN cairnops_connector_bindings binding ON binding.id = source.connector_binding_id
		WHERE target.archived_at IS NULL AND target.reconciled_into_target_id IS NULL
		ORDER BY target.id, source.id
	`)
	if err != nil {
		return nil, fmt.Errorf("load reconciliation identities: %w", err)
	}
	defer rows.Close()
	byID := make(map[string]*detectedTarget)
	order := make([]string, 0)
	for rows.Next() {
		var targetID, targetName, sourceID, sourceName, sourceKind, sourceOrigin, externalName string
		var configJSON, metadataJSON []byte
		var lastObservedAt *time.Time
		if err := rows.Scan(
			&targetID, &targetName, &sourceID, &sourceName, &sourceKind, &sourceOrigin,
			&configJSON, &lastObservedAt, &externalName, &metadataJSON,
		); err != nil {
			return nil, fmt.Errorf("scan reconciliation identity: %w", err)
		}
		target := byID[targetID]
		if target == nil {
			target = &detectedTarget{identity: connectors.TargetIdentity{
				TargetReference: connectors.TargetReference{ID: targetID, Name: targetName},
				Names:           []string{targetName},
			}}
			byID[targetID] = target
			order = append(order, targetID)
		}
		if sourceID == "" {
			continue
		}
		sourceIdentity := connectors.DiscoveredIdentity{Names: []string{sourceName, externalName}}
		appendIdentityDocument(&sourceIdentity, configJSON, lastObservedAt)
		appendIdentityDocument(&sourceIdentity, metadataJSON, lastObservedAt)
		target.identity.Names = append(target.identity.Names, sourceIdentity.Names...)
		target.identity.Addresses = append(target.identity.Addresses, sourceIdentity.Addresses...)
		target.identity.Identifiers = append(target.identity.Identifiers, sourceIdentity.Identifiers...)
		target.sources = append(target.sources, detectedSource{
			summary:  SourceSummary{ID: sourceID, TargetID: targetID, Name: sourceName, Kind: sourceKind, Origin: sourceOrigin},
			identity: sourceIdentity,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation identities: %w", err)
	}
	items := make([]detectedTarget, 0, len(order))
	for _, id := range order {
		items = append(items, *byID[id])
	}
	return items, nil
}

func appendIdentityDocument(identity *connectors.DiscoveredIdentity, encoded []byte, observedAt *time.Time) {
	var document map[string]any
	if json.Unmarshal(encoded, &document) != nil {
		return
	}
	appendText := func(destination *[]string, key string) {
		if value, ok := document[key].(string); ok && strings.TrimSpace(value) != "" {
			*destination = append(*destination, value)
		}
	}
	appendText(&identity.Names, "technical_name")
	appendText(&identity.Names, "friendly_name")
	appendText(&identity.Identifiers, "machine_id")
	appendText(&identity.Addresses, "url")
	appendText(&identity.Addresses, "hostname")
	appendText(&identity.Addresses, "host")
	appendText(&identity.Addresses, "address")
	appendText(&identity.Addresses, "server_name")
	appendText(&identity.Addresses, "name")
	if interfaces, ok := document["interfaces"].([]any); ok {
		for _, raw := range interfaces {
			item, _ := raw.(map[string]any)
			if address, ok := item["address"].(string); ok {
				identity.Addresses = append(identity.Addresses, address)
			}
		}
	}
	// Une adresse jamais confirmée depuis trente jours n'est plus une preuve
	// positive. Les noms et identifiants stables restent disponibles.
	if observedAt != nil && observedAt.Before(time.Now().UTC().Add(-30*24*time.Hour)) {
		identity.Addresses = nil
	}
}

func targetPairKey(left, right string) string {
	items := []string{left, right}
	sort.Strings(items)
	return items[0] + ":" + items[1]
}
