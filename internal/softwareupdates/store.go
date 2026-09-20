package softwareupdates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool    *pgxpool.Pool
	secrets *secretbox.Box
}

func NewStore(pool *pgxpool.Pool, secrets *secretbox.Box) *Store {
	return &Store{pool: pool, secrets: secrets}
}

const serviceSelect = `SELECT s.binding_id::text,b.target_id::text,b.external_name,s.installed_version,s.target_version,s.observed_at,
 s.known AND b.integration_enabled AND c.status <> 'disabled' AND c.last_checked_at > now()-make_interval(secs => c.sync_interval_seconds*3),
 s.source,s.confirmed_at,s.revision,s.state,s.last_error,s.checked_at,s.collection,s.collection_revision,s.content_hash,
 coalesce(nullif(b.metadata->>'release_source_url',''),b.metadata->>'version_url','')
 FROM cairnops_software_services s JOIN cairnops_connector_bindings b ON b.id=s.binding_id
 JOIN cairnops_connectors c ON c.id=b.connector_id JOIN cairnops_targets t ON t.id=b.target_id `

func scanService(row pgx.Row) (Service, error) {
	var s Service
	var candidate string
	err := row.Scan(&s.ID, &s.TargetID, &s.Name, &s.Installed, &s.Target, &s.ObservedAt, &s.Known, &s.Source, &s.ConfirmedAt, &s.Revision, &s.State, &s.LastError, &s.CheckedAt, &s.Collection, &s.CollectionRevision, &s.ContentHash, &candidate)
	s.Suggested = Suggest(candidate)
	s.Analyses = []Analysis{}
	s.History = []History{}
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}
func (s *Store) List(ctx context.Context, targetID string) ([]Service, error) {
	projection := strings.Replace(serviceSelect, "s.collection,s.collection_revision", "NULL::jsonb,s.collection_revision", 1)
	rows, err := s.pool.Query(ctx, projection+`WHERE ($1='' OR b.target_id::text=$1) AND t.archived_at IS NULL ORDER BY b.external_name,s.binding_id`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Service{}
	for rows.Next() {
		v, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		v.Collection = nil
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) Get(ctx context.Context, id string) (Service, error) {
	v, err := scanService(s.pool.QueryRow(ctx, serviceSelect+`WHERE s.binding_id=$1::uuid`, id))
	if err != nil {
		return v, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id,revision,installed_version,target_version,source,content_hash,result,model,created_at,notes FROM cairnops_software_analyses WHERE binding_id=$1::uuid ORDER BY id DESC LIMIT 30`, id)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var a Analysis
		if err = rows.Scan(&a.ID, &a.Revision, &a.Installed, &a.Target, &a.Source, &a.Hash, &a.Result, &a.Model, &a.CreatedAt, &a.Notes); err != nil {
			rows.Close()
			return v, err
		}
		a.Current = a.Revision == v.Revision && a.Hash == v.ContentHash && v.State == "ready"
		v.Analyses = append(v.Analyses, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return v, err
	}
	rows, err = s.pool.Query(ctx, `SELECT installed_version,target_version,observed_at FROM cairnops_software_history WHERE binding_id=$1::uuid ORDER BY id DESC LIMIT 500`, id)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var h History
		if err = rows.Scan(&h.Installed, &h.Target, &h.ObservedAt); err != nil {
			return v, err
		}
		v.History = append(v.History, h)
	}
	return v, rows.Err()
}
func (s *Store) Confirm(ctx context.Context, id, actor string, input Source) error {
	source, err := NormalizeSource(input)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(source)
	result, err := s.pool.Exec(ctx, `UPDATE cairnops_software_services s SET source=$2::jsonb,confirmed_by=$3::uuid,confirmed_at=now(),revision=revision+1,next_check_at=now(),state='pending',last_error='' WHERE binding_id=$1::uuid AND EXISTS(SELECT 1 FROM cairnops_connector_bindings b WHERE b.id=s.binding_id)`, id, b, actor)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) Config(ctx context.Context) (AIConfig, error) {
	var c AIConfig
	err := s.pool.QueryRow(ctx, `SELECT enabled,endpoint,model,credential_sealed<>'' FROM cairnops_software_ai WHERE singleton`).Scan(&c.Enabled, &c.Endpoint, &c.Model, &c.KeyConfigured)
	return c, err
}
func (s *Store) SaveConfig(ctx context.Context, c AIConfig) error {
	c.Endpoint = strings.TrimSuffix(strings.TrimSpace(c.Endpoint), "/")
	c.Model = strings.TrimSpace(c.Model)
	if _, err := publicURL(c.Endpoint); err != nil {
		return err
	}
	if c.Model == "" || len(c.Model) > 160 || len(c.APIKey) > 8192 {
		return fmt.Errorf("%w: model and valid key required", ErrInvalid)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var old, secret string
	err = tx.QueryRow(ctx, `SELECT endpoint,credential_sealed FROM cairnops_software_ai WHERE singleton FOR UPDATE`).Scan(&old, &secret)
	if err != nil {
		return err
	}
	if c.APIKey != "" {
		secret, err = s.secrets.Seal([]byte(c.APIKey), "software-ai:"+c.Endpoint)
		if err != nil {
			return err
		}
	} else if old != c.Endpoint {
		return fmt.Errorf("%w: provide a new key when changing the provider", ErrInvalid)
	}
	if c.Enabled && secret == "" {
		return fmt.Errorf("%w: API key required", ErrInvalid)
	}
	_, err = tx.Exec(ctx, `UPDATE cairnops_software_ai SET enabled=$1,endpoint=$2,model=$3,credential_sealed=$4,updated_at=now() WHERE singleton`, c.Enabled, c.Endpoint, c.Model, secret)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE cairnops_software_services SET next_check_at=now() WHERE state IN ('awaiting_ai','retry')`)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) runtimeConfig(ctx context.Context) (AIConfig, error) {
	c, err := s.Config(ctx)
	if err != nil || !c.Enabled {
		return c, err
	}
	var sealed string
	err = s.pool.QueryRow(ctx, `SELECT credential_sealed FROM cairnops_software_ai WHERE singleton`).Scan(&sealed)
	if err != nil {
		return c, err
	}
	key, err := s.secrets.Open(sealed, "software-ai:"+c.Endpoint)
	c.APIKey = string(key)
	return c, err
}
