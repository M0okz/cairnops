package softwareupdates

import "context"

// Argus supplies the source until an administrator explicitly overrides it.
// Lock bindings together with services so a concurrent connector refresh cannot
// cause adoption of an obsolete URL. No network calls happen in this transaction.
func (s *Store) syncArgusSources(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT s.binding_id::text,s.source,s.confirmed_at IS NOT NULL,
 coalesce(nullif(b.metadata->>'release_source_url',''),b.metadata->>'version_url','')
 FROM cairnops_software_services s JOIN cairnops_connector_bindings b ON b.id=s.binding_id
 JOIN cairnops_connectors c ON c.id=b.connector_id
 WHERE c.kind='argus' AND (s.confirmed_at IS NULL OR s.source_origin='argus')
 FOR UPDATE OF b,s`)
	if err != nil {
		return err
	}
	type candidate struct {
		id        string
		source    Source
		confirmed bool
		url       string
	}
	candidates := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.id, &c.source, &c.confirmed, &c.url); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range candidates {
		source := Suggest(c.url)
		if source == nil {
			if c.confirmed {
				_, err = tx.Exec(ctx, `UPDATE cairnops_software_services SET source='{}',confirmed_at=NULL,confirmed_by=NULL,revision=revision+1,state='awaiting_source',last_error='',next_check_at=now() WHERE binding_id=$1::uuid`, c.id)
			}
		} else if !c.confirmed || c.source != *source {
			_, err = tx.Exec(ctx, `UPDATE cairnops_software_services SET source=$2::jsonb,source_origin='argus',confirmed_at=now(),confirmed_by=NULL,revision=revision+1,state='pending',last_error='',next_check_at=now() WHERE binding_id=$1::uuid`, c.id, mustJSON(source))
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
