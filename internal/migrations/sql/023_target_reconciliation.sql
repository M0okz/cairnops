-- Un rapprochement ne supprime jamais l'identité absorbée. Son identifiant
-- continue de résoudre vers la Cible survivante et garde la décision humaine.
ALTER TABLE cairnops_targets
    ADD COLUMN identity_managed_at timestamptz,
    ADD COLUMN reconciled_into_target_id uuid REFERENCES cairnops_targets(id) ON DELETE RESTRICT,
    ADD COLUMN reconciled_at timestamptz,
    ADD COLUMN reconciled_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    ADD COLUMN reconciliation_reason text NOT NULL DEFAULT '',
    ADD CONSTRAINT cairnops_targets_reconciliation_check CHECK (
        (reconciled_into_target_id IS NULL AND reconciled_at IS NULL AND reconciliation_reason = '')
        OR (
            reconciled_into_target_id IS NOT NULL
            AND reconciled_into_target_id <> id
            AND reconciled_at IS NOT NULL
            AND archived_at IS NOT NULL
            AND length(btrim(reconciliation_reason)) BETWEEN 3 AND 1000
        )
    );

CREATE INDEX cairnops_targets_reconciled_into_idx
    ON cairnops_targets (reconciled_into_target_id)
    WHERE reconciled_into_target_id IS NOT NULL;

-- Les anciens noms restent des chemins de recherche vers l'identité active.
CREATE TABLE cairnops_target_aliases (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    absorbed_target_id uuid REFERENCES cairnops_targets(id) ON DELETE RESTRICT,
    alias text NOT NULL CHECK (length(btrim(alias)) BETWEEN 1 AND 160),
    origin text NOT NULL CHECK (origin IN ('reconciliation', 'rename', 'integration')),
    created_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (target_id, alias)
);

CREATE UNIQUE INDEX cairnops_target_aliases_absorbed_idx
    ON cairnops_target_aliases (absorbed_target_id)
    WHERE absorbed_target_id IS NOT NULL;

-- Une suggestion est une décision à examiner, jamais une mutation automatique.
-- identity_key rend l'écriture idempotente pour le détecteur incrémental.
CREATE TABLE cairnops_target_reconciliation_suggestions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    identity_key text NOT NULL UNIQUE CHECK (length(identity_key) BETWEEN 3 AND 255),
    kind text NOT NULL CHECK (kind IN ('target_merge', 'source_move')),
    left_target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    right_target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    source_id uuid REFERENCES cairnops_signal_sources(id) ON DELETE CASCADE,
    confidence text NOT NULL CHECK (confidence IN ('high', 'medium', 'low')),
    score integer NOT NULL CHECK (score >= 0),
    evidence jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(evidence) = 'array'),
    contradictions jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(contradictions) = 'array'),
    evidence_fingerprint text NOT NULL CHECK (length(evidence_fingerprint) = 64),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending', 'rejected', 'snoozed', 'accepted', 'superseded'
    )),
    snoozed_until timestamptz,
    decision_reason text NOT NULL DEFAULT '' CHECK (length(decision_reason) <= 1000),
    decided_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    decided_at timestamptz,
    last_detected_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (left_target_id <> right_target_id),
    CHECK ((kind = 'source_move' AND source_id IS NOT NULL) OR (kind = 'target_merge' AND source_id IS NULL)),
    CHECK ((status = 'snoozed' AND snoozed_until IS NOT NULL) OR status <> 'snoozed')
);

CREATE INDEX cairnops_target_reconciliation_suggestions_queue_idx
    ON cairnops_target_reconciliation_suggestions (status, confidence, score DESC, updated_at DESC);

-- L'opération survit à la page et aux redémarrages. Les étapes sont des états
-- réels du travail, pas un pourcentage décoratif.
CREATE TABLE cairnops_target_reconciliation_operations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL CHECK (kind IN ('target_merge', 'source_move')),
    primary_target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE RESTRICT,
    secondary_target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE RESTRICT,
    source_id uuid REFERENCES cairnops_signal_sources(id) ON DELETE RESTRICT,
    suggestion_id uuid REFERENCES cairnops_target_reconciliation_suggestions(id) ON DELETE SET NULL,
    archive_origin boolean NOT NULL DEFAULT false,
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 3 AND 1000),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    stage text NOT NULL DEFAULT 'preparing' CHECK (stage IN (
        'preparing', 'consolidating', 'reconciling_incidents',
        'recalculating_metrics', 'finalizing', 'completed', 'failed'
    )),
    preview jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(preview) = 'object'),
    result jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result) = 'object'),
    last_error text NOT NULL DEFAULT '' CHECK (length(last_error) <= 2000),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 3),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_until timestamptz,
    requested_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (primary_target_id <> secondary_target_id),
    CHECK ((kind = 'source_move' AND source_id IS NOT NULL) OR (kind = 'target_merge' AND source_id IS NULL))
);

CREATE INDEX cairnops_target_reconciliation_operations_due_idx
    ON cairnops_target_reconciliation_operations (next_attempt_at, created_at)
    WHERE status IN ('queued', 'running');

CREATE INDEX cairnops_target_reconciliation_operations_targets_idx
    ON cairnops_target_reconciliation_operations (primary_target_id, secondary_target_id, status);

-- Journal administratif commun aux rapprochements, corrections de Source et
-- décisions prises sur les suggestions.
CREATE TABLE cairnops_target_activity (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN (
        'reconciliation_started', 'reconciled', 'source_moved',
        'suggestion_rejected', 'suggestion_snoozed'
    )),
    actor_id uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    message text NOT NULL DEFAULT '' CHECK (length(message) <= 1000),
    data jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cairnops_target_activity_target_time_idx
    ON cairnops_target_activity (target_id, occurred_at DESC, id DESC);

ALTER TABLE cairnops_incident_activity DROP CONSTRAINT cairnops_incident_activity_kind_check;
ALTER TABLE cairnops_incident_activity ADD CONSTRAINT cairnops_incident_activity_kind_check CHECK (kind IN (
    'opened', 'signal_added', 'signal_resolved', 'resolved', 'invalidated',
    'acknowledged', 'ack_sync_succeeded', 'ack_sync_failed', 'upstream_acknowledged',
    'target_reconciled', 'source_reassigned'
));

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_kind_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_kind_check CHECK (kind IN (
    'target.changed', 'source.changed', 'observation.created', 'component.heartbeat',
    'connector.changed', 'incident.changed', 'maintenance.changed',
    'notification.changed', 'device.changed', 'indicator.changed',
    'reconciliation.changed'
));

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_entity_type_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_entity_type_check CHECK (
    entity_type IN (
        'target', 'source', 'component', 'connector', 'incident',
        'maintenance', 'notification', 'device', 'indicator', 'reconciliation'
    )
);

CREATE FUNCTION cairnops_reconciliation_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'reconciliation.changed',
        'reconciliation',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_reconciliation_suggestions_changed
AFTER INSERT OR UPDATE OF status, confidence, score, evidence, snoozed_until OR DELETE
ON cairnops_target_reconciliation_suggestions
FOR EACH ROW EXECUTE FUNCTION cairnops_reconciliation_changed_event();

CREATE TRIGGER cairnops_reconciliation_operations_changed
AFTER INSERT OR UPDATE OF status, stage, last_error OR DELETE
ON cairnops_target_reconciliation_operations
FOR EACH ROW EXECUTE FUNCTION cairnops_reconciliation_changed_event();
