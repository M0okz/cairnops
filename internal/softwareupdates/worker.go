package softwareupdates

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/M0okz/cairnops/internal/versions"
	"github.com/jackc/pgx/v5"
)

type Worker struct {
	store  *Store
	client *http.Client
	logger *slog.Logger
}

func NewWorker(store *Store, logger *slog.Logger) *Worker {
	return &Worker{store: store, client: publicClient(), logger: logger}
}
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		if err := w.tick(ctx); err != nil && ctx.Err() == nil {
			w.logger.Warn("software notes processing failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (w *Worker) tick(ctx context.Context) error {
	if err := w.store.syncArgusSources(ctx); err != nil {
		return err
	}
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
	var id string
	err := w.store.pool.QueryRow(ctx, `WITH due AS (
 SELECT s.binding_id FROM cairnops_software_services s JOIN cairnops_connector_bindings b ON b.id=s.binding_id
 JOIN cairnops_targets t ON t.id=b.target_id JOIN cairnops_connectors c ON c.id=b.connector_id
 WHERE s.confirmed_at IS NOT NULL AND s.known AND b.integration_enabled AND t.archived_at IS NULL
 AND c.status<>'disabled' AND c.last_checked_at>now()-make_interval(secs=>c.sync_interval_seconds*3)
 AND s.next_check_at<=now() AND (s.lease_until IS NULL OR s.lease_until<now())
 ORDER BY s.next_check_at LIMIT 1 FOR UPDATE OF s SKIP LOCKED)
 UPDATE cairnops_software_services s SET lease_token=$1,lease_until=now()+interval '10 minutes'
 FROM due WHERE s.binding_id=due.binding_id RETURNING s.binding_id::text`, token).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	jobCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	service, err := w.store.Get(jobCtx, id)
	if err != nil {
		return err
	}
	finish := func(state, message string, delay int) error {
		_, e := w.store.pool.Exec(ctx, `UPDATE cairnops_software_services SET state=CASE WHEN revision=$3 THEN $4 ELSE state END,last_error=CASE WHEN revision=$3 THEN $5 ELSE last_error END,next_check_at=CASE WHEN revision=$3 THEN now()+make_interval(secs=>$6) ELSE next_check_at END,lease_token=NULL,lease_until=NULL WHERE binding_id=$1::uuid AND lease_token=$2`, id, token, service.Revision, state, message, delay)
		return e
	}
	// Seule une cible plus récente justifie une collecte : une cible antérieure
	// ou non ordonnable n'a pas de notes à comparer et ne doit pas consommer de quota.
	switch versions.Assess(service.Installed, service.Target).Situation {
	case versions.Current:
		return finish("up_to_date", "", 86400)
	case versions.TargetOlder, versions.Unordered:
		return finish("not_applicable", "", 86400)
	}
	deferred := func(err error) error {
		var limited *RateLimitedError
		if errors.As(err, &limited) {
			delay := int(time.Until(limited.Until).Seconds()) + 30
			return finish("retry", limited.Error(), max(delay, 60))
		}
		return finish("retry", err.Error(), 3600)
	}
	c, err := Collect(jobCtx, w.client, service.Source, service.Installed, service.Target)
	if err != nil {
		return deferred(err)
	}
	bytes, _ := json.Marshal(c)
	hashBytes := sha256.Sum256(bytes)
	hash := hex.EncodeToString(hashBytes[:])
	result, err := w.store.pool.Exec(jobCtx, `UPDATE cairnops_software_services SET collection=$4::jsonb,collection_revision=$3,checked_at=now(),content_hash=$5 WHERE binding_id=$1::uuid AND lease_token=$2 AND revision=$3`, id, token, service.Revision, bytes, hash)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return finish("pending", "", 0)
	}
	// Réutiliser le résultat exact même après un retour à une comparaison déjà analysée.
	var cachedID int64
	err = w.store.pool.QueryRow(jobCtx, `SELECT id FROM cairnops_software_analyses WHERE binding_id=$1::uuid AND installed_version=$2 AND target_version=$3 AND source=$4::jsonb AND content_hash=$5 ORDER BY id DESC LIMIT 1`, id, service.Installed, service.Target, mustJSON(service.Source), hash).Scan(&cachedID)
	if err == nil {
		_, err = w.store.pool.Exec(jobCtx, `UPDATE cairnops_software_analyses SET revision=$2 WHERE id=$1`, cachedID, service.Revision)
		if err != nil {
			return err
		}
		return finish("ready", "", 86400)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if !hasNotes(c) {
		// Aucune note publiée : la vérification quotidienne détectera leur arrivée.
		return finish("notes_unavailable", "", 86400)
	}
	cfg, err := w.store.runtimeConfig(jobCtx)
	if err != nil {
		return finish("retry", "ai_configuration_unavailable", 3600)
	}
	if !cfg.Enabled {
		return finish("awaiting_ai", "", 86400)
	}
	summary, err := Generate(jobCtx, w.client, cfg, c)
	if err != nil {
		return deferred(err)
	}
	_, err = w.store.pool.Exec(jobCtx, `INSERT INTO cairnops_software_analyses(binding_id,revision,installed_version,target_version,source,content_hash,result,model,notes)
 SELECT binding_id,revision,installed_version,target_version,source,$4,$5::jsonb,$6,$7::jsonb FROM cairnops_software_services
 WHERE binding_id=$1::uuid AND lease_token=$2 AND revision=$3`, id, token, service.Revision, hash, mustJSON(summary), cfg.Model, mustJSON(c.Notes))
	if err != nil {
		return err
	}
	return finish("ready", "", 86400)
}
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("encode software data: %v", err))
	}
	return b
}

func hasNotes(c Collection) bool {
	for _, note := range c.Notes {
		if !note.Missing {
			return true
		}
	}
	return false
}
